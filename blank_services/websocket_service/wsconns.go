package main

import (
	"project/wsconnector"
)

type wsconn struct {
}

// wsservice.Handler interface implementation
func (wsc *wsconn) HandleWSCreating(sender wsconnector.Sender) error {

	return nil
}

// wsservice.Handler {wsconnector.WsHandler} interface implementation
func (wsc *wsconn) Handle(msg wsconnector.MessageReader) error {

	return nil
}

// wsservice.Handler {wsconnector.WsHandler} interface implementation
func (wsc *wsconn) HandleClose(err error) {

}
