package universalservice_nonepoll

import (
	"context"
	"net"
	"time"

	"github.com/big-larry/suckhttp"
	"github.com/big-larry/suckutils"
	"github.com/okonma-violet/services/basicmessage"
	"github.com/okonma-violet/services/basicmessage/basicmessagetypes"
	"github.com/okonma-violet/services/logs/logger"
)

type BaseHandleFunc func(logger.Logger, net.Conn) error

type HandleCreator interface {
	PrepareHandling(ctx context.Context, pubs_getter Publishers_getter) (BaseHandleFunc, Closer, error)
}

type httpmsghandler interface {
	HandleHTTP(*suckhttp.Request, logger.Logger) (*suckhttp.Response, error)
}

type basicmsghandler interface {
	HandleBasic(*basicmessage.BasicMessage, logger.Logger) (*basicmessage.BasicMessage, error)
}

func CreateHTTPHandleFunc(h httpmsghandler) BaseHandleFunc {
	return func(l logger.Logger, conn net.Conn) error {
		request, err := suckhttp.ReadRequest(context.Background(), conn, time.Minute)
		if err != nil {
			return err
		}
		l.Debug("handle", "new req for "+request.Uri.Path)
		response, err := h.HandleHTTP(request, l)
		if response == nil {
			response = suckhttp.NewResponse(500, "Internal Server Error")
		}
		stcode, _ := response.GetStatus()
		l.Debug("handle", suckutils.Concat("req handled, resp code:", suckutils.Itoa(uint32(stcode))))

		if err != nil {
			if writeErr := response.Write(conn, time.Minute); writeErr != nil {
				l.Error("handle/Write", writeErr)
			}
			return err
		}
		return response.Write(conn, time.Minute)
	}
}

func CreateBasicHandleFunc(h basicmsghandler) BaseHandleFunc {
	return func(l logger.Logger, conn net.Conn) error {
		requestmsg, err := basicmessage.ReadMessage(conn, time.Minute) //suckhttp.ReadRequest(context.Background(), conn, time.Minute)
		if err != nil {
			return err
		}
		l.Debug("handle", "new req")
		responsemsg, err := h.HandleBasic(requestmsg, l)
		if responsemsg == nil {
			responsemsg = &basicmessage.BasicMessage{Payload: []byte{byte(basicmessagetypes.OperationCodeInternalServerError)}}
		}
		l.Debug("handle", "req handled")
		if err != nil {
			if _, writeErr := conn.Write(responsemsg.ToByte()); writeErr != nil {
				l.Error("handle/Write", writeErr)
			}
			return err
		}
		_, err = conn.Write(responsemsg.ToByte())
		return err
	}
}
