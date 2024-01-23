package server

import (
	"encoding/json"
	"fmt"
	"gigo-ws/coder/agent/agent/server/core"
	"gigo-ws/coder/agent/agent/server/payload"
	"strconv"

	"cdr.dev/slog"
	"nhooyr.io/websocket/wsjson"
)

func (a *HttpApi) ExecCode(socket *masterWebSocket, msg *payload.WebSocketPayload[any]) {
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
			a.Logger.Error(
				socket.ctx,
				"failed to send validation error payload",
				slog.Error(err),
			)
		}
	}

	var mes payload.ExecRequestPayload

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
			a.Logger.Error(
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

	if mes.Code == "" {
		// return the error payload to the client
		err := wsjson.Write(socket.ctx, socket.ws, payload.PrepPayload(
			msg.SequenceID,
			payload.WebSocketMessageTypeGenericError,
			payload.GenericErrorPayload{
				Error: "no code received",
			},
		))
		if err != nil {
			a.Logger.Error(
				socket.ctx,
				"no code received",
				slog.Error(err),
			)
		}
		return
	}

	activeCommand, err := core.ExecCode(socket.ctx, mes.Code, mes.Lang, a.Logger)
	if err != nil {
		socket.logger.Error(
			socket.ctx,
			"failed to execute requested code",
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
			a.Logger.Error(
				socket.ctx,
				"failed to send error payload",
				slog.Error(err),
			)
		}
		return
	}

	// create a new id for this command
	commandID := a.Snowflake.Generate().Int64()

	// save the active command to the websocket connection
	socket.activeCommands.Store(commandID, activeCommand)

	// defer the removal of the command
	defer func() {
		socket.activeCommands.Delete(commandID)
	}()

	// return the chat payload to the client
	for r := range activeCommand.ResponseChan {
		// update every response with the command id
		r.CommandID = commandID
		r.CommandIDString = fmt.Sprintf("%d", commandID)

		err = wsjson.Write(socket.ctx, socket.ws, payload.PrepPayload(
			msg.SequenceID,
			payload.WebSocketMessageTypeExecResponse,
			r,
		))
		if err != nil {
			a.Logger.Error(
				socket.ctx,
				"failed to send new exec payload",
				slog.Error(err),
			)
		}
	}
}

func (a *HttpApi) CancelExec(socket *masterWebSocket, msg *payload.WebSocketPayload[any]) {
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
			a.Logger.Error(
				socket.ctx,
				"failed to send validation error payload",
				slog.Error(err),
			)
		}
	}

	var mes payload.CancelExecRequestPayload

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
			a.Logger.Error(
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

	// parse command id
	commandID, _ := strconv.ParseInt(mes.CommandID, 10, 64)

	// save the active command to the websocket connection
	command, ok := socket.activeCommands.Load(commandID)
	if !ok {
		a.Logger.Warn(socket.ctx, "received command cancellation request for non-existant command", slog.F("command_id", commandID))
		err = wsjson.Write(socket.ctx, socket.ws, payload.PrepPayload(
			msg.SequenceID,
			payload.WebSocketMessageTypeCancelExecResponse,
			payload.CancelExecResponsePayload{
				CommandID: mes.CommandID,
			},
		))
		if err != nil {
			a.Logger.Error(
				socket.ctx,
				"failed to send cancel exec response",
				slog.Error(err),
			)
		}
		return
	}

	command.(*core.ActiveCommand).Cancel()

	err = wsjson.Write(socket.ctx, socket.ws, payload.PrepPayload(
		msg.SequenceID,
		payload.WebSocketMessageTypeCancelExecResponse,
		payload.CancelExecResponsePayload{
			CommandID: mes.CommandID,
		},
	))
	if err != nil {
		a.Logger.Error(
			socket.ctx,
			"failed to send cancel exec response",
			slog.Error(err),
		)
	}
}

func (a *HttpApi) StdinExec(socket *masterWebSocket, msg *payload.WebSocketPayload[any]) {
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
			a.Logger.Error(
				socket.ctx,
				"failed to send validation error payload",
				slog.Error(err),
			)
		}
	}

	var mes payload.StdinExecRequestPayload

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
			a.Logger.Error(
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

	// parse command id
	commandID, _ := strconv.ParseInt(mes.CommandID, 10, 64)

	// save the active command to the websocket connection
	command, ok := socket.activeCommands.Load(commandID)
	if !ok {
		a.Logger.Warn(socket.ctx, "received stdin request for non-existant command", slog.F("command_id", commandID))
		err = wsjson.Write(socket.ctx, socket.ws, payload.PrepPayload(
			msg.SequenceID,
			payload.WebSocketMessageTypeStdinExecResponse,
			payload.StdinExecResponsePayload{
				CommandID: mes.CommandID,
			},
		))
		if err != nil {
			a.Logger.Error(
				socket.ctx,
				"failed to send stdin exec response",
				slog.Error(err),
			)
		}
		return
	}

	_, err = command.(*core.ActiveCommand).Stdin.Write([]byte(mes.Input))
	if err != nil {
		a.Logger.Error(socket.ctx, "failed to write stdin to command", slog.Error(err))
		err = wsjson.Write(socket.ctx, socket.ws, payload.PrepPayload(
			msg.SequenceID,
			payload.WebSocketMessageTypeGenericError,
			payload.GenericErrorPayload{
				Error: "failed to write input",
				Code:  payload.WebSocketErrorCodeServerError,
			},
		))
		if err != nil {
			a.Logger.Error(
				socket.ctx,
				"failed to send stdin exec response",
				slog.Error(err),
			)
		}
	}

	err = wsjson.Write(socket.ctx, socket.ws, payload.PrepPayload(
		msg.SequenceID,
		payload.WebSocketMessageTypeStdinExecResponse,
		payload.StdinExecResponsePayload{
			CommandID: mes.CommandID,
		},
	))
	if err != nil {
		a.Logger.Error(
			socket.ctx,
			"failed to send stdin exec response",
			slog.Error(err),
		)
	}
}
