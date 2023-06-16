package main

import (
	"context"
	"errors"
	"flag"
	"time"

	"github.com/big-larry/suckhttp"
	"github.com/okonma-violet/services/basicmessage"
	"github.com/okonma-violet/services/basicmessage/basicmessagetypes"
	"github.com/okonma-violet/services/logs/logger"
	"github.com/okonma-violet/services/universalservice_nonepoll"
)

// read this from configfile
type config struct {
	exampleflag bool
}

// your shit here
type service struct {
	pub universalservice_nonepoll.Publisher
}

const thisServiceName universalservice_nonepoll.ServiceName = "example"
const examplePubName universalservice_nonepoll.ServiceName = "examplepub"

func (c *config) InitFlags() {
	flag.BoolVar(&c.exampleflag, "example", false, "example flag init")
}

func (c *config) PrepareHandling(ctx context.Context, pubs_getter universalservice_nonepoll.Publishers_getter) (universalservice_nonepoll.BaseHandleFunc, universalservice_nonepoll.Closer, error) {
	s := &service{pub: *pubs_getter.Get(examplePubName)}
	return universalservice_nonepoll.CreateHTTPHandleFunc(s), s, nil
	return universalservice_nonepoll.CreateBasicHandleFunc(s), s, nil
}

func (s *service) HandleHTTP(r *suckhttp.Request, l logger.Logger) (*suckhttp.Response, error) {
	s.pub.SendBasicMessage(&basicmessage.BasicMessage{Payload: []byte("something")})
	s.pub.SendBasicMessageWithTimeout(&basicmessage.BasicMessage{Payload: []byte("something")}, time.Hour)

	req, _ := suckhttp.NewRequest(suckhttp.POST, "/")
	s.pub.SendHTTP(req)

	return nil, nil                       // = 500 response
	return nil, errors.New("example err") // logs both returned err and possible conn.Write() err
	return suckhttp.NewResponse(200, "OK"), nil
	return suckhttp.NewResponse(400, "Bad Request"), errors.New("example err") // this response wil be sent, err will be logged
}

func (s *service) HandleBasic(m *basicmessage.BasicMessage, l logger.Logger) (*basicmessage.BasicMessage, error) {
	s.pub.SendBasicMessage(&basicmessage.BasicMessage{Payload: []byte("something")})
	s.pub.SendBasicMessageWithTimeout(&basicmessage.BasicMessage{Payload: []byte("something")}, time.Hour)

	req, _ := suckhttp.NewRequest(suckhttp.POST, "/")
	s.pub.SendHTTP(req)

	return nil, nil                       // = 500 response
	return nil, errors.New("example err") // logs both returned err and possible conn.Write() err
	return &basicmessage.BasicMessage{Payload: append([]byte{byte(basicmessagetypes.OperationCodeOK)}, []byte("something")...)}, nil
	return &basicmessage.BasicMessage{Payload: append([]byte{byte(basicmessagetypes.OperationCodeBadRequest)}, []byte("something")...)}, errors.New("example err") // this response wil be sent, err will be logged
}

func (s *service) Close(l logger.Logger) error {
	return nil
}

func main() {
	universalservice_nonepoll.InitNewService(thisServiceName, &config{}, 1, examplePubName)
	universalservice_nonepoll.InitNewService(thisServiceName, &config{}, 6, examplePubName)
}
