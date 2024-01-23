package payload

// WebSocketMessageType
//
//	The type of web socket message.
type WebSocketMessageType int

const (
	WebSocketMessageTypeInit WebSocketMessageType = iota
	WebSocketMessageTypeValidationError
	WebSocketMessageTypeGenericError
	WebSocketMessageTypeExecRequest
	WebSocketMessageTypeExecResponse
	WebSocketMessageTypeLintRequest
	WebSocketMessageTypeLintResponse
	WebSocketMessageTypeCancelExecRequest
	WebSocketMessageTypeCancelExecResponse
	WebSocketMessageTypeStdinExecRequest
	WebSocketMessageTypeStdinExecResponse
)

func (t WebSocketMessageType) String() string {
	return [...]string{
		"WebSocketMessageTypeInit",
		"WebSocketMessageTypeValidationError",
		"WebSocketMessageTypeGenericError",
		"WebSocketMessageTypeExecRequest",
		"WebSocketMessageTypeExecResponse",
		"WebSocketMessageTypeLintRequest",
		"WebSocketMessageTypeLintResponse",
		"WebSocketMessageTypeCancelExecRequest",
		"WebSocketMessageTypeCancelExecResponse",
		"WebSocketMessageTypeStdinExecRequest",
		"WebSocketMessageTypeStdinExecResponse",
	}[t]
}

// WebSocketMessageOrigin
//
//	The origin of a web socket message.
type WebSocketMessageOrigin int

const (
	WebSocketMessageOriginServer WebSocketMessageOrigin = iota
	WebSocketMessageOriginClient
)

// WebSocketPayload
//
//	Top level payload for web socket messages.
type WebSocketPayload[T any] struct {
	// SequenceID
	//
	//  The ID of the message sequence. This is set by the initiator on the first call
	//  for a sequence of messages. The field is simply echoed in all messages in the
	//  sequence to make correlating messages on the client easier.
	SequenceID string `json:"sequence_id"`

	// Type
	//
	//  The type of payload.
	Type WebSocketMessageType `json:"type" validate:"required,gte=0,lte=10"`

	// Origin
	//
	//  The origin of the message.
	Origin WebSocketMessageOrigin `json:"origin" validate:"required,gte=0,lte=1"`

	// CreatedAt
	//
	//  The time the message was created in unix milliseconds.
	CreatedAt int64 `json:"created_at" validate:"required,gt=0"`

	// Test
	//
	//  Marks whether the message is a test message.
	Test bool `json:"test"`

	// Payload
	//
	//  The actual payload of the message.
	Payload T `json:"payload" validate:"required"`
}
