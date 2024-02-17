package logger

import (
	"io"
	"time"

	"github.com/okonma-violet/services/logs/encode"
)

type flusher struct {
	ch         chan [][]byte
	flushlvl   encode.LogsFlushLevel
	cancel     chan struct{}
	allflushed chan struct{}
	w          io.Writer
}

var flushertags []byte

const chanlen = 4

// TODO: make logsserver
// when nonlocal = true, flush logs to logsservers, when logsserver is not available, saves logs for further flush to this server on reconnect
func NewFlusher(logsflushlvl encode.LogsFlushLevel, wrs ...io.Writer) LogsFlusher {
	f := &flusher{
		ch:         make(chan [][]byte, chanlen),
		flushlvl:   logsflushlvl,
		cancel:     make(chan struct{}),
		allflushed: make(chan struct{}),
	}
	if len(wrs) == 1 {
		f.w = wrs[0]
	}
	if len(wrs) > 1 {
		f.w = io.MultiWriter(wrs...)
	}
	go f.flushWorker()
	return f
}

func (f *flusher) flushWorker() {
	for {
		select {
		case logslist := <-f.ch:
			for _, bytelog := range logslist {
				if encode.GetLogLvl(bytelog) >= f.flushlvl {
					encode.PrintLog(f.w, bytelog)
				}
			}
		case <-f.cancel:
			for {
				select {
				case logslist := <-f.ch:
					for _, bytelog := range logslist {
						if encode.GetLogLvl(bytelog) >= f.flushlvl {
							encode.PrintLog(f.w, bytelog)
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

func (f *flusher) Close() {
	close(f.cancel)
}

func (f *flusher) Done() <-chan struct{} {
	return f.allflushed
}
func (f *flusher) DoneWithTimeout(timeout time.Duration) {
	t := time.NewTimer(timeout)
	select {
	case <-f.allflushed:
		return
	case <-t.C:
		encode.PrintLog(f.w, encode.EncodeLog(encode.Error, time.Now(), flushertags, "DoneWithTimeout", "reached timeout, skip last flush"))
		return
	}

}
