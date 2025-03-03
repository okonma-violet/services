package universalservice_nonepoll

import (
	"context"
	"flag"

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
	LogsPath         string
	ServiceName      string
}

// NON-EPOLL.
// example of usage: ../blank_services/universalservice/

func InitNewServiceWithoutName(config handleCreator, handlethreads int, publishers_names ...ServiceName) {
	servconf := &config_base{}
	if err := confdecoder.DecodeFile("config.txt", servconf, config); err != nil {
		panic("reading/decoding config.txt err: " + err.Error())
	}
	if servconf.ServiceName == "" {
		panic("ServiceName in config.txt not specified")
	}
	if servconf.ConfiguratorAddr == "" {
		panic("ConfiguratorAddr in config.txt not specified")
	}
	if flagsin, ok := config.(flagger); ok {
		flagsin.InitFlags()
	}
	initNewService(servconf, ServiceName(servconf.ServiceName), config, handlethreads, publishers_names...)
}

func InitNewService(servicename ServiceName, config handleCreator, handlethreads int, publishers_names ...ServiceName) {
	if servicename == "" {
		panic("servicename is not specified")
	}
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
	initNewService(servconf, servicename, config, handlethreads, publishers_names...)
}

func initNewService(servconf *config_base, servicename ServiceName, config handleCreator, handlethreads int, publishers_names ...ServiceName) {
	dbgramusage := flag.Bool("ramusage", false, "prints ram usage stat (print stats by interval and on exit)")
	dbrramusageticksec := flag.Float64("ramusage-interval", 5.0, "sets interval for \"-ramusage\" in (float) seconds, default is 5.0s")
	flag.IntVar(&handlethreads, "threads", handlethreads, "rewrites built threads number")
	flag.Parse()

	var flsh logger.LogsFlusher
	if servconf.LogsPath == "" {
		flsh = logger.NewFlusher(encode.DebugLevel)
		encode.Println(encode.Warning.Byte(), "writing logs to stdout only (you can set \"LogsPath\" in config file, dont forget to create that dir)")
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

	servStatus := newServiceStatus()

	var pubs *publishers
	var err error
	if len(publishers_names) != 0 {
		if pubs, err = newPublishers(l.NewSubLogger("pubs"), servStatus, publishers_names); err != nil {
			panic(err)
		}
	} else {
		servStatus.setPubsStatus(true)
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
		if err = clsr.Close(l.NewSubLogger("Closer")); err != nil {
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
