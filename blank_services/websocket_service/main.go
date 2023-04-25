package main

import (
	"context"
	"project/test/types"
	"project/wsservice"
)

//read this from configfile
type config struct {
}

//your shit here
type service struct {
}

const thisServiceName wsservice.ServiceName = "example"

func (c *config) CreateHandlers(ctx context.Context, pubs_getter wsservice.Publishers_getter) (wsservice.Service, error) {

	return &service{}, nil
}

// wsservice.Service interface implementation
func (s *service) CreateNewWsData(l types.Logger) wsservice.Handler {

	return &wsconn{}
}

// may be omitted
func (s *service) Close() error {
	return nil
}

func main() {
	// one of this:
	wsservice.InitNewService(thisServiceName, &config{}, 1, "publishername")
	wsservice.InitNewServiceWithoutConfigurator(thisServiceName, &config{}, 1)
}
