package connectornonepoll

import (
	"context"
	"errors"
	"io"
	"net"
	"os"
	"sync"
	"time"
)

type NonEpollConnector[Tm any, PTm interface {
	Readable
	*Tm
}] struct {
	conn       net.Conn
	msghandler MessageHandler[PTm]
	mux        sync.Mutex
	isclosed   bool
}

func NewNonEpollConnector[Tmessage any,
	PTmessage interface {
		Readable
		*Tmessage
	}, Th MessageHandler[PTmessage]](conn net.Conn, messagehandler Th) (*NonEpollConnector[Tmessage, PTmessage], error) {
	if conn == nil {
		return nil, ErrNilConn
	}

	connector := &NonEpollConnector[Tmessage, PTmessage]{conn: conn, msghandler: messagehandler}

	return connector, nil
}

// run inside a routine / readTimeout must not be zero / readTimeout shots -> checks ctx / closes conn on return
func (connector *NonEpollConnector[Tm, PTm]) Serve(ctx context.Context, readTimeout time.Duration) error {
	if connector.IsClosed() {
		return ErrClosedConnector
	}

	if readTimeout == 0 {
		return errors.New("zero readTimeout")
	}

	var err error
	defer func() { // иначе ошибка nil-овая в Close(err) будет
		connector.Close(err)
	}()

	for {
		select {
		case <-ctx.Done():
			return ErrContextDone
		default:
			message := PTm(new(Tm))
			connector.conn.SetReadDeadline(time.Now().Add(readTimeout))
			if err = message.ReadWithoutDeadline(connector.conn); err != nil {
				if errors.Is(err, os.ErrDeadlineExceeded) {
					continue
				}
				if errors.Is(err, io.EOF) || errors.Is(err, io.ErrUnexpectedEOF) {
					err = ErrConnClosedByOtherSide
					return err
				}
				return err
			}
			if err = connector.msghandler.Handle(message); err != nil {
				return err
			}
		}
	}
}

func (connector *NonEpollConnector[_, _]) Send(message []byte) error {

	if connector.IsClosed() {
		return ErrClosedConnector
	}
	//connector.conn.SetWriteDeadline(time.Now().Add(time.Second))
	_, err := connector.conn.Write(message)
	return err
}

func (connector *NonEpollConnector[_, _]) Close(reason error) {
	connector.mux.Lock()
	defer connector.mux.Unlock()

	if connector.isclosed {
		return
	}
	connector.isclosed = true
	connector.conn.Close()
	connector.msghandler.HandleClose(reason)
}

// call in HandleClose() will cause deadlock
func (connector *NonEpollConnector[_, _]) IsClosed() bool {
	connector.mux.Lock()
	defer connector.mux.Unlock()
	return connector.isclosed
}

func (connector *NonEpollConnector[_, _]) RemoteAddr() net.Addr {
	return connector.conn.RemoteAddr()
}
