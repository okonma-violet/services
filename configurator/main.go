package main

import (
	"context"
	"errors"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/big-larry/suckutils"
	"github.com/okonma-violet/confdecoder"
	"github.com/okonma-violet/connector"
	"github.com/okonma-violet/dynamicworkerspool"
	"github.com/okonma-violet/services/epolllistener"
	"github.com/okonma-violet/services/logs/encode"
	"github.com/okonma-violet/services/logs/logger"
	"github.com/okonma-violet/services/types/netprotocol"
)

type config struct {
	ListenLocal    string // куда стучатся сервисы, обслуживаемые этим конфигуратором (могут быть и не локальными)
	ListenExternal string // куда стучатся другие конфигураторы. можно ListenLocal = ListenExternal, если tcp
	Settings       string
}

const connectors_min_threads int = 1
const connectors_max_threads int = 5
const threadkilling_timeout time.Duration = time.Second * 5
const reconnectsCheckTicktime time.Duration = time.Second * 10
const reconnectsTargetBufSize int = 4

var thisConfOuterPort []byte

// TODO: рассылка обновлений по подпискам для своих (обслуживаемых локальным конфигуратором) удаленных сервисов будет работать будет через жопу (т.е. не будет работать)
// TODO: решить гемор с обменом со вторым конфигуратором данными о третьих конфигураторах, и как на это будет реагировать ридсеттингс
func main() {
	conf := &config{}
	err := confdecoder.DecodeFile("config.txt", conf)
	if err != nil {
		panic("read config file err: " + err.Error())
	}
	if (conf.ListenLocal == "" && conf.ListenExternal == "") || conf.Settings == "" {
		panic("some fields in config file are empty or not specified")
	}

	ctx, cancel := createContextWithInterruptSignal()

	flsh := logger.NewFlusher(encode.DebugLevel)
	l := flsh.NewLogsContainer("configurator")

	epolllistener.SetEpoll(connector.SetupEpoll(func(e error) {
		l.Error("Epoll", e)
		cancel()
	}))

	pool := dynamicworkerspool.NewPool(connectors_min_threads, connectors_max_threads, threadkilling_timeout)
	connector.SetupPoolHandling(pool)

	initReconnection(ctx, reconnectsCheckTicktime, reconnectsTargetBufSize, 1)

	subs := newSubscriptions(ctx, l, 5, nil)

	servs := newServices(l.NewSubLogger("services"), conf.Settings, subs)
	subs.servs = servs

	var local_ln, external_ln listenier
	var allow_remote_on_local_ln bool

	if conf.ListenExternal != "" {
		if netprotocol.NetProtocolTcp.String() != conf.ListenExternal[:strings.Index(conf.ListenExternal, ":")] {
			panic("specified netprotocol for ListenExternal must be tcp")
		}
		if conf.ListenExternal != conf.ListenLocal {
			if external_ln, err = newListener(conf.ListenExternal[:strings.Index(conf.ListenExternal, ":")], conf.ListenExternal[strings.LastIndex(conf.ListenExternal, ":"):], true, subs, servs, l); err != nil {
				panic("newListener remote err: " + err.Error())
			}
			thisConfOuterPort = []byte(conf.ListenExternal[strings.LastIndex(conf.ListenExternal, ":")+1:])
		} else {
			thisConfOuterPort = []byte(conf.ListenLocal[strings.LastIndex(conf.ListenLocal, ":")+1:])
			allow_remote_on_local_ln = true
		}
	}
	localaddr := conf.ListenLocal[strings.Index(conf.ListenLocal, ":")+1:]
	if netprotocol.NetProtocolTcp.String() == conf.ListenLocal[:strings.Index(conf.ListenLocal, ":")] {
		localaddr = suckutils.ConcatTwo(":", localaddr)
	}
	if local_ln, err = newListener(conf.ListenLocal[:strings.Index(conf.ListenLocal, ":")], localaddr, allow_remote_on_local_ln, subs, servs, l); err != nil {
		panic("newListener local err: " + err.Error())
	}

	<-ctx.Done()
	l.Debug("Context", "done, exiting")
	local_ln.close()
	if external_ln != nil {
		external_ln.close()
	}
	pool.Close()
	if err = pool.DoneWithTimeout(time.Second * 5); err != nil {
		l.Error("Gopool", errors.New("break gopool.done waiting: timed out"))
	}
	flsh.Close()
	flsh.DoneWithTimeout(time.Second * 5)
}

func createContextWithInterruptSignal() (context.Context, context.CancelFunc) {
	ctx, cancel := context.WithCancel(context.Background())
	stop := make(chan os.Signal, 1)
	signal.Notify(stop, os.Interrupt, syscall.SIGTERM)

	go func() {
		<-stop
		cancel()
	}()
	return ctx, cancel
}
