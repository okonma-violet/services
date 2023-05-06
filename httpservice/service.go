package httpservice

import (
	"context"
	"net"
	"os"
	"os/signal"

	"syscall"
	"time"

	"github.com/big-larry/suckhttp"
	"github.com/okonma-violet/confdecoder"
	"github.com/okonma-violet/connector"
	"github.com/okonma-violet/services/logs/encode"
	"github.com/okonma-violet/services/logs/logger"
)

type ServiceName string

type Servicier interface {
	CreateHandler(ctx context.Context, pubs_getter Publishers_getter) (HTTPService, error)
}

type closer interface {
	Close() error
}

type config_toml struct {
	ConfiguratorAddr string
}

type HTTPService interface {
	// nil response = 500
	Handle(request *suckhttp.Request, logger logger.Logger) (*suckhttp.Response, error)
}

const pubscheckTicktime time.Duration = time.Second * 2

// TODO: ПЕРЕЕХАТЬ КОНФИГУРАТОРА НА НОН-ЕПУЛ КОННЕКТОР
// TODO: придумать шото для неторчащих наружу сервисов

func InitNewService(servicename ServiceName, config Servicier, keepConnAlive bool, handlethreads, connsbuf int, publishers_names ...ServiceName) {
	servconf := &config_toml{}
	if err := confdecoder.DecodeFile("config.txt", servconf, config); err != nil {
		panic("reading/decoding config.txt err: " + err.Error())
	}
	if servconf.ConfiguratorAddr == "" {
		panic("ConfiguratorAddr in config.txt not specified")
	}

	ctx, cancel := createContextWithInterruptSignal()

	flsh := logger.NewFlusher(encode.DebugLevel)
	l := flsh.NewLogsContainer(string(servicename))

	servStatus := newServiceStatus()

	connector.SetupEpoll(func(e error) {
		l.Error("epoll OnWaitError", e)
		cancel()
	})
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
	handler, err := config.CreateHandler(ctx, pubs)
	if err != nil {
		panic(err)
	}

	ln := newListener(ctx, l.NewSubLogger("listener"), servStatus, handlethreads, connsbuf, keepConnAlive, func(conn net.Conn) error {
		request, err := suckhttp.ReadRequest(ctx, conn, time.Minute)
		if err != nil {
			return err
		}
		response, err := handler.Handle(request, l)
		if response == nil {
			response = suckhttp.NewResponse(500, "Internal Server Error")
		}
		if err != nil {
			if writeErr := response.Write(conn, time.Minute); writeErr != nil {
				l.Error("Write response", writeErr)
			}
			return err
		}
		return response.Write(conn, time.Minute)
	})

	configurator := newConfigurator(ctx, l, servStatus, pubs, ln, servconf.ConfiguratorAddr, servicename, time.Second*5)
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
	case <-configurator.terminationByConfigurator:
		l.Info("Shutdown", "reason: termination by configurator")
		cancel() //??
		break
	}
	ln.close()

	if closehandler, ok := handler.(closer); ok {
		if err = closehandler.Close(); err != nil {
			l.Error("Handler.Closer", err)
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
