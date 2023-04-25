package main

import (
	"io"
	"project/wsconnector"

	"github.com/gobwas/ws"
)

type message struct {
}

// wsservice.Handler {wsconnector.WsHandler} interface implementation
func (*wsconn) NewMessage() wsconnector.MessageReader {
	return &message{}
}

// wsservice.Handler {wsconnector.WsHandler {wsconnector.MessageReader}} interface implementation
func (m *message) Read(r io.Reader, h ws.Header) error {
	//return wsconnector.ReadAndDecodeJson(r, m)

	return nil
}
