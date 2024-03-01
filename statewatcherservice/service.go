package universalservice_nonepoll

import (
	"context"
	"flag"

	"time"

	"github.com/okonma-violet/confdecoder"
)

type ServiceName string

type flagger interface {
	InitFlags()
}

type Closer interface {
	Close() error
}

type config_base struct {
	ConfiguratorAddr string
	LogsPath         string
}

// NON-EPOLL.
// example of usage: ../blank_services/universalservice/

func InitNewService(servicename ServiceName, config HandleCreator, handlethreads int, publishers_names ...ServiceName) {
	servconf := &config_base{}
	if err := confdecoder.DecodeFile("config.txt", servconf, config); err != nil {
		panic("reading/decoding config.txt err: " + err.Error())
	}
	if servconf.ConfiguratorAddr == "" {
		panic("ConfiguratorAddr in config.txt not specified")
	}
	if flagsin, ok := config.(flagger); ok {
		flagsin.InitFlags()
	}

	flag.IntVar(&handlethreads, "threads", handlethreads, "rewrites built threads number")
	flag.Parse()

	pubs, err := newPublishers(l.NewSubLogger("pubs"), servStatus, publishers_names)
	if err != nil {
		panic(err)
	}

	ctx, cancel := createContextWithInterruptSignal()
	handler, clsr, err := config.PrepareHandling(ctx, pubs)
	if err != nil {
		panic(err)
	}

	ln := newListener(ctx, l, servStatus, handlethreads, handler)

	cnfgr_ctx, cnfgr_ctx_cancel := context.WithCancel(context.Background())
	configurator := newConfigurator(cnfgr_ctx, l.NewSubLogger("configurator"), servStatus, pubs, ln, servconf.ConfiguratorAddr, servicename, time.Second*5)

	//ln.configurator = configurator
	servStatus.setOnSuspendFunc(configurator.onSuspend)
	servStatus.setOnUnSuspendFunc(configurator.onUnSuspend)

	select {
	case <-ctx.Done():
		l.Info("Shutdown", "reason: context done")
		break
	case <-configurator.terminated():
		l.Info("Shutdown", "reason: termination by configurator")
		cancel() //??
		break
	}
	ln.close()

	if clsr != nil {
		if err = clsr.Close(); err != nil {
			l.Error("Closer", err)
		}
	}

	cnfgr_ctx_cancel()

	if *dbgramusage {
		<-memstat_ch // to go to exit procedures
		<-memstat_ch // to release grt
	}

	flsh.Close()
	flsh.DoneWithTimeout(time.Second * 5)
}
