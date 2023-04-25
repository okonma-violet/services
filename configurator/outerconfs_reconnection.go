package main

import (
	"context"
	"errors"
	"net"
	"time"

	"github.com/big-larry/suckutils"
	"github.com/okonma-violet/connector"
	"github.com/okonma-violet/services/types/configuratortypes"
	"github.com/okonma-violet/services/types/netprotocol"
)

var reconnectReq chan *service

func initReconnection(ctx context.Context, ticktime time.Duration, targetbufsize int, queuesize int) {
	if targetbufsize == 0 || queuesize == 0 {
		panic("target buffer size / queue size must be > 0")
	}
	if reconnectReq == nil {
		reconnectReq = make(chan *service, queuesize)
	} else {
		panic("reconnection is already initiated")
	}
	go serveReconnects(ctx, ticktime, targetbufsize)
}

// библиотечный реконнектор использовать нельзя, ибо он при одновременном реконнекте с двух сторон нас либо мягко задедлочит, либо получатся два разных но работающих подключения (шо весело, конечно)
// ONLY FOR OTHER CONFIGURATORS RECONNECTION
func serveReconnects(ctx context.Context, ticktime time.Duration, targetbufsize int) {
	buf := make([]*service, 0, targetbufsize)
	ticker := time.NewTicker(ticktime)

	for {
		select {
		case <-ctx.Done():
			return
		case req := <-reconnectReq:
			buf = append(buf, req)
		case <-ticker.C:
			for i := 0; i < len(buf); i++ {
				buf[i].statusmux.Lock()
				if buf[i].status == configuratortypes.StatusOff {
					if buf[i].outerAddr.netw != netprotocol.NetProtocolTcp {
						buf[i].l.Error("Reconnect", errors.New("cant reconnect to non-tcp address"))
						continue
					}
					conn, err := net.Dial(buf[i].outerAddr.netw.String(), suckutils.ConcatThree(buf[i].outerAddr.remotehost, ":", buf[i].outerAddr.port))
					if err != nil {
						buf[i].statusmux.Unlock()
						buf[i].l.Error("Reconnect/Dial", err)
						continue
					}

					if err = handshake(conn); err != nil {
						buf[i].statusmux.Unlock()
						buf[i].l.Error("Reconnect/handshake", err)
						continue
					}

					newcon, err := connector.NewEpollConnector[connector.BasicMessage](conn, buf[i])
					if err != nil {
						buf[i].statusmux.Unlock()
						buf[i].l.Error("Reconnect/NewEpollConnector", err)
						continue
					}

					if err = newcon.StartServing(); err != nil {
						newcon.ClearFromCache()
						buf[i].statusmux.Unlock()
						buf[i].l.Error("Reconnect/StartServing", err)
						continue
					}
					buf[i].connector = newcon

					if err = sendUpdateToOuterConf(buf[i]); err != nil {
						buf[i].statusmux.Unlock()
						buf[i].l.Error("sendUpdateToOuterConf", err)
						buf[i].connector.Close(err)
						continue
					}
				}
				buf[i].statusmux.Unlock()
				buf = append(buf[:i], buf[i+1:]...) // трем из буфера

				i--
			}
			if cap(buf) > targetbufsize && len(buf) <= targetbufsize { // при переполнении буфера снова его уменьшаем, если к этому моменту разберемся с реконнектами
				newbuf := make([]*service, len(buf), targetbufsize)
				copy(newbuf, buf)
				buf = newbuf
			}
		}
	}
}

func handshake(conn net.Conn) error {
	if _, err := conn.Write(connector.FormatBasicMessage([]byte(configuratortypes.ConfServiceName))); err != nil {
		return err
	}
	buf := make([]byte, 5)
	conn.SetReadDeadline(time.Now().Add(time.Second * 5))
	_, err := conn.Read(buf)
	if err != nil {
		return errors.New(suckutils.ConcatTwo("err reading configurator's approving, err: ", err.Error()))
	}
	if buf[4] == byte(configuratortypes.OperationCodeOK) {
		return nil
	} else {
		return errors.New("service's approving format not supported or weird")
	}
}
