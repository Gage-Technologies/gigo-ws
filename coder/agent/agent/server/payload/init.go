package payload

// InitPayload
//
//	Payload returned to the client by the server on websocket upgrade
//	to initialize the connection.
type InitPayload struct {
	// Epoch
	//
	//  Initialization time of the websocket connection in unix millis.
	Epoch int64 `json:"epoch"`

	// NodeID
	//
	//  The ID of the node that the client is connected to.
	NodeID string `json:"node_id"`

	// Admin
	//
	// Whether the user is an administrator.
	Admin bool `json:"admin"`
}
