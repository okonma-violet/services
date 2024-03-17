package universalservice_nonepoll

import (
	"context"
	"errors"
	"net"
	"os"
	"time"

	"github.com/big-larry/suckhttp"
	"github.com/big-larry/suckutils"
	"github.com/okonma-violet/dynamicworkerspool"
	"github.com/okonma-violet/services/basicmessage"
	"github.com/okonma-violet/services/basicmessage/basicmessagetypes"
	"github.com/okonma-violet/services/logs/logger"
)

const keepalive_readtimeout time.Duration = time.Minute

type BaseHandleFunc func(logger.Logger, net.Conn) error

type handleCreator interface {
	PrepareHandling(ctx context.Context, pubs_getter Publishers_getter) (BaseHandleFunc, Closer, error)
}

type httpmsghandler interface {
	HandleHTTP(*suckhttp.Request, logger.Logger) (*suckhttp.Response, error)
}

type basicmsghandler interface {
	// should close conn by itself
	HandleBasic(*basicmessage.BasicMessage, logger.Logger) (*basicmessage.BasicMessage, error)
}

func CreateHTTPHandleFunc(h httpmsghandler) BaseHandleFunc {
	return func(l logger.Logger, conn net.Conn) error {
		defer conn.Close()

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
		l.Debug("handle", suckutils.Concat("req handled, resp code: ", suckutils.Itoa(uint32(stcode))))

		if err != nil {
			if writeErr := response.Write(conn, time.Minute); writeErr != nil {
				l.Error("handle/Write", writeErr)
			}
			return err
		}
		return response.Write(conn, time.Minute)
	}
}

// HandleHTTP() returns err -> closes conn.
// suckhttp.ReadRequest() returns nontimeout err -> closes conn.
// uses dynamicworkerspool lib for serving conns.
// maxservegrtonetime - minaliveservegrt = num of dynamically spawned serving grts
func CreateHTTPKeepAliveHandleFunc(h httpmsghandler, minaliveservegrt, maxservegrtonetime int) (BaseHandleFunc, dynamicworkerspool.GrtPoolerCloser) {
	grtpool := dynamicworkerspool.NewPool(minaliveservegrt, maxservegrtonetime, threadKillingTimeout)

	return func(l logger.Logger, conn net.Conn) (err error) {
		err = grtpool.ScheduleWithTimeout(
			func() {
				for {
					request, err := suckhttp.ReadRequest(context.Background(), conn, keepalive_readtimeout)
					if err != nil {
						if os.IsTimeout(err) {
							if grtpool.IsClosed() {
								l.Debug("handle", "grtpool closed, closing conn")
								break
							}
							continue
						}
						l.Error("handle/ReadRequest", err)
						break
					}

					l.Debug("handle", "new req for "+request.Uri.Path)
					response, err := h.HandleHTTP(request, l)
					if response == nil {
						response = suckhttp.NewResponse(500, "Internal Server Error")
					}

					stcode, _ := response.GetStatus()
					l.Debug("handle", suckutils.ConcatTwo("req handled, resp code: ", suckutils.Itoa(uint32(stcode))))

					if err != nil {
						l.Error("handle/HandleHTTP", err)
						if err = response.Write(conn, time.Minute); err != nil {
							l.Error("handle/Write", err)
						}
						break
					}
					if err = response.Write(conn, time.Minute); err != nil {
						l.Error("handle/Write", err)
						break
					}
				}
				conn.Close()
				l.Debug("handle", "conn closed")
			},
			time.Minute)

		if err != nil {
			err = errors.New("grtpool schedule err: " + err.Error())
			conn.Close()
			l.Error("handle", errors.New("conn closed due to "+err.Error()))
		}
		return err
	}, grtpool
}

func CreateBasicHandleFunc(h basicmsghandler) BaseHandleFunc {
	return func(l logger.Logger, conn net.Conn) error {
		defer conn.Close()

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
