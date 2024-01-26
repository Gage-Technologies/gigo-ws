package server

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"runtime/debug"
	"sync"
	"sync/atomic"
	"time"

	"cdr.dev/slog"

	"gigo-ws/coder/agent/agent/server/core"
	"gigo-ws/coder/agent/agent/server/payload"

	"github.com/go-playground/validator/v10"
	"github.com/sourcegraph/conc/pool"
	"nhooyr.io/websocket"
	"nhooyr.io/websocket/wsjson"
)

const (
	// max message size is 10Mib
	maxMessageSize = 10 * 1024 * 1024
)

// masterWebSocket
//
//	masterWebSocket is used to pass the properties of a master web socket
//	connection to the multiple goroutines that will be spawned to handle
//	the connection.
type masterWebSocket struct {
	// web socket connection
	ws *websocket.Conn

	// time of the last websocket interaction
	lastInteraction *atomic.Pointer[time.Time]

	// worker pool to manage the concurrent resources of this connection
	pool *pool.Pool

	// context for the connection
	ctx context.Context

	// cancel function for the connection
	cancel context.CancelCauseFunc

	// logger for the connection
	logger slog.Logger

	// map of handlers for each message type
	handlers map[payload.WebSocketMessageType]WebSocketHandlerFunc

	// map of active commands
	activeCommands sync.Map
}

// WebSocketHandlerFunc
//
//	WebSocketHandlerFunc is the function signature for handlers meant to process
//	a specific WebSocketPayload type when sent from the client to the server.
type WebSocketHandlerFunc func(socket *masterWebSocket, msg *payload.WebSocketPayload[any])

// MasterWebSocket
//
//	The master web socket to manage all messages for the API.
func (a *HttpApi) MasterWebSocket(w http.ResponseWriter, r *http.Request) {
	// upgrade the connection to a web socket
	ws, err := websocket.Accept(w, r, &websocket.AcceptOptions{
		OriginPatterns: []string{"*"},
	})
	if err != nil {
		a.handleError(w, r, http.StatusInternalServerError, DefaultErrorMessage, err)
		return
	}

	// update the max size of a single payload to 256KiB
	ws.SetReadLimit(maxMessageSize)

	// WARNING: we can no longer use the built in response handlers like
	// handleError and handleJsonResponse we have just hijacked the connection
	// and upgraded to a websocket

	// create context for the connection
	// NOTE: we create an entirely new context since the request context
	// will be terminated once this function returns but the socket will
	// live much longer than that.
	ctx, cancel := context.WithCancelCause(context.Background())

	a.Logger.Info(
		r.Context(),
		"new web socket connection",
	)

	// send the init payload to start the connection with the client
	err = wsjson.Write(ctx, ws, payload.PrepPayload(
		a.Snowflake.Generate().Base36(),
		payload.WebSocketMessageTypeInit,
		payload.InitPayload{
			Epoch:  time.Now().UnixMilli(),
			NodeID: fmt.Sprintf("%v", a.NodeID),
		},
	))
	if err != nil {
		cancel(fmt.Errorf("failed to send init payload: %w", err))
		a.Logger.Error(
			r.Context(),
			"failed to send init payload",
			slog.Error(err),
		)
		return
	}

	// wrap the last message time in an atomic pointer so we can
	// concurrently update it as we receive messages
	var safeTime atomic.Pointer[time.Time]
	t := time.Now()
	safeTime.Store(&t)

	// assemble the master web socket
	socket := &masterWebSocket{
		ws:              ws,
		lastInteraction: &safeTime,
		// we allocate 4 workers for the socket plus one for the user poller
		pool:   pool.New().WithMaxGoroutines(5),
		ctx:    ctx,
		cancel: cancel,
		logger: a.Logger,
		handlers: map[payload.WebSocketMessageType]WebSocketHandlerFunc{
			payload.WebSocketMessageTypeExecRequest:       a.ExecCode,
			payload.WebSocketMessageTypeLintRequest:       a.LintCode,
			payload.WebSocketMessageTypeCancelExecRequest: a.CancelExec,
			payload.WebSocketMessageTypeStdinExecRequest:  a.StdinExec,
		},
	}

	// launch the master web socket loop via the server's waitgroup
	a.wg.Go(func() {
		a.activeConnections.Add(1)
		defer a.activeConnections.Add(-1)
		a.masterWebSocketLoop(socket)
	})
}

// masterWebSocketLoop
//
//	The master web socket loop to manage all messages for the API.
func (a *HttpApi) masterWebSocketLoop(socket *masterWebSocket) {
	// defer the cleanup function to ensure our resources are cleaned up
	defer func() {
		// cancel the context to ensure all goroutines are terminated
		socket.cancel(fmt.Errorf("masterWebSocketLoop: exiting on closure"))

		// close the web socket connection
		// NOTE: if we are closing here then something has gone wrong
		socket.ws.Close(websocket.StatusInternalError, "internal server error")

		// cancel all of the command context
		socket.activeCommands.Range(func(_ any, value any) bool {
			value.(*core.ActiveCommand).Cancel()
			return true
		})

		// wait for the cleanup of all the goroutines associatd with this websocket
		socket.pool.Wait()
	}()

	// create a ticker to send a ping to the client every 5 seconds
	// to ensure the connection is still alive
	ticker := time.NewTicker(5 * time.Second)

	// create a channel to receive messages from the client
	// NOTE: we use a buffered channel here to prevent the goroutine
	// from blocking if the client is sending messages faster than
	// we can process them.
	messages := make(chan *payload.WebSocketPayload[any], 100)

	// launch a goroutine to read messages from the client
	a.wg.Go(func() {
		a.masterWebSocketRead(socket, messages)
	})

	// create atomic int for the amount of active workers
	var activeWorkers atomic.Int32

	// loop until the connection is closed handling messages and executing pings
	for {
		select {
		case <-socket.ctx.Done():
			if ctxErr := context.Cause(socket.ctx); ctxErr != nil {
				socket.logger.Debug(socket.ctx, "masterWebSocketLoop closed", slog.Error(ctxErr))
			}
			return
		case <-ticker.C:
			// log active workers
			// socket.logger.Debug("active workers", zap.Int32("count", activeWorkers.Load()))

			// send ping on ticker interval to keep the connection alive
			err := socket.ws.Ping(socket.ctx)
			if err != nil {
				socket.logger.Error(socket.ctx, "failed to send ping to client", slog.Error(err))
				return
			}
		case message := <-messages:
			// socket.logger.Info("Received message", zap.String("MessageType", message.Type.String()))
			// attempt to load the handler for the message type
			handler, ok := socket.handlers[message.Type]
			if !ok {
				socket.logger.Warn(socket.ctx, "received unknown message type", slog.F("type", int(message.Type)))
				continue
			}

			// socket.logger.Debug("received message", zap.String("type", message.Type.String()))

			// submit the handler to the worker pool
			socket.pool.Go(func() {
				// increment the active workers
				activeWorkers.Add(1)
				defer activeWorkers.Add(-1)

				// execute the handler
				a.handlerWrapper(handler, socket, message)
			})
		}
	}

}

// masterWebSocketRead
//
//	The master web socket read loop to read messages from the client.
func (a *HttpApi) masterWebSocketRead(socket *masterWebSocket, messages chan *payload.WebSocketPayload[any]) {
	// defer the cleanup function to ensure our resources are cleaned up
	defer func() {
		if r := recover(); r != nil {
			socket.cancel(fmt.Errorf("masterWebSocketRead: panic: %v", r))
			socket.logger.Error(socket.ctx, "masterWebSocketRead: panic: ", slog.Error(r.(error)))
		} else {
			socket.cancel(fmt.Errorf("masterWebSocketRead: exiting on closure"))
		}
		socket.ws.Close(websocket.StatusNormalClosure, "")
	}()

	// loop until the connection is closed
	for {
		// read the next message from the client
		var message payload.WebSocketPayload[any]
		err := wsjson.Read(socket.ctx, socket.ws, &message)
		if err != nil {
			if websocket.CloseStatus(err) != -1 || errors.Is(err, io.EOF) {
				socket.logger.Debug(socket.ctx, "websocket closed")
			} else {
				socket.logger.Error(socket.ctx, "failed to read message from client", slog.Error(err))
			}
			return
		}

		// validate the message payload
		if !a.validateWebSocketPayload(socket, &message, nil) {
			continue
		}

		// update the last interaction time
		t := time.Now()
		socket.lastInteraction.Store(&t)

		// send the message to the write loop
		messages <- &message
	}
}

// validateWebSocketPayload
//
//	Loads a json request from the websocket payload and validates it's schema.
func (a *HttpApi) validateWebSocketPayload(socket *masterWebSocket, msg *payload.WebSocketPayload[any], inner interface{}) bool {
	// validate the schema
	var err error
	if inner != nil {
		err = a.validator.Struct(inner)
	} else {
		err = a.validator.Struct(msg)
	}

	// handle known validation errors
	var validationErrors validator.ValidationErrors
	if errors.As(err, &validationErrors) {
		// create a map and fill with the field names and validation errors
		failedValidations := make(map[string]string)
		for _, validationError := range validationErrors {
			failedValidations[validationError.Field()] = validationError.Tag()
		}

		// return the error payload to the client
		err = wsjson.Write(socket.ctx, socket.ws, payload.PrepPayload(
			msg.SequenceID,
			payload.WebSocketMessageTypeValidationError,
			payload.ValidationErrorPayload{
				GenericErrorPayload: payload.GenericErrorPayload{
					Error: "validation failed",
					Code:  payload.WebSocketErrorCodeBadRequest,
				},
				ValidationErrors: failedValidations,
			},
		))
		if err != nil {
			a.Logger.Error(
				socket.ctx,
				"failed to send validation error payload",
				slog.Error(err),
			)
		}

		return false
	}

	// handle unexpected validation errors
	if err != nil {
		// return the error payload to the client
		err = wsjson.Write(socket.ctx, socket.ws, payload.PrepPayload(
			msg.SequenceID,
			payload.WebSocketMessageTypeGenericError,
			payload.GenericErrorPayload{
				Error: DefaultErrorMessage,
				Code:  payload.WebSocketErrorCodeServerError,
			},
		))
		if err != nil {
			a.Logger.Error(
				socket.ctx,
				"failed to send validation error payload",
				slog.Error(err),
			)
		}
		return false
	}

	return true
}

// handlerWrapper
//
//	Wraps all handler functions to provide global middleware such as panic recovery.
func (a *HttpApi) handlerWrapper(handler WebSocketHandlerFunc, socket *masterWebSocket, msg *payload.WebSocketPayload[any]) {
	// catch any panics and return them as errors
	defer func() {
		if r := recover(); r != nil {
			// format the panic and the stack trace into an error
			panicErr := fmt.Errorf("panic: %v\n%s", r, debug.Stack())
			socket.logger.Error(
				socket.ctx,
				"unexpected panic in websocket handler",
				slog.F("msg_type", msg.Type.String()),
				slog.Error(panicErr),
			)
			// return the error payload to the client
			err := wsjson.Write(socket.ctx, socket.ws, payload.PrepPayload(
				msg.SequenceID,
				payload.WebSocketMessageTypeGenericError,
				payload.GenericErrorPayload{
					Error: DefaultErrorMessage,
					Code:  payload.WebSocketErrorCodeServerError,
				},
			))
			if err != nil {
				a.Logger.Error(
					socket.ctx,
					"failed to send error payload",
					slog.Error(err),
				)
			}
		}
	}()

	// handle the message
	handler(socket, msg)
}
