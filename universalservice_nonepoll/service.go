package universalservice_nonepoll

import (
	"context"
	"flag"
	"os"
	"os/signal"

	"syscall"
	"time"

	"github.com/okonma-violet/confdecoder"
	"github.com/okonma-violet/services/logs/encode"
	"github.com/okonma-violet/services/logs/logger"
)

type ServiceName string

type flagger interface {
	InitFlags()
}

type Closer interface {
	Close(logger.Logger) error
}

type config_base struct {
	ConfiguratorAddr string
}

const pubscheckTicktime time.Duration = time.Second * 2

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

	ctx, cancel := createContextWithInterruptSignal()

	flsh := logger.NewFlusher(encode.DebugLevel)
	l := flsh.NewLogsContainer(string(servicename))

	servStatus := newServiceStatus()

	// Connector here is only needed for configurator
	// if err := connector.SetupGopoolHandling(handlethreads, 1, handlethreads); err != nil {
	// 	panic(err)
	// }

	var pubs *publishers
	var err error
	if len(publishers_names) != 0 {
		if pubs, err = newPublishers(ctx, l.NewSubLogger("pubs"), servStatus, nil, pubscheckTicktime, publishers_names); err != nil {
			panic(err)
		}
	} else {
		servStatus.setPubsStatus(true)
	}
	handler, clsr, err := config.PrepareHandling(ctx, pubs)
	if err != nil {
		panic(err)
	}

	ln := newListener(ctx, l, servStatus, handlethreads, handler)

	configurator := newConfigurator(ctx, l.NewSubLogger("configurator"), servStatus, pubs, ln, servconf.ConfiguratorAddr, servicename, time.Second*5)
	if pubs != nil {
		pubs.configurator = configurator
	}

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
		if err = clsr.Close(l.NewSubLogger("Closer")); err != nil {
			l.Error("Closer", err)
		}
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

//func handler(ctx context.Context, conn net.Conn) error
