package payload

import "time"

// PrepPayload
//
//	Prepares a payload for sending over the websocket.
func PrepPayload[T any](seqId string, msgType WebSocketMessageType, payload T) WebSocketPayload[T] {
	return WebSocketPayload[T]{
		SequenceID: seqId,
		Type:       msgType,
		Origin:     WebSocketMessageOriginServer,
		CreatedAt:  time.Now().UnixNano() / 1e6,
		Payload:    payload,
	}
}
