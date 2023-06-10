package connectornonepoll

import (
	"context"
	"errors"
	"net"
	"sync"
	"time"
)

type NonEpollReConnector[Tm any, PTm interface {
	Readable
	*Tm
}] struct {
	connector  *NonEpollConnector[Tm, PTm]
	msghandler MessageHandler[PTm]

	mux       sync.Mutex
	isstopped bool

	dialTimeout      time.Duration
	reconnectTimeout time.Duration

	reconAddr Addr

	doOnDial         func(net.Conn) error // right after dial, before NewEpollConnector() call
	doAfterReconnect func() error         // after StartServing() call
}

type Addr struct {
	netw    string
	address string
}

func NewNonEpollReConnector[Tmessage any, PTmessage interface {
	Readable
	*Tmessage
}, Thandler MessageHandler[PTmessage]](conn net.Conn, netw, addr string, messagehandler Thandler, dialTimeout time.Duration, reconnectTimeout time.Duration, doOnDial func(net.Conn) error, doAfterReconnect func() error) (*NonEpollReConnector[Tmessage, PTmessage], error) {
	reconn := &NonEpollReConnector[Tmessage, PTmessage]{
		msghandler: messagehandler,

		doOnDial:         doOnDial,
		doAfterReconnect: doAfterReconnect,

		dialTimeout:      dialTimeout,
		reconnectTimeout: reconnectTimeout,
	}

	if netw != "" && addr != "" {
		reconn.reconAddr = Addr{netw: netw, address: addr}
	} else if conn != nil {
		reconn.reconAddr = Addr{netw: conn.RemoteAddr().Network(), address: conn.RemoteAddr().String()}
	} else {
		return nil, errors.New("netw & addr not specified and conn is nil")
	}

	if conn != nil {
		reconn.connector, _ = NewNonEpollConnector[Tmessage, PTmessage](conn, reconn)
	}

	return reconn, nil
}

// run inside a routine
func (recon *NonEpollReConnector[Tm, PTm]) Serve(ctx context.Context, readTimeout time.Duration) error {
	dialer := &net.Dialer{Timeout: recon.dialTimeout}

	for {
		select {
		case <-ctx.Done():
			recon.mux.Lock()
			recon.isstopped = true //??????????????
			recon.mux.Unlock()
			return ErrContextDone
		default:
			recon.mux.Lock()
			if !recon.isstopped {
				if recon.connector == nil || recon.connector.IsClosed() {
					conn, err := dialer.Dial(recon.reconAddr.netw, recon.reconAddr.address)
					if err != nil {
						recon.mux.Unlock()
						time.Sleep(recon.reconnectTimeout)
						continue // не логается
					}
					if recon.doOnDial != nil {
						if err := recon.doOnDial(conn); err != nil {
							conn.Close()
							recon.mux.Unlock()
							continue
						}
					}

					newcon, err := NewNonEpollConnector[Tm, PTm](conn, recon)
					if err != nil {
						conn.Close()
						recon.mux.Unlock()
						continue // не логается
					}

					recon.connector = newcon

					if recon.doAfterReconnect != nil {
						if err := recon.doAfterReconnect(); err != nil {
							recon.connector.Close(err)
							recon.mux.Unlock()
							continue
						}
					}
				}
				recon.mux.Unlock()
				recon.connector.Serve(ctx, readTimeout)
				time.Sleep(recon.reconnectTimeout) // чтобы не было долбежки реконнектами успешными
			}
		}
	}
}

func (a Addr) Network() string {
	return a.netw
}
func (a Addr) String() string {
	return a.address
}

func (reconnector *NonEpollReConnector[_, PTm]) Handle(message PTm) error {
	return reconnector.msghandler.Handle(message)
}
func (reconnector *NonEpollReConnector[_, _]) HandleClose(reason error) {
	reconnector.msghandler.HandleClose(reason)
}
func (reconnector *NonEpollReConnector[_, _]) Send(message []byte) error {
	return reconnector.connector.Send(message)
}

// doesn't stop reconnection
func (reconnector *NonEpollReConnector[_, _]) Close(reason error) {
	reconnector.mux.Lock()
	defer reconnector.mux.Unlock()
	if reconnector.connector != nil {
		reconnector.connector.Close(reason)
	}
}

// call in HandleClose() will cause deadlock
func (reconnector *NonEpollReConnector[_, _]) IsClosed() bool {
	if reconnector.connector != nil {
		return reconnector.connector.IsClosed()
	}
	return true
}
func (reconnector *NonEpollReConnector[_, _]) RemoteAddr() net.Addr {
	return reconnector.reconAddr
}
func (reconnector *NonEpollReConnector[_, _]) IsReconnectStopped() bool { // только извне! иначе потенциальная блокировка
	reconnector.mux.Lock()
	defer reconnector.mux.Unlock()
	return reconnector.isstopped
}

// DOES NOT CLOSE CONN
func (reconnector *NonEpollReConnector[_, _]) CancelReconnect() { // только извне! иначе потенциальная блокировка
	reconnector.mux.Lock()
	defer reconnector.mux.Unlock()
	reconnector.isstopped = true
}
