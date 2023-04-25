package logger

import (
	"time"

	"github.com/okonma-violet/services/logs/encode"
)

type LogsContainer struct {
	ch   chan [][]byte
	tags []byte //tags
}

func (f *Flusher) NewLogsContainer(tags ...string) Logger {
	tagslist := encode.AppendTags(nil, tags...)
	return &LogsContainer{ch: f.ch, tags: tagslist}
}

func (l *LogsContainer) Debug(name, logstr string) {
	l.ch <- [][]byte{encode.EncodeLog(encode.Debug, time.Now(), l.tags, name, logstr)}
}

func (l *LogsContainer) Info(name, logstr string) {
	l.ch <- [][]byte{encode.EncodeLog(encode.Info, time.Now(), l.tags, name, logstr)}
}

func (l *LogsContainer) Warning(name, logstr string) {
	l.ch <- [][]byte{encode.EncodeLog(encode.Warning, time.Now(), l.tags, name, logstr)}
}

func (l *LogsContainer) Error(name string, logerr error) {
	var logstr string
	if logerr != nil {
		logstr = logerr.Error()
	} else {
		logstr = "nil err"
	}
	l.ch <- [][]byte{encode.EncodeLog(encode.Error, time.Now(), l.tags, name, logstr)}
}

func (l *LogsContainer) NewSubLogger(tags ...string) Logger {
	return &LogsContainer{ch: l.ch, tags: encode.AppendTags(l.tags, tags...)}
}
