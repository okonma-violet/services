package main

import (
	"context"

	"github.com/okonma-violet/services/logs/logger"
	"github.com/okonma-violet/services/nonlistenerservice_nonepoll"
)

// read this from configfile
type config struct {
}

// your shit here
type service struct {
}

const thisServiceName nonlistenerservice_nonepoll.ServiceName = "example"

func (c *config) InitFlags() {
}

func (c *config) Prepare(ctx context.Context, pubs_getter nonlistenerservice_nonepoll.Publishers_getter) (nonlistenerservice_nonepoll.NLService, error) {
	s := &service{}
	return s, nil
}

func (s *service) DoJob(ctx context.Context, l logger.Logger) error {
	select {
	case <-ctx.Done():
		return nil
	default:
		//return nonlistenerservice_nonepoll.ErrSoftTermination
		return nil
	}
}

// may be omitted
func (s *service) Close() error {
	return nil
}

func main() {
	nonlistenerservice_nonepoll.InitNewService(thisServiceName, &config{}, 1)
}
