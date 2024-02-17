package nonlistenerservice_nonepoll

import (
	"context"
	"errors"
	"flag"

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

type config_serv struct {
	ConfiguratorAddr string
	LogsPath         string
}

type NLService interface {
	// YOU MUST LISTEN TO routinectx.Done() AND RETURN WHEN IT SHOTS!!!  routinectx - sub of rootctx.
	// Returning non-nil error -> shutdown program. If you want to terminate without err catched - return ErrSoftTermination
	DoJob(routinectx context.Context, logger logger.Logger) error
	Close(logger.Logger) error
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
	dbgramusage := flag.Bool("ramusage", false, "prints ram usage stat (print stats by interval and on exit)")
	dbrramusageticksec := flag.Float64("ramusage-interval", 5.0, "sets interval for \"-ramusage\" in (float) seconds, default is 5.0s")
	flag.IntVar(&workthreads, "threads", workthreads, "rewrites built threads number")
	flag.Parse()

	var flsh logger.LogsFlusher
	if servconf.LogsPath == "" {
		flsh = logger.NewFlusher(encode.DebugLevel)
		encode.Println(encode.Warning.Byte(), "writing logs to stdout only (you can set \"LogsPath\" in config file)")
	} else {
		logfile, logfilepath := createLogFileAt(servconf.LogsPath, string(servicename))
		defer logfile.Close()
		flsh = logger.NewFlusher(encode.DebugLevel, logfile)
		encode.Println(encode.Info.Byte(), "created logfile at "+logfilepath)
	}
	l := flsh.NewLogsContainer(string(servicename))

	var memstat_ch chan struct{}
	if *dbgramusage {
		memstat_ch = make(chan struct{})
		go ramusagewatcher(l, *dbrramusageticksec, memstat_ch)
	}

	if !*nocnf && servconf.ConfiguratorAddr == "" {
		panic("ConfiguratorAddr in config.txt not specified")
	}

	// как изящнее сделать не придумал еще
	var execut *executor
	var cnfgr *configurator
	var cnfgr_ctx_cancel context.CancelFunc
	var workersdata NLService
	var err error

	ctx, cancel := createContextWithInterruptSignal()

	if *nocnf {
		if len(publishers_names) != 0 {
			panic(errors.New("you cant use lib's publishers in confless mode"))
		}
		workersdata, err = config.Prepare(ctx, nil)
		if err != nil {
			panic(err)
		}
		cnfgr_ctx_cancel = func() {}
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

		var cnfgr_ctx context.Context
		cnfgr_ctx, cnfgr_ctx_cancel = context.WithCancel(context.Background())
		cnfgr = newConfigurator(cnfgr_ctx, l.NewSubLogger("configurator"), execut, servStatus, pubs, servconf.ConfiguratorAddr, servicename, time.Second*5)

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

	if err = workersdata.Close(l.NewSubLogger("Closer")); err != nil {
		l.Error("Closer", err)
	}
	cnfgr_ctx_cancel()

	if *dbgramusage {
		<-memstat_ch // to go to exit procedures
		<-memstat_ch // to release grt
	}

	flsh.Close()
	flsh.DoneWithTimeout(time.Second * 5)
}
