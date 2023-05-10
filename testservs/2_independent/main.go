package main

import (
	"context"
	"strconv"
	"sync"

	"github.com/big-larry/suckhttp"
	httpservicenonepoll "github.com/okonma-violet/services/httpservice_nonepoll"
	"github.com/okonma-violet/services/logs/logger"
)

// read this from configfile
type config struct {
}

// your shit here
type service struct {
	counter int
	sync.Mutex
}

const thisServiceName httpservicenonepoll.ServiceName = "second"

func (c *config) CreateHandler(ctx context.Context, pubs_getter httpservicenonepoll.Publishers_getter) (httpservicenonepoll.HTTPService, error) {
	s := &service{}
	return s, nil
}

func (s *service) Handle(r *suckhttp.Request, l logger.Logger) (*suckhttp.Response, error) {
	s.Lock()
	s.counter++
	cnt := s.counter
	s.Unlock()

	response := suckhttp.NewResponse(200, "OK").SetBody([]byte(strconv.Itoa(cnt)))
	return response, nil
}

// may be omitted
func (s *service) Close() error {
	return nil
}

func main() {
	httpservicenonepoll.InitNewService(thisServiceName, &config{}, false, 5, 5)
}
