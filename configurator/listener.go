package main

import (
	"encoding/binary"
	"errors"
	"net"

	"strings"
	"time"

	"github.com/big-larry/suckutils"
	"github.com/okonma-violet/services/basicmessage"
	"github.com/okonma-violet/services/epolllistener"
	"github.com/okonma-violet/services/logs/logger"
	"github.com/okonma-violet/services/types/configuratortypes"
)

type listener struct {
	ln *epolllistener.EpollListener
}

type listener_info struct {
	allowRemote bool

	subs  subscriptionsier
	servs *services
	l     logger.Logger
}

type listenier interface {
	close()
}

// суть разделения на внешний и локальный листенер - юникс по локалке. а так - конфигуратору сейчас до пизды, если к внешнему листнеру подрубается локальный сервис (и я не особо вижу смысл вешать ограничение)

func newListener(network, address string, allowRemote bool, subs subscriptionsier, servs *services, l logger.Logger) (listenier, error) {

	lninfo := &listener_info{allowRemote: allowRemote, subs: subs, servs: servs, l: l}
	ln, err := epolllistener.EpollListen(network, address, lninfo)
	if err != nil {
		return nil, err
	}
	if err = ln.StartServing(); err != nil {
		ln.ClearFromCache()
		return nil, err
	}
	lninfo.l.Info("Listener", suckutils.ConcatTwo("start listening at ", ln.Addr().String()))
	lstnr := &listener{ln: ln}
	return lstnr, nil
}

// for listener's interface
func (lninfo *listener_info) HandleNewConn(conn net.Conn) {
	lninfo.l.Debug("HandleNewConn", suckutils.ConcatTwo("new conn from ", conn.RemoteAddr().String()))
	var connLocalhosted bool
	if connLocalhosted = isConnLocalhost(conn); !connLocalhosted && !lninfo.allowRemote {
		lninfo.l.Warning("HandleNewConn", suckutils.Concat("new remote conn to local-only listener from: ", conn.RemoteAddr().String(), ", conn denied"))
		conn.Close()
		return
	}

	conn.SetReadDeadline(time.Now().Add(time.Second * 5))
	buf := make([]byte, 4)
	_, err := conn.Read(buf)
	if err != nil {
		lninfo.l.Error("HandleNewConn/Read", err)
		conn.Close()
		return
	}

	buf = make([]byte, binary.LittleEndian.Uint32(buf))
	if _, err = conn.Read(buf); err != nil {
		lninfo.l.Error("HandleNewConn/Read", err)
		conn.Close()
		return
	}
	name := ServiceName(buf)

	state, ok := lninfo.servs.list[name]
	if !ok {
		lninfo.l.Warning("HandleNewConn", suckutils.Concat("unknown service trying to connect: ", string(name)))
		conn.Write(basicmessage.FormatBasicMessage([]byte{byte(configuratortypes.OperationCodeNOTOK)})) // TODO: костыль, переделать
		conn.Close()
		return
	}
	if err := state.initNewConnection(conn, connLocalhosted, name == ServiceName(configuratortypes.ConfServiceName)); err != nil {
		lninfo.l.Error("HandleNewConn/initNewConnection", errors.New(suckutils.ConcatFour("new conn from service \"", string(name), "\" error: ", err.Error())))
		conn.Close()
		return
	}
}

// for listener's interface
func (lninfo *listener_info) AcceptError(err error) {
	lninfo.l.Error("Accept", err)
}

func (ln *listener) close() {
	ln.ln.Close() // ошибки внутри Close() не отслеживаются
}

func isConnLocalhost(conn net.Conn) bool {
	if conn.LocalAddr().Network() == "unix" {
		return true
	}
	if (conn.LocalAddr().String())[:strings.Index(conn.LocalAddr().String(), ":")] == (conn.RemoteAddr().String())[:strings.Index(conn.RemoteAddr().String(), ":")] {
		return true
	}
	return false
}
