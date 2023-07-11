package logger

import (
	"time"

	"github.com/okonma-violet/services/logs/encode"
)

type Flusher struct {
	ch         chan [][]byte
	flushlvl   encode.LogsFlushLevel
	cancel     chan struct{}
	allflushed chan struct{}
}

var flushertags []byte

const chanlen = 4

// TODO: make logsserver
// when nonlocal = true, flush logs to logsservers, when logsserver is not available, saves logs for further flush to this server on reconnect
func NewFlusher(logsflushlvl encode.LogsFlushLevel /*, nonlocal bool*/) LogsFlusher {
	f := &Flusher{
		ch:         make(chan [][]byte, chanlen),
		flushlvl:   logsflushlvl,
		cancel:     make(chan struct{}),
		allflushed: make(chan struct{}),
	}
	go f.flushWorker()
	return f
}

func (f *Flusher) flushWorker() {
	for {
		select {
		case logslist := <-f.ch:
			for _, bytelog := range logslist {
				if encode.GetLogLvl(bytelog) >= f.flushlvl {
					encode.PrintLog(bytelog)
				}
			}
		case <-f.cancel:
			for {
				select {
				case logslist := <-f.ch:
					for _, bytelog := range logslist {
						if encode.GetLogLvl(bytelog) >= f.flushlvl {
							encode.PrintLog(bytelog)
						}
					}
				default:
					close(f.allflushed)
					return
				}
			}
		}
	}
}

func (f *Flusher) Close() {
	close(f.cancel)
}

func (f *Flusher) Done() <-chan struct{} {
	return f.allflushed
}
func (f *Flusher) DoneWithTimeout(timeout time.Duration) {
	t := time.NewTimer(timeout)
	select {
	case <-f.allflushed:
		return
	case <-t.C:
		encode.PrintLog(encode.EncodeLog(encode.Error, time.Now(), flushertags, "DoneWithTimeout", "reached timeout, skip last flush"))
		return
	}

}
