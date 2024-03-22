package server

import (
	"cdr.dev/slog"
	"context"
	"encoding/json"
	"gigo-ws/coder/agent/agent/lsp"
	"gigo-ws/coder/agent/agent/server/payload"
	"nhooyr.io/websocket/wsjson"
)

func (a *HttpApi) LaunchLsp(socket *masterWebSocket, msg *payload.WebSocketPayload[any]) {
	// marshall and unmarshall into the expected payload so we can validate it
	buf, err := json.Marshal(msg.Payload)
	if err != nil {
		// return the error payload to the client
		err := wsjson.Write(socket.ctx, socket.ws, payload.PrepPayload(
			msg.SequenceID,
			payload.WebSocketMessageTypeValidationError,
			payload.ValidationErrorPayload{
				GenericErrorPayload: payload.GenericErrorPayload{
					Error: "validation failed",
					Code:  payload.WebSocketErrorCodeBadRequest,
				},
				ValidationErrors: map[string]string{
					"payload": "payload is not of type ExecRequestPayload",
				},
			},
		))
		if err != nil {
			socket.logger.Error(
				socket.ctx,
				"failed to send validation error payload",
				slog.Error(err),
			)
		}
	}

	var mes payload.LaunchLspRequestPayload

	err = json.Unmarshal(buf, &mes)
	if err != nil {
		// return the error payload to the client
		err := wsjson.Write(socket.ctx, socket.ws, payload.PrepPayload(
			msg.SequenceID,
			payload.WebSocketMessageTypeValidationError,
			payload.ValidationErrorPayload{
				GenericErrorPayload: payload.GenericErrorPayload{
					Error: "validation failed",
					Code:  payload.WebSocketErrorCodeBadRequest,
				},
				ValidationErrors: map[string]string{
					"payload": "payload is not of type ExecRequestPayload",
				},
			},
		))
		if err != nil {
			socket.logger.Error(
				socket.ctx,
				"failed to send validation error payload",
				slog.Error(err),
			)
		}
	}

	// ensure that we have the right internal payload type
	if !a.validateWebSocketPayload(socket, msg, mes) {
		return
	}

	socket.logger.Info(socket.ctx, "lsp launch requested", slog.F("lang", mes.Lang))

	// check if there is an existing lsp
	l := a.lsp.Load()

	// if there is one then we need to make sure it's the same lang
	if l != nil {
		if l.Language == mes.Lang {
			socket.logger.Info(socket.ctx, "lsp already active", slog.F("lang", mes.Lang))
			err = wsjson.Write(socket.ctx, socket.ws, payload.PrepPayload(
				msg.SequenceID,
				payload.WebSocketMessageTypeLaunchLspResponse,
				payload.LaunchLspResponsePayload{
					Success: true,
				},
			))
			if err != nil {
				socket.logger.Error(
					socket.ctx,
					"failed to send new exec payload",
					slog.Error(err),
				)
			}
			return
		}

		socket.logger.Info(socket.ctx, "lsp of different language active; shutting down", slog.F("lang", mes.Lang), slog.F("old_lang", l.Language))

		// close the lsp if it is the same language
		l.Close()
	}

	socket.logger.Info(socket.ctx, "launching lsp", slog.F("lang", mes.Lang))

	err = lsp.PrepLsp(mes.Lang, socket.ctx, mes.Content, mes.FileName)
	if err != nil {
		socket.logger.Error(
			socket.ctx,
			"failed to prep lsp",
			slog.Error(err),
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
			socket.logger.Error(
				socket.ctx,
				"failed to send error payload",
				slog.Error(err),
			)
		}
		return
	}

	// launch a new lsp and store it in the server
	a.lsp.Store(lsp.NewLspServer(context.TODO(), mes.Lang, a.Logger))

	err = wsjson.Write(socket.ctx, socket.ws, payload.PrepPayload(
		msg.SequenceID,
		payload.WebSocketMessageTypeLaunchLspResponse,
		payload.LaunchLspResponsePayload{
			Success: true,
		},
	))
	if err != nil {
		socket.logger.Error(
			socket.ctx,
			"failed to send new exec payload",
			slog.Error(err),
		)
	}
}
