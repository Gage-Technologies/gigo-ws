package server

import (
	"encoding/json"
	"gigo-ws/coder/agent/agent/server/core"
	"gigo-ws/coder/agent/agent/server/payload"

	"cdr.dev/slog"
	"nhooyr.io/websocket/wsjson"
)

func (a *HttpApi) LintCode(socket *masterWebSocket, msg *payload.WebSocketPayload[any]) {
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
					"payload": "payload is not of type LintRequestPayload",
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

	var mes payload.LintRequestPayload

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
					"payload": "payload is not of type LintRequestPayload",
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

	payloadRes, err := core.LintCode(socket.ctx, mes.Code, mes.Lang, a.Logger)
	if err != nil {
		socket.logger.Error(
			socket.ctx,
			"failed to lint requested code",
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

	// return the chat payload to the client
	err = wsjson.Write(socket.ctx, socket.ws, payload.PrepPayload(
		msg.SequenceID,
		payload.WebSocketMessageTypeLintResponse,
		payloadRes,
	))
	if err != nil {
		a.Logger.Error(
			socket.ctx,
			"failed to send new chat payload",
			slog.Error(err),
		)
	}
}
