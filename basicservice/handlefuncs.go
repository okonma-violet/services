package basicservice

import (
	"context"
	"net"
	"time"

	"github.com/big-larry/suckhttp"
	"github.com/okonma-violet/services/basicmessage"
	"github.com/okonma-violet/services/basicmessage/basicmessagetypes"
	"github.com/okonma-violet/services/logs/logger"
)

type BaseHandleFunc func(logger.Logger, net.Conn) error

type HandleCreator interface {
	PrepareHandling(ctx context.Context, pubs_getter Publishers_getter) (BaseHandleFunc, Closer, error)
}

func CreateHTTPHandleFunc(hf func(*suckhttp.Request, logger.Logger) (*suckhttp.Response, error)) BaseHandleFunc {
	return func(l logger.Logger, conn net.Conn) error {
		request, err := suckhttp.ReadRequest(context.Background(), conn, time.Minute)
		if err != nil {
			return err
		}
		response, err := hf(request, l)
		if response == nil {
			response = suckhttp.NewResponse(500, "Internal Server Error")
		}
		if err != nil {
			if writeErr := response.Write(conn, time.Minute); writeErr != nil {
				l.Error("Write", writeErr)
			}
			return err
		}
		return response.Write(conn, time.Minute)
	}
}

func CreateBasicHandleFunc(hf func(*basicmessage.BasicMessage, logger.Logger) (*basicmessage.BasicMessage, error)) BaseHandleFunc {
	return func(l logger.Logger, conn net.Conn) error {
		requestmsg, err := basicmessage.ReadMessage(conn, time.Minute) //suckhttp.ReadRequest(context.Background(), conn, time.Minute)
		if err != nil {
			return err
		}
		responsemsg, err := hf(requestmsg, l)
		if responsemsg == nil {
			responsemsg = &basicmessage.BasicMessage{Payload: []byte{byte(basicmessagetypes.OperationCodeInternalServerError)}}
		}
		if err != nil {
			if _, writeErr := conn.Write(responsemsg.ToByte()); writeErr != nil {
				l.Error("Write", writeErr)
			}
			return err
		}
		_, err = conn.Write(responsemsg.ToByte())
		return err
	}
}
