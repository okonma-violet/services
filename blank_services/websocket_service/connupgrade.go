package main

import "project/wsconnector"

// wsservice.Handler {wsconnector.WsHandler {wsconnector.UpgradeReqChecker}} interface implementation
func (wsc *wsconn) CheckPath(path []byte) wsconnector.StatusCode {

	return 200
}

// wsservice.Handler {wsconnector.WsHandler {wsconnector.UpgradeReqChecker}} interface implementation
func (wsc *wsconn) CheckHost(host []byte) wsconnector.StatusCode {

	return 200
}

// will be called on every header, that is not used in websocket handshake procedure.
// wsservice.Handler {wsconnector.WsHandler {wsconnector.UpgradeReqChecker}} interface implementation
func (wsc *wsconn) CheckHeader(key []byte, value []byte) wsconnector.StatusCode {

	return 200
}

// will be called before sending successful upgrade response.
// wsservice.Handler {wsconnector.WsHandler {wsconnector.UpgradeReqChecker}} interface implementation
func (wsc *wsconn) CheckBeforeUpgrade() wsconnector.StatusCode {

	return 200
}
