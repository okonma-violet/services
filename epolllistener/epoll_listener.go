package epolllistener

import (
	"errors"
	"net"
	"os"
	"strings"
	"sync"

	"github.com/mailru/easygo/netpoll"
)

var ErrWeirdData error = errors.New("weird data")
var ErrReadTimeout error = errors.New("read timeout")
var ErrAuthorizationDenied error = errors.New("authorization denied")

// for user's implementation
type PoolScheduler interface {
	Schedule(task func())
	//ScheduleWithTimeout(task func(), timeout time.Duration) error
}

// for user's implementation
type ListenerHandler interface {
	HandleNewConn(net.Conn)
	AcceptError(error) // all errs during accept and so on, e.g. auth denied, accept errs
}

// implemented by listener
type Informer interface {
	RemoteAddr() net.Addr
	IsClosed() bool
}

// implemented by listener
type Closer interface {
	Close(error)
}

type EpollListener struct {
	listener        net.Listener
	desc            *netpoll.Desc
	listenerhandler ListenerHandler
	mux             sync.RWMutex
	isclosed        bool
}

func EpollListen(network, address string, listenerhandler ListenerHandler) (*EpollListener, error) {
	if network == "unix" {
		if !strings.HasPrefix(address, "/tmp/") || !strings.HasSuffix(address, ".sock") {
			return nil, errors.New("unix address must be in form \"/tmp/[socketname].sock\"")
		}
		if err := os.RemoveAll(address); err != nil {
			return nil, err
		}
	}
	listener, err := net.Listen(network, address)
	if err != nil {
		return nil, err
	}
	return NewEpollListener(listener, listenerhandler)
}

func NewEpollListener(listener net.Listener, listenerhandler ListenerHandler) (*EpollListener, error) {

	if listener == nil {
		return nil, errors.New("listener is nil")
	}

	desc, err := netpoll.HandleListener(listener, netpoll.EventRead|netpoll.EventOneShot)
	if err != nil {
		return nil, err
	}

	epollln := &EpollListener{listener: listener, desc: desc, listenerhandler: listenerhandler}

	return epollln, nil
}

func (listener *EpollListener) StartServing() error {
	return poller.Start(listener.desc, listener.handle)
}

func (listener *EpollListener) ClearFromCache() {
	listener.mux.Lock()
	defer listener.mux.Unlock()

	listener.isclosed = true
	poller.Stop(listener.desc)
	listener.desc.Close()
	listener.listener.Close()
}

func (listener *EpollListener) handle(e netpoll.Event) {
	defer poller.Resume(listener.desc)

	listener.mux.RLock()

	conn, err := listener.listener.Accept()
	if err != nil {
		listener.listenerhandler.AcceptError(err)
		listener.mux.RUnlock()
		return
	}
	listener.mux.RUnlock()

	if pool != nil {
		pool.Schedule(func() {
			listener.listenerhandler.HandleNewConn(conn)
		})
		return
	}
	listener.listenerhandler.HandleNewConn(conn)
}

func (listener *EpollListener) Close() {
	listener.mux.Lock()
	defer listener.mux.Unlock()

	if listener.isclosed {
		return
	}
	listener.isclosed = true
	poller.Stop(listener.desc)
	listener.desc.Close()
	listener.listener.Close()
}

func (listener *EpollListener) IsClosed() bool {
	listener.mux.RLock()
	defer listener.mux.RUnlock()
	return listener.isclosed
}

func (listener *EpollListener) Addr() net.Addr {
	return listener.listener.Addr()
}
