package nonlistenerservice_nonepoll

import (
	"context"
	"errors"
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

type Servicier interface {
	Prepare(rootctx context.Context, pubs_getter Publishers_getter) (NLService, error)
}

type flagger interface {
	InitFlags()
}

type closer interface {
	Close(logger.Logger) error
}

type config_serv struct {
	ConfiguratorAddr string
}

type NLService interface {
	// YOU MUST LISTEN TO routinectx.Done() AND RETURN WHEN IT SHOTS!!!  routinectx - sub of rootctx.
	// Returning non-nil error -> shutdown program. If you want to terminate without err catched - return ErrSoftTermination
	DoJob(routinectx context.Context, logger logger.Logger) error
}

// Want to use flags - write func InitFlags() for Servicier.
// Want to closefunc on exit - write func Close() for NLService
func InitNewService(servicename ServiceName, config Servicier, workthreads int, publishers_names ...ServiceName) {
	if flagsin, ok := config.(flagger); ok {
		flagsin.InitFlags()
	}

	servconf := &config_serv{}
	if err := confdecoder.DecodeFile("config.txt", servconf, config); err != nil {
		panic("reading/decoding config.txt err: " + err.Error())
	}

	nocnf := flag.Bool("confless", false, "run without configurator (in this mode you can't use lib's publishers)")
	flag.IntVar(&workthreads, "threads", workthreads, "rewrites built threads number")
	flag.Parse()

	if !*nocnf && servconf.ConfiguratorAddr == "" {
		panic("ConfiguratorAddr in config.txt not specified")
	}

	ctx, cancel := createContextWithInterruptSignal()

	flsh := logger.NewFlusher(encode.DebugLevel)
	l := flsh.NewLogsContainer(string(servicename))

	// как изящнее сделать не придумал еще
	var execut *executor
	var cnfgr *configurator
	var workersdata NLService
	var err error
	if *nocnf {
		if len(publishers_names) != 0 {
			panic(errors.New("you cant use lib's publishers in confless mode"))
		}
		workersdata, err = config.Prepare(ctx, nil)
		if err != nil {
			panic(err)
		}
		execut = newExecutor(ctx, l, workthreads, workersdata)
		execut.run()
		cnfgr = &configurator{terminationByConfigurator: make(chan struct{})}
	} else {
		servStatus := newServiceStatus()
		var pubs *publishers
		if len(publishers_names) != 0 {
			if pubs, err = newPublishers(l.NewSubLogger("pubs"), servStatus, publishers_names); err != nil {
				panic(err)
			}
		} else {
			servStatus.setPubsStatus(true)
		}
		workersdata, err = config.Prepare(ctx, pubs)
		if err != nil {
			panic(err)
		}
		execut = newExecutor(ctx, l, workthreads, workersdata)
		cnfgr = newConfigurator(ctx, l.NewSubLogger("configurator"), execut, servStatus, pubs, servconf.ConfiguratorAddr, servicename, time.Second*5)

		servStatus.setOnSuspendFunc(cnfgr.onSuspend)
		servStatus.setOnUnSuspendFunc(cnfgr.onUnSuspend)
	}

	select {
	case <-ctx.Done():
		l.Info("Shutdown", "reason: context done")
		break
	case <-cnfgr.terminated():
		l.Info("Shutdown", "reason: terminated by configurator")
		cancel() //??
		break
	case <-execut.terminated():
		l.Info("Shutdown", "reason: terminated by user")
		//execut.cancel()
		cancel()
		break
	}

	if closehandler, ok := workersdata.(closer); ok {
		if err = closehandler.Close(l.NewSubLogger("Closer")); err != nil {
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
