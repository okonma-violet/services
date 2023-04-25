package main

import (
	"context"

	"github.com/big-larry/suckhttp"
	"github.com/okonma-violet/services/httpservice"
	"github.com/okonma-violet/services/logs/logger"
)

// read this from configfile
type config struct {
}

// your shit here
type service struct {
}

const thisServiceName httpservice.ServiceName = "example"

func (c *config) CreateHandler(ctx context.Context, pubs_getter httpservice.Publishers_getter) (httpservice.HTTPService, error) {
	s := &service{}

	return s, nil
}

func (s *service) Handle(r *suckhttp.Request, l logger.Logger) (*suckhttp.Response, error) {

	return suckhttp.NewResponse(200, "OK"), nil
}

// may be omitted
func (s *service) Close() error {
	return nil
}

func main() {
	httpservice.InitNewService(thisServiceName, &config{}, false, 1, "publishername")
}
