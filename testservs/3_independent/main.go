package main

import (
	"context"
	"crypto/md5"
	"encoding/hex"
	"time"

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

const thisServiceName httpservice.ServiceName = "third"

func (c *config) CreateHandler(ctx context.Context, pubs_getter httpservice.Publishers_getter) (httpservice.HTTPService, error) {
	s := &service{}
	return s, nil
}

func (s *service) Handle(r *suckhttp.Request, l logger.Logger) (*suckhttp.Response, error) {

	response := suckhttp.NewResponse(200, "OK").SetBody([]byte(getMD5(time.Now().String())))
	return response, nil
}

// may be omitted
func (s *service) Close() error {
	return nil
}

func main() {
	httpservice.InitNewService(thisServiceName, &config{}, false, 5, 5)
}

func getMD5(str string) string {
	hash := md5.New()
	b := make([]byte, len(str))
	copy(b, []byte(str))
	if _, err := hash.Write(b); err != nil {
		panic(err)
	}
	return hex.EncodeToString(hash.Sum(nil))
}
