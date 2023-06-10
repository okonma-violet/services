package nonlistenerservice_nonepoll

import (
	"context"
	"errors"
	"strconv"
	"sync"

	"github.com/big-larry/suckutils"
	"github.com/okonma-violet/services/logs/logger"
)

// TODO: сделать чек завершения работы всеми рутинами
// TODO: в принципе работа всего этого выглядит как-то неконтролируемо

var ErrSoftTermination = errors.New("soft terminated")

type executor struct {
	rootctx            context.Context
	routinesctx_cancel context.CancelFunc

	terminationByUser chan struct{}
	termmux           sync.Mutex

	workersdata NLService

	active_workers chan struct{}

	l logger.Logger
}

func newExecutor(rootctx context.Context, l logger.Logger, threads int, workersdata NLService) *executor {
	if threads < 1 {
		panic("threads must not be less than 1")
	}
	return &executor{rootctx: rootctx,
		l:                  l,
		active_workers:     make(chan struct{}, threads),
		workersdata:        workersdata,
		terminationByUser:  make(chan struct{}),
		routinesctx_cancel: func() {}}
}

func (ex *executor) run() {
	ex.routinesctx_cancel()

	rtctx, cancel := context.WithCancel(ex.rootctx)
	ex.routinesctx_cancel = cancel
loop:
	for i := cap(ex.active_workers); i > 0; i-- { // такой for пушто если родительский контекст стрельнет (sigterm) - рискуем запетлиться тут
		select {
		case ex.active_workers <- struct{}{}:
			ll := ex.l.NewSubLogger(suckutils.ConcatTwo("exec-grt-", strconv.Itoa(len(ex.active_workers))))

			go func() {
				ll.Debug("Work", "started")
			wloop:
				for {
					select {
					case <-rtctx.Done():
						ll.Warning("Context", "done, exiting")
						break wloop
					default:
						if err := ex.workersdata.DoJob(rtctx, ll); err != nil {
							if err == ErrSoftTermination {
								ll.Debug("Work", "softly terminated by user")
								ex.terminateByUser()
								break wloop
							}
							ll.Error("Work", err)
							ex.terminateByUser()
							break wloop
						}
					}
				}
				<-ex.active_workers
				//ll.Debug("Work", "done")
			}()
			continue loop
		default:
			return
		}
	}

}

func (ex *executor) cancel() {
	ex.routinesctx_cancel()
}

func (ex *executor) terminateByUser() {
	ex.termmux.Lock()
	defer ex.termmux.Unlock()

	select {
	case <-ex.terminationByUser:
		return
	default:
		close(ex.terminationByUser)
	}
}

func (ex *executor) terminated() <-chan struct{} {
	return ex.terminationByUser
}

// func (ex *executor) alldone() <-chan struct{} {
// 	return f.allflushed
// }
// func (ex *executor) alldoneWithTimeout(timeout time.Duration) {
// 	t := time.NewTimer(timeout)
// 	select {
// 	case <-f.allflushed:
// 		return
// 	case <-t.C:
// 		encode.PrintLog(encode.EncodeLog(encode.Error, time.Now(), flushertags, "DoneWithTimeout", "reached timeout, skip last flush"))
// 		return
// 	}
// }
