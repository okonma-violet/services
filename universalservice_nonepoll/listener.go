package universalservice_nonepoll

import (
	"context"
	"errors"
	"net"
	"os"

	"sync"
	"time"

	"github.com/big-larry/suckutils"
	"github.com/okonma-violet/dynamicworkerspool"
	"github.com/okonma-violet/services/logs/logger"
)

type listener struct {
	ln      net.Listener
	handler BaseHandleFunc

	pool *dynamicworkerspool.Pool

	servStatus *serviceStatus
	l          logger.Logger

	rootctx      context.Context
	subctxcancel context.CancelFunc
	accepting    chan struct{}

	sync.Mutex
}

const handlerCallTimeout time.Duration = time.Second * 5
const handlerCallMaxExceededTimeouts = 3
const threadKillingTimeout time.Duration = time.Second * 3

func newListener(ctx context.Context, l logger.Logger, servStatus *serviceStatus, threads int, handler BaseHandleFunc) *listener {
	if threads < 1 {
		panic("threads num cant be less than 1")
	}
	lstnr := &listener{
		accepting:  make(chan struct{}, 1),
		rootctx:    ctx,
		handler:    handler,
		servStatus: servStatus,
		l:          l,
	}
	if threads > 1 {
		lstnr.pool = dynamicworkerspool.NewPool((threads/2)+1, threads, threadKillingTimeout)
	}
	return lstnr
}

// TODO: я пока не придумал шо делать, если поднять листнер не удалось и мы ушли в суспенд (сейчас мы тупо не выйдем из суспенда)
func (listener *listener) listen(network, address string) error {
	if listener == nil {
		panic("listener.listen() called on nil listener")
	}
	listener.Lock()
	defer listener.Unlock()

	if listener.ln != nil {
		if listener.ln.Addr().String() == address {
			return nil
		} else {
			if listener.subctxcancel != nil {
				listener.subctxcancel()
			}
		}
	}
	listener.accepting <- struct{}{}

	var subctx context.Context
	subctx, listener.subctxcancel = context.WithCancel(listener.rootctx)

	var err error
	if network == "unix" {
		if err = os.RemoveAll(address); err != nil {
			goto failure
		}
	}
	listener.ln, err = net.Listen(network, address)
	if err != nil {
		goto failure
	}

	go listener.acceptWorker(subctx)
	listener.servStatus.setListenerStatus(true)
	listener.l.Info("listener", suckutils.ConcatTwo("start listening at ", listener.ln.Addr().String()))
	return nil
failure:
	listener.servStatus.setListenerStatus(false)
	listener.l.Error("listener", errors.New(suckutils.Concat("unable to listen to ", network, ":", address, " error: ", err.Error())))
	listener.subctxcancel()
	listener.ln = nil
	return err
}

func (listener *listener) acceptWorker(ctx context.Context) {

	connsToHandle := make(chan net.Conn)
	go listener.handlingWorker(connsToHandle)

	timer := time.NewTimer(handlerCallTimeout)
	var err error
	var conn net.Conn
loop:
	for {
		select {
		case <-ctx.Done():
			listener.l.Debug("acceptWorker", "context done, returning")
			listener.ln.Close()
			listener.ln = nil
			close(connsToHandle)
			timer.Stop()
			break loop
		default:
			if err != nil {
				listener.l.Error("acceptWorker/Accept", err)
			}
			conn, err = listener.ln.Accept()
			if err != nil {
				continue loop
			}
			if !listener.servStatus.onAir() {
				listener.l.Warning("acceptWorker", suckutils.ConcatTwo("service suspended, discard conn from ", conn.RemoteAddr().String()))
				conn.Close()
				continue loop
			}
			for {
				var i time.Duration
				if !timer.Stop() {
					<-timer.C
				}
				timer.Reset(handlerCallTimeout)
				select {
				case <-timer.C:
					if i += 1; i > handlerCallMaxExceededTimeouts {
						listener.l.Warning("acceptWorker", suckutils.ConcatThree("exceeded max timeout, no free handlingWorker available for ", (handlerCallTimeout*i).String(), ", close connection"))
						conn.Close()
						continue loop
					}
					listener.l.Warning("acceptWorker", suckutils.ConcatTwo("exceeded timeout, no free handlingWorker available for ", (handlerCallTimeout*i).String()))
				case connsToHandle <- conn:
					continue loop
				}
			}
		}
	}
	<-listener.accepting
}

func (listener *listener) handlingWorker(connsToHandle chan net.Conn) {
	ll := listener.l.NewSubLogger("Handle")
	for conn := range connsToHandle {
		listener.l.Debug("handlingWorker", "new request from "+conn.RemoteAddr().String())

		if listener.pool != nil {
			listener.pool.Schedule(func() {
				if err := listener.handler(ll, conn); err != nil {
					listener.l.Error("handlingWorker/handle", errors.New(suckutils.ConcatThree(conn.RemoteAddr().String(), ", err: ", err.Error())))
				}
				conn.Close()
			})
		} else {
			if err := listener.handler(ll, conn); err != nil {
				listener.l.Error("handlingWorker/handle", errors.New(suckutils.ConcatThree(conn.RemoteAddr().String(), ", err: ", err.Error())))
			}
			conn.Close()
		}
	}
}

// calling close() we r closing listener forever (no further listen() calls) and waiting for all reqests to be handled
func (listener *listener) close() {
	listener.Lock()
	defer listener.Unlock()

	if listener.ln != nil {
		listener.ln.Close()
		listener.l.Debug("Close/ln", "succesfully closed")
	}
	if listener.pool != nil {
		listener.pool.Close()
		listener.l.Debug("Close/pool", "closing pool")
		err := listener.pool.DoneWithTimeout(time.Second * 5)
		if err != nil {
			listener.l.Warning("Close/pool", "closed by timeout")
		} else {
			listener.l.Debug("Close/pool", "succesfully closed")
		}
	}
}

func (listener *listener) onAir() bool {
	listener.Lock()
	defer listener.Unlock()
	return listener.ln != nil
}

func (listener *listener) Addr() (string, string) {
	if listener == nil {
		return "", ""
	}
	listener.Lock()
	defer listener.Unlock()
	if listener.ln == nil {
		return "", ""
	}
	return listener.ln.Addr().Network(), listener.ln.Addr().String()
}
