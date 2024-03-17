package main

import (
	"context"
	"errors"
	"time"

	"github.com/okonma-violet/services/basicmessage"
	"github.com/okonma-violet/services/basicmessage/basicmessagetypes"
	"github.com/okonma-violet/services/logs/logger"
	"github.com/okonma-violet/services/universalservice_nonepoll"
)

// read this from configfile
type config struct {
}

// your shit here
type service struct {
}

const thisServiceName universalservice_nonepoll.ServiceName = "testex2"

func (c *config) InitFlags() {
}

func (c *config) PrepareHandling(ctx context.Context, pubs_getter universalservice_nonepoll.Publishers_getter) (universalservice_nonepoll.BaseHandleFunc, universalservice_nonepoll.Closer, error) {
	s := &service{}
	return universalservice_nonepoll.CreateBasicHandleFunc(s), s, nil
}

func (s *service) HandleBasic(m *basicmessage.BasicMessage, l logger.Logger) (*basicmessage.BasicMessage, error) {
	l.Info("req", "payload: "+string(m.Payload))
	l.Info("", "timeouting")
	time.Sleep(time.Second * 30)

	return nil, nil                       // = 500 response
	return nil, errors.New("example err") // logs both returned err and possible conn.Write() err
	return &basicmessage.BasicMessage{Payload: append([]byte{byte(basicmessagetypes.OperationCodeOK)}, []byte("something")...)}, nil
	return &basicmessage.BasicMessage{Payload: append([]byte{byte(basicmessagetypes.OperationCodeBadRequest)}, []byte("something")...)}, errors.New("example err") // this response wil be sent, err will be logged
}

func (s *service) Close(l logger.Logger) error {
	return nil
}

func main() {
	universalservice_nonepoll.InitNewService(thisServiceName, &config{}, 1)
}
