package httpservicenonepoll

import (
	"context"
	"errors"
	"net"

	"strings"
	"time"

	"github.com/big-larry/suckutils"

	connectornonepoll "github.com/okonma-violet/services/connector_nonepoll"
	"github.com/okonma-violet/services/logs/logger"
	"github.com/okonma-violet/services/types/configuratortypes"
	"github.com/okonma-violet/services/types/netprotocol"
)

type configurator struct {
	conn            *connectornonepoll.NonEpollReConnector[connectornonepoll.BasicMessage, *connectornonepoll.BasicMessage]
	thisServiceName ServiceName

	publishers *publishers
	listener   *listener
	servStatus *serviceStatus

	terminationByConfigurator chan struct{}
	l                         logger.Logger
}

const conf_ReadTimeout time.Duration = time.Second * 5
const conf_DialTimeout time.Duration = time.Second * 2
const conf_ReconnectTimeout time.Duration = time.Millisecond * 500

// func newFakeConfigurator(ctx context.Context, l logger.Logger, servStatus *serviceStatus, listener *listener) *configurator {
// 	connector.InitReconnection(ctx, time.Second*5, 1, 1)
// 	c := &configurator{
// 		l:          l,
// 		servStatus: servStatus,
// 		listener:   listener,
// 	}

// 	ln, err := net.Listen("tcp", "127.0.0.1:0")
// 	if err != nil {
// 		c.l.Error("newFakeConfigurator", err)
// 		return nil
// 	}
// 	go func() {
// 		if _, err := ln.Accept(); err != nil {
// 			panic("newFakeConfigurator/ln.Accept err:" + err.Error())
// 		}
// 	}()
// 	conn, err := net.Dial("tcp", ln.Addr().String())
// 	if err != nil {
// 		c.l.Error("newFakeConfigurator/Dial", err)
// 		return nil
// 	}

// 	if c.conn, err = connector.NewEpollReConnector(conn, c, nil, nil); err != nil {
// 		c.l.Error("NewEpollReConnector", err)
// 		return nil
// 	}

// 	go func() {
// 		p := 9010
// 		for {
// 			time.Sleep(time.Millisecond * 50)
// 			addr := configuratortypes.FormatAddress(netprotocol.NetProtocolTcp, "127.0.0.1:"+strconv.Itoa(p+1))
// 			if err := c.Handle(&connector.BasicMessage{Payload: append(append(make([]byte, 0, len(addr)+2), byte(configuratortypes.OperationCodeSetOutsideAddr), byte(len(addr))), addr...)}); err != nil {
// 				c.l.Error("FakeConfiguratorsMessageHandle", err)
// 				p++
// 				continue
// 			}
// 			if c.servStatus.isListenerOK() {
// 				return
// 			}

// 		}
// 	}()
// 	return c
// }

func newConfigurator(ctx context.Context, l logger.Logger, servStatus *serviceStatus, pubs *publishers, listener *listener, configuratoraddr string, thisServiceName ServiceName, reconnectTimeout time.Duration) *configurator {

	c := &configurator{
		thisServiceName:           thisServiceName,
		l:                         l,
		servStatus:                servStatus,
		publishers:                pubs,
		listener:                  listener,
		terminationByConfigurator: make(chan struct{}, 1)}

	go func() {
		var err error
		c.conn, err = connectornonepoll.NewNonEpollReConnector[connectornonepoll.BasicMessage](nil, (configuratoraddr)[:strings.Index(configuratoraddr, ":")], (configuratoraddr)[strings.Index(configuratoraddr, ":")+1:],
			c, conf_DialTimeout, conf_ReconnectTimeout, c.handshake, c.afterConnProc)
		if err != nil {
			panic(err)
		}
		if err = c.conn.Serve(ctx, conf_ReadTimeout); err != nil {
			l.Warning("Conn", suckutils.ConcatTwo("serving exited, reason: ", err.Error()))
		}
	}()

	return c
}

func (c *configurator) handshake(conn net.Conn) error {
	if _, err := conn.Write(connectornonepoll.FormatBasicMessage([]byte(c.thisServiceName))); err != nil {
		c.l.Error("Conn/handshake", errors.New(suckutils.ConcatTwo("Write() err: ", err.Error())))
		return err
	}
	buf := make([]byte, 5)
	conn.SetReadDeadline(time.Now().Add(time.Second * 2))
	n, err := conn.Read(buf)
	if err != nil {
		c.l.Error("Conn/handshake", errors.New(suckutils.ConcatTwo("err reading configurator's approving, err: ", err.Error())))
		return errors.New(suckutils.ConcatTwo("err reading configurator's approving, err: ", err.Error()))
	}
	if n == 5 {
		if buf[4] == byte(configuratortypes.OperationCodeOK) {
			c.l.Debug("Conn/handshake", "succesfully (re)connected and handshaked to "+conn.RemoteAddr().String())
			return nil
		} else if buf[4] == byte(configuratortypes.OperationCodeNOTOK) { // "PANIC"??
			c.l.Error("Conn/handshake", errors.New("configurator do not approve this service"))
			if c.conn != nil {
				c.l.Warning("Conn/handshake", ("cancelling reconnect, OperationCodeNOTOK recieved"))
				go c.conn.CancelReconnect() // горутина пушто этот хэндшейк под залоченным мьютексом выполняется
			}
			c.terminationByConfigurator <- struct{}{}
			return errors.New("configurator do not approve this service")
		}
	}
	c.l.Error("Conn/handshake", errors.New("configurator's approving format not supported or weird"))
	return errors.New("configurator's approving format not supported or weird")
}

func (c *configurator) afterConnProc() error {
	myStatus := byte(configuratortypes.StatusSuspended)
	if c.servStatus.onAir() {
		myStatus = byte(configuratortypes.StatusOn)
	}
	if err := c.conn.Send(connectornonepoll.FormatBasicMessage([]byte{byte(configuratortypes.OperationCodeMyStatusChanged), myStatus})); err != nil {
		c.l.Error("Conn/afterConnProc", errors.New(suckutils.ConcatTwo("Send() err: ", err.Error())))
		return err
	}

	if c.publishers != nil {
		pubnames := c.publishers.GetAllPubNames()
		if len(pubnames) != 0 {
			message := append(make([]byte, 0, len(pubnames)*15), byte(configuratortypes.OperationCodeSubscribeToServices))
			for _, pub_name := range pubnames {
				pub_name_byte := []byte(pub_name)
				message = append(append(message, byte(len(pub_name_byte))), pub_name_byte...)
			}
			if err := c.conn.Send(connectornonepoll.FormatBasicMessage(message)); err != nil {
				c.l.Error("Conn/afterConnProc", errors.New(suckutils.ConcatTwo("Send() err: ", err.Error())))
				return err
			}
		}
	}

	if err := c.conn.Send(connectornonepoll.FormatBasicMessage([]byte{byte(configuratortypes.OperationCodeGiveMeOuterAddr)})); err != nil {
		c.l.Error("Conn/afterConnProc", errors.New(suckutils.ConcatTwo("Send() err: ", err.Error())))
		return err
	}
	c.l.Debug("Conn/afterConnProc", "succesfully sended after(re)connect things")
	return nil
}

func (c *configurator) send(message []byte) error {
	if c == nil {
		return errors.New("nil configurator")
	}
	if c.conn == nil {
		return connectornonepoll.ErrNilConn
	}
	if c.conn.IsClosed() {
		return connectornonepoll.ErrClosedConnector
	}
	if err := c.conn.Send(message); err != nil {
		c.conn.Close(err)
		return err
	}
	return nil
}

func (c *configurator) onSuspend(reason string) {
	c.l.Warning("OwnStatus", suckutils.ConcatTwo("suspended, reason: ", reason))
	c.send(connectornonepoll.FormatBasicMessage([]byte{byte(configuratortypes.OperationCodeMyStatusChanged), byte(configuratortypes.StatusSuspended)}))
}

func (c *configurator) onUnSuspend() {
	c.l.Warning("OwnStatus", "unsuspended")
	c.send(connectornonepoll.FormatBasicMessage([]byte{byte(configuratortypes.OperationCodeMyStatusChanged), byte(configuratortypes.StatusOn)}))
}

// func (c *configurator) NewMessage() connector.MessageReader {
// 	return connector.NewBasicMessage()
// }

func (c *configurator) Handle(message *connectornonepoll.BasicMessage) error {
	payload := message.Payload
	if len(payload) == 0 {
		return connectornonepoll.ErrEmptyPayload
	}
	switch configuratortypes.OperationCode(payload[0]) {
	case configuratortypes.OperationCodePing:
		return nil
	case configuratortypes.OperationCodeMyStatusChanged:
		return nil
	case configuratortypes.OperationCodeImSupended:
		return nil
	case configuratortypes.OperationCodeSetOutsideAddr:
		if len(payload) < 2 {
			return connectornonepoll.ErrWeirdData
		}
		if len(payload) < 2+int(payload[1]) {
			return connectornonepoll.ErrWeirdData
		}
		if netw, addr, err := configuratortypes.UnformatAddress(payload[2 : 2+int(payload[1])]); err != nil {
			return err
		} else {
			if netw == netprotocol.NetProtocolNil {
				c.listener.stop()
				c.servStatus.setListenerStatus(true)
				return nil
			}
			if netw == netprotocol.NetProtocolTcp {
				if cur_netw, cur_addr := c.listener.Addr(); cur_addr[strings.LastIndex(cur_addr, ":")+1:] == addr && cur_netw == netw.String() {
					return nil
				}
				addr = suckutils.ConcatTwo(":", addr)
			} else if cur_netw, cur_addr := c.listener.Addr(); cur_addr == addr && cur_netw == netw.String() {
				return nil
			}
			var err error
			for i := 0; i < 3; i++ {
				if err = c.listener.listen(netw.String(), addr); err != nil {
					c.listener.l.Error("listen", err)
					time.Sleep(time.Second)
				} else {
					return nil
				}
			}
			return err
		}
	case configuratortypes.OperationCodeUpdatePubs:
		updates := configuratortypes.SeparatePayload(payload[1:])
		if len(updates) != 0 {
			for _, update := range updates {
				pubname, raw_addr, status, err := configuratortypes.UnformatOpcodeUpdatePubMessage(update)
				if err != nil {
					return err
				}
				netw, addr, err := configuratortypes.UnformatAddress(raw_addr)
				if err != nil {
					c.l.Error("Handle/OperationCodeUpdatePubs/UnformatAddress", err)
					return connectornonepoll.ErrWeirdData
				}
				if netw == netprotocol.NetProtocolNonlocalUnix {
					continue // TODO:
				}

				c.publishers.update(ServiceName(pubname), netw.String(), addr, status)
			}
			return nil
		} else {
			return connectornonepoll.ErrWeirdData
		}
	}
	return connectornonepoll.ErrWeirdData
}

func (c *configurator) HandleClose(reason error) {
	if reason != nil {
		c.l.Warning("conn", suckutils.ConcatTwo("conn closed, reason err: ", reason.Error()))
	} else {
		c.l.Warning("conn", "conn closed, no reason specified")
	}

	// в суспенд не уходим, пока у нас есть паблишеры - нам пофиг
}
