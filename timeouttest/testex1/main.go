package main

import (
	"context"
	"fmt"

	"github.com/big-larry/suckhttp"
	"github.com/okonma-violet/services/basicmessage"
	"github.com/okonma-violet/services/logs/logger"
	"github.com/okonma-violet/services/universalservice_nonepoll"
)

// read this from configfile
type config struct {
}

// your shit here
type service struct {
	pub *universalservice_nonepoll.Publisher
}

const thisServiceName universalservice_nonepoll.ServiceName = "testex1"
const examplePubName universalservice_nonepoll.ServiceName = "testex2"

func (c *config) InitFlags() {
	//flag.BoolVar(&c.exampleflag, "example", false, "example flag init")
}

func (c *config) PrepareHandling(ctx context.Context, pubs_getter universalservice_nonepoll.Publishers_getter) (universalservice_nonepoll.BaseHandleFunc, universalservice_nonepoll.Closer, error) {
	s := &service{pub: pubs_getter.Get(examplePubName)}
	return universalservice_nonepoll.CreateHTTPHandleFunc(s), s, nil
}

func (s *service) HandleHTTP(r *suckhttp.Request, l logger.Logger) (*suckhttp.Response, error) {
	_, err := s.pub.SendBasicMessage(&basicmessage.BasicMessage{Payload: []byte("something")})
	fmt.Println(err)
	return suckhttp.NewResponse(200, "OK"), nil
}

func (s *service) Close(l logger.Logger) error {
	return nil
}

func main() {
	universalservice_nonepoll.InitNewService(thisServiceName, &config{}, 1, examplePubName)
}
