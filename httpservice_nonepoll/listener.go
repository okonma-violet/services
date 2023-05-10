package httpservicenonepoll

import (
	"context"
	"errors"
	"net"
	"os"

	"sync"
	"time"

	"github.com/big-larry/suckutils"
	"github.com/okonma-violet/services/logs/logger"
)

type listener struct {
	listener      net.Listener
	connsToHandle chan net.Conn
	activeWorkers chan struct{}
	handler       handlefunc
	keepAlive     bool

	servStatus *serviceStatus
	//configurator *configurator // если раскомменчивать - не забыть раскомментить строку в service.go после вызова newConfigurator()
	l logger.Logger

	ctx context.Context

	cancelAccept bool
	sync.RWMutex
}

type handlefunc func(conn net.Conn) error

const handlerCallTimeout time.Duration = time.Second * 5
const handlerCallMaxExceededTimeouts = 3

func newListener(ctx context.Context, l logger.Logger, servStatus *serviceStatus /* configurator *configurator,*/, threads int, connsbuf int, keepAlive bool, handler handlefunc) *listener {
	if threads < 1 {
		panic("threads num cant be less than 1")
	}
	return &listener{ctx: ctx,
		connsToHandle: make(chan net.Conn, connsbuf),
		activeWorkers: make(chan struct{}, threads),
		handler:       handler,
		keepAlive:     keepAlive,
		servStatus:    servStatus,
		//configurator:  configurator,
		l: l}
}

// TODO: я пока не придумал шо делать, если поднять листнер не удалось и мы ушли в суспенд (сейчас мы тупо не выйдем из суспенда)
func (listener *listener) listen(network, address string) error {
	if listener == nil {
		panic("listener.listen() called on nil listener")
	}
	listener.RLock()
	if listener.listener != nil {
		if listener.listener.Addr().String() == address {
			listener.RUnlock()
			return nil
		}
	}
	listener.RUnlock()
	listener.stop()

	listener.Lock()
	defer listener.Unlock()

loop:
	for {
		select {
		case listener.activeWorkers <- struct{}{}:
			go listener.handlingWorker()
			continue loop
		default:
			break loop
		}
	}

	var err error
	if network == "unix" {
		if err = os.RemoveAll(address); err != nil {
			goto failure
		}
	}
	if listener.listener, err = net.Listen(network, address); err != nil {
		goto failure
	}

	listener.cancelAccept = false
	go listener.acceptWorker()

	listener.servStatus.setListenerStatus(true)
	listener.l.Info("listen", suckutils.ConcatFour("start listening at ", network, ":", address))
	return nil
failure:
	listener.servStatus.setListenerStatus(false)
	return err
}

func (listener *listener) acceptWorker() {
	listener.RLock()
	defer listener.RUnlock()

	timer := time.NewTimer(handlerCallTimeout)
	for {
		conn, err := listener.listener.Accept()
		if err != nil {
			if listener.cancelAccept {
				listener.l.Debug("acceptWorker", "cancelAccept recieved, stop accept loop")
				timer.Stop()
				return
			}
			listener.l.Error("acceptWorker/Accept", err)
			continue
		}
		if !listener.servStatus.onAir() {
			listener.l.Warning("acceptWorker", suckutils.ConcatTwo("suspended, discard handling conn from ", conn.RemoteAddr().String()))
			conn.Close()
			continue
		}
	loop:
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
					break loop
				}
				listener.l.Warning("acceptWorker", suckutils.ConcatTwo("exceeded timeout, no free handlingWorker available for ", (handlerCallTimeout*i).String()))
			case listener.connsToHandle <- conn:
				break loop
			}
		}
	}
}

func (listener *listener) handlingWorker() {
	for conn := range listener.connsToHandle {
		listener.l.Debug("handlingWorker", "new request from "+conn.RemoteAddr().String())

		if err := listener.handler(conn); err != nil { // гарантированный первый хэндл, иначе если сразу в loop-у лезть, то при завершении контекста мы хер че отхэндлим
			listener.l.Error("handlingWorker/handle", errors.New(suckutils.ConcatThree(conn.RemoteAddr().String(), ", err: ", err.Error())))
			goto closing
		}
		if listener.keepAlive {
		loop:
			for {
				select {
				case <-listener.ctx.Done():
					break loop
				default:
					if err := listener.handler(conn); err != nil {
						// по идее, тут двойной сёр ошибкой в логи, если в хендлере она тоже логается
						listener.l.Error("handlingWorker/handle", errors.New(suckutils.ConcatThree(conn.RemoteAddr().String(), ", keepAlive closed on err: ", err.Error())))
						break loop
					}
				}
			}
		}
	closing:
		conn.Close()
	}
	<-listener.activeWorkers
}

// calling stop() we can call listen() again.
// и мы не ждем пока все отхэндлится
func (listener *listener) stop() {
	if listener == nil {
		panic("listener.stop() called on nil listener")
	}
	if !listener.onAir() {
		return
	}
	listener.cancelAccept = true
	if err := listener.listener.Close(); err != nil {
		listener.l.Error("listener.stop()/listener.Close()", err)
	}

	listener.Lock()
	listener.listener = nil
	listener.Unlock()
	listener.servStatus.setListenerStatus(false)
	listener.l.Debug("listener", "stopped")
	//listener.wg.Wait()
}

// calling close() we r closing listener forever (no further listen() calls) and waiting for all reqests to be handled
// потенциальная дыра: вызов listener.close() при keepAlive=true и НЕ завершенном контексте (см. handlingWorker())
func (listener *listener) close() {
	if listener == nil {
		panic("listener.close() called on nil listener")
	}
	listener.stop()
	listener.Lock() // если параллельно будет вызван listen(), то хуй мы воркеров красиво завершим (ибо они спавниться будут до таймаута)
	close(listener.connsToHandle)

	timeout := time.NewTimer(time.Second * 10).C
loop:
	for i := 0; i < cap(listener.activeWorkers); i++ {
		select {
		case listener.activeWorkers <- struct{}{}:
			continue loop
		case <-timeout:
			listener.l.Debug("listener", "closed on timeout 10s")
			return
		}
	}
	listener.l.Debug("listener", "succesfully closed")
}

func (listener *listener) onAir() bool {
	listener.RLock()
	defer listener.RUnlock()
	return listener.listener != nil
}

func (listener *listener) Addr() (string, string) {
	if listener == nil {
		return "", ""
	}
	listener.RLock()
	defer listener.RUnlock()
	if listener.listener == nil {
		return "", ""
	}
	return listener.listener.Addr().Network(), listener.listener.Addr().String()
}
