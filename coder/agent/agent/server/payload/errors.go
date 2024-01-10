package payload

// WebSocketErrorCode
//
//	Error codes returned by the server to the client.
type WebSocketErrorCode int

const (
	WebSocketErrorCodeBadRequest WebSocketErrorCode = iota
	WebSocketErrorCodeServerError
	WebSocketErrorCodeUnauthorized
)

// GenericErrorPayload
//
//	Payload returned to the client by the server if an error occurs.
type GenericErrorPayload struct {
	// Error
	//
	//  The error message.
	Error string `json:"error"`

	// Code
	//
	//  The error code.
	Code WebSocketErrorCode `json:"code"`
}

// ValidationErrorPayload
//
//	Payload returned to the client by the server if a client sent payload
//	fails server-side validation.
type ValidationErrorPayload struct {
	GenericErrorPayload

	// ValidationErrors
	//
	//  A map of field names to error messages.
	ValidationErrors map[string]string `json:"validation_errors"`
}
