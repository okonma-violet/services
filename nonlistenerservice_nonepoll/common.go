package nonlistenerservice_nonepoll

import (
	"context"
	"os"
	"os/signal"
	"runtime"
	"strconv"
	"syscall"
	"time"

	"github.com/okonma-violet/services/logs/logger"
)

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

// can panic
func createLogFileAt(path, servicename string) (*os.File, string) {
	stat, err := os.Stat(path)
	if err != nil {
		panic("os.Stat err on LogsPath: " + err.Error())
	}
	if !stat.IsDir() {
		panic("LogsPath is not a directory")
	}

	// stat, err = os.Stat(path)
	// if err != nil {
	// 	if !os.IsNotExist(err) {
	// 		panic("os.Stat err on LogsPath/servicename: " + err.Error())
	// 	}
	// 	if err = os.Mkdir(logfilepath, 0755); err != nil {
	// 		panic("os.Mkdir at LogsPath/servicename, err: " + err.Error())
	// 	}
	// }
	logfilepath := path + "/" + "log_" + string(servicename) + time.Now().Format("_02-01-06_15:04:05.log")
	logfile, err := os.Create(logfilepath)
	if err != nil {
		panic("os.Create logfile at " + path + " err: " + err.Error())
	}
	return logfile, logfilepath
}

// to release this grt - read from ch twice
func ramusagewatcher(l logger.Logger, ticksec float64, ch chan struct{}) {
	var memstat runtime.MemStats
	var maxalloc uint64
	ticker := time.NewTicker(time.Duration(float64(time.Second) * ticksec))

	for {
		select {
		case <-ticker.C:
			runtime.ReadMemStats(&memstat)
			if memstat.Alloc > maxalloc {
				maxalloc = memstat.Alloc
			}
			l.Info("MemStat", "heap alloc: "+strconv.FormatUint(memstat.Alloc/1048576, 10)+" MiB, sys: "+strconv.FormatUint(memstat.Sys/1048576, 10)+" MiB, total heap alloc: "+strconv.FormatUint(memstat.TotalAlloc/1048576, 10)+" MiB, gc num: "+strconv.FormatUint(uint64(memstat.NumGC), 10))
		case ch <- struct{}{}:
			runtime.ReadMemStats(&memstat)
			if memstat.Alloc > maxalloc {
				maxalloc = memstat.Alloc
			}
			l.Info("MemStat", "max heap alloc (at one moment): "+strconv.FormatUint(maxalloc/1048576, 10)+" MiB, sys: "+strconv.FormatUint(memstat.Sys/1048576, 10)+" MiB, total heap alloc: "+strconv.FormatUint(memstat.TotalAlloc/1048576, 10)+" MiB, gc num: "+strconv.FormatUint(uint64(memstat.NumGC), 10))
			ch <- struct{}{}
			return
		}
	}
}
