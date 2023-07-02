package signalr

import (
	"context"
	"io"
)

// Connection describes a connection between signalR client and server
type Connection interface {
	io.Reader
	io.Writer
	Context() context.Context
	ConnectionID() string
	UserId() uint64
	SetUserId(id uint64)
	Request()
	SetConnectionID(id string)
	Information() ConnectionInfo
}

// TransferMode is either TextTransferMode or BinaryTransferMode
type TransferMode int

// MessageType constants.
const (
	// TextTransferMode is for UTF-8 encoded text messages like JSON.
	TextTransferMode TransferMode = iota + 1
	// BinaryTransferMode is for binary messages like MessagePack.
	BinaryTransferMode
)

// ConnectionWithTransferMode is a Connection with TransferMode (e.g. Websocket)
type ConnectionWithTransferMode interface {
	TransferMode() TransferMode
	SetTransferMode(transferMode TransferMode)
}

type ConnectionInfo struct {
	UserId       string `json:"userId"`
	OnlineTime   string `json:"onlineTime"`
	AuthTime     string `json:"authTime"`
	ConnectionId string `json:"connectionId"`
	RemoteAddr   string `json:"remoteAddr"`
	EndpointType string `json:"endPointType"`
	RequestCount string `json:"requestCount"`
}
