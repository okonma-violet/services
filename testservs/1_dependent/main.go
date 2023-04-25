package main

import (
	"context"
	"errors"

	"github.com/big-larry/suckhttp"
	"github.com/okonma-violet/services/httpservice"
	"github.com/okonma-violet/services/logs/logger"
)

// read this from configfile
type config struct {
}

// your shit here
type service struct {
	pub_second *httpservice.Publisher
	pub_third  *httpservice.Publisher
}

const thisServiceName httpservice.ServiceName = "first"

const pubname_second httpservice.ServiceName = "second"
const pubname_third httpservice.ServiceName = "third"

func (c *config) CreateHandler(ctx context.Context, pubs_getter httpservice.Publishers_getter) (httpservice.HTTPService, error) {
	s := &service{
		pub_second: pubs_getter.Get(pubname_second),
		pub_third:  pubs_getter.Get(pubname_third),
	}
	return s, nil
}

func (s *service) Handle(r *suckhttp.Request, l logger.Logger) (*suckhttp.Response, error) {
	req, err := httpservice.CreateHTTPRequest(suckhttp.GET)
	if err != nil {
		l.Error("CreateHTTPRequest", err)
		return nil, nil
	}
	resp2, err := s.pub_second.SendHTTP(req)
	if err != nil {
		l.Error("pub_second.SendHTTP", err)
		return nil, nil
	}
	if sc, st := resp2.GetStatus(); sc != 200 {
		l.Error("pub_second/resp status", errors.New(st))
		return nil, nil
	}
	resp3, err := s.pub_third.SendHTTP(req)
	if err != nil {
		l.Error("pub_third.SendHTTP", err)
		return nil, nil
	}
	if sc, st := resp3.GetStatus(); sc != 200 {
		l.Error("pub_third/resp status", errors.New(st))
		return nil, nil
	}
	response := suckhttp.NewResponse(200, "OK").SetBody([]byte("service 1\nCount from service 2: " + string(resp2.GetBody()) + "\nRandom hash from service 3: " + string(resp3.GetBody())))

	return response, nil
}

// may be omitted
func (s *service) Close() error {
	return nil
}

func main() {
	httpservice.InitNewService(thisServiceName, &config{}, false, 5, 5, pubname_second, pubname_third)
}
