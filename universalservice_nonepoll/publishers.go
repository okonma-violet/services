package universalservice_nonepoll

import (
	"context"
	"errors"
	"net"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/big-larry/suckhttp"
	"github.com/big-larry/suckutils"
	"github.com/okonma-violet/services/basicmessage"
	"github.com/okonma-violet/services/logs/logger"
	"github.com/okonma-violet/services/types/configuratortypes"
)

// TODO: рассмотреть идею о white и grey листах паблишеров

type address struct {
	netw string
	addr string
}

type publishers struct {
	list       map[ServiceName]*Publisher
	pubupdates chan pubupdate

	servstatus   *serviceStatus
	configurator *configurator
	l            logger.Logger

	//mux sync.Mutex
}

type Publisher struct {
	//conn        net.Conn
	servicename ServiceName
	addresses   []address
	current_ind int
	mux         sync.RWMutex
	l           logger.Logger
}

type Publishers_getter interface {
	Get(name ServiceName) *Publisher
}

// type Publisher_Sender interface {
// 	SendHTTP(request *suckhttp.Request) (response *suckhttp.Response, err error)
// }

type pubupdate struct {
	name   ServiceName
	addr   address
	status configuratortypes.ServiceStatus
}

var dialer *net.Dialer

const dialtimeout time.Duration = time.Second * 5
const pubscheckTicktime time.Duration = time.Second * 6

func newPublishers(l logger.Logger, servStatus *serviceStatus, pubNames []ServiceName) (*publishers, error) {
	p := &publishers{
		l:          l,
		list:       make(map[ServiceName]*Publisher, len(pubNames)),
		pubupdates: make(chan pubupdate, 1),
		servstatus: servStatus,
	}
	for _, pubname := range pubNames {
		if _, err := p.newPublisher(pubname); err != nil {
			return nil, err
		}
	}
	return p, nil

}

func (pubs *publishers) servePublishers(ctx context.Context, configurator *configurator) {
	pubs.configurator = configurator
	dialer = &net.Dialer{Timeout: dialtimeout}
	go pubs.publishersWorker(ctx)
}

func (pubs *publishers) update(pubname ServiceName, netw, addr string, status configuratortypes.ServiceStatus) {
	pubs.pubupdates <- pubupdate{name: pubname, addr: address{netw: netw, addr: addr}, status: status}
}

func (pubs *publishers) publishersWorker(ctx context.Context) {
	ticker := time.NewTicker(pubscheckTicktime)
loop:
	for {
		select {
		case <-ctx.Done():
			pubs.l.Debug("publishersWorker", "context done, exiting")
			return
		case update := <-pubs.pubupdates:
			// pubs.mux.Lock()
			// чешем мапу
			if pub, ok := pubs.list[update.name]; ok {
				// если есть в мапе
				// pubs.mux.Unlock()
				// чешем список адресов
				for i := 0; i < len(pub.addresses); i++ {
					// если нашли в списке адресов
					if update.addr.netw == pub.addresses[i].netw && update.addr.addr == pub.addresses[i].addr {
						// если нужно удалять из списка адресов
						if update.status == configuratortypes.StatusOff || update.status == configuratortypes.StatusSuspended {
							pub.mux.Lock()

							pub.addresses = append(pub.addresses[:i], pub.addresses[i+1:]...)
							// if pub.conn != nil {
							// 	if pub.conn.RemoteAddr().String() == update.addr.addr {
							// 		pub.conn.Close()
							// 		pubs.l.Debug("publishersWorker", suckutils.ConcatFour("due to update, closed conn to \"", string(update.name), "\" from ", update.addr.addr))
							// 	}
							// }

							if pub.current_ind > i {
								pub.current_ind--
							}

							pub.mux.Unlock()

							pubs.l.Debug("publishersWorker", suckutils.Concat("pub \"", string(update.name), "\" from ", update.addr.addr, " updated to ", update.status.String()))
							continue loop

						} else if update.status == configuratortypes.StatusOn { // если нужно добавлять в список адресов = варнинг, но может ложно стрельнуть при старте сервиса, когда при подключении к конфигуратору запрос на апдейт помимо хендшейка может отправить эта горутина по тикеру
							pubs.l.Warning("publishersWorker", suckutils.Concat("recieved pubupdate to status_on for already updated status_on for \"", string(update.name), "\" from ", update.addr.addr))
							continue loop

						} else { // если кривой апдейт
							pubs.l.Error("publishersWorker", errors.New(suckutils.Concat("unknown statuscode: ", strconv.Itoa(int(update.status)), "at update pub \"", string(update.name), "\" from ", update.addr.addr)))
							continue loop
						}
					}
				}
				// если не нашли в списке адресов

				// если нужно добавлять в список адресов
				if update.status == configuratortypes.StatusOn {
					pub.mux.Lock()
					pub.addresses = append(pub.addresses, update.addr)
					pubs.l.Debug("publishersWorker", suckutils.Concat("added new addr ", update.addr.netw, ":", update.addr.addr, " for pub ", string(pub.servicename)))
					pub.mux.Unlock()
					continue loop

				} else if update.status == configuratortypes.StatusOff || update.status == configuratortypes.StatusSuspended { // если нужно удалять из списка адресов = ошибка
					pubs.l.Error("publishersWorker", errors.New(suckutils.Concat("recieved pubupdate to status_suspend/off for already updated status_suspend/off for \"", string(update.name), "\" from ", update.addr.addr)))
					continue loop

				} else { // если кривой апдейт = ошибка
					pubs.l.Error("publishersWorker", errors.New(suckutils.Concat("unknown statuscode: ", strconv.Itoa(int(update.status)), "at update pub \"", string(update.name), "\" from ", update.addr.addr)))
					continue loop
				}

			} else { // если нет в мапе = ошибка и отписка
				// pubs.mux.Unlock()
				pubs.l.Error("publishersWorker", errors.New(suckutils.Concat("recieved update for non-publisher \"", string(update.name), "\", sending unsubscription")))

				pubname_byte := []byte(update.name)
				message := append(append(make([]byte, 0, 2+len(update.name)), byte(configuratortypes.OperationCodeUnsubscribeFromServices), byte(len(pubname_byte))), pubname_byte...)
				if err := pubs.configurator.send(basicmessage.FormatBasicMessage(message)); err != nil {
					pubs.l.Error("publishersWorker/configurator.Send", err)
				}
			}
		case <-ticker.C:
			empty_pubs := make([]string, 0, 1)
			empty_pubs_len := 0
			//pubs.rwmux.RLock()
			// pubs.mux.Lock()
			for pub_name, pub := range pubs.list {
				pub.mux.Lock()
				if len(pub.addresses) == 0 {
					empty_pubs = append(empty_pubs, string(pub_name))
					empty_pubs_len += len(pub_name)
				}
				pub.mux.Unlock()
			}
			// pubs.mux.Unlock()
			//pubs.rwmux.RUnlock()
			if len(empty_pubs) != 0 {
				pubs.servstatus.setPubsStatus(false)
				pubs.l.Warning("publishersWorker", suckutils.ConcatTwo("no publishers with names: ", strings.Join(empty_pubs, ", ")))
				message := make([]byte, 1, 1+empty_pubs_len+len(empty_pubs))
				message[0] = byte(configuratortypes.OperationCodeSubscribeToServices)
				for _, pubname := range empty_pubs {
					//check pubname len?
					message = append(append(message, byte(len(pubname))), []byte(pubname)...)
				}
				if err := pubs.configurator.send(basicmessage.FormatBasicMessage(message)); err != nil {
					pubs.l.Error("Publishers", errors.New(suckutils.ConcatTwo("sending subscription to configurator error: ", err.Error())))
				}
			} else {
				pubs.servstatus.setPubsStatus(true)
			}

		}
	}
}

func (pubs *publishers) GetAllPubNames() []ServiceName {
	// pubs.mux.Lock()
	// defer pubs.mux.Unlock()
	res := make([]ServiceName, 0, len(pubs.list))
	for pubname := range pubs.list {
		res = append(res, pubname)
	}
	return res
}

func (pubs *publishers) Get(servicename ServiceName) *Publisher {
	if pubs == nil {
		return nil
	}
	// pubs.mux.Lock()
	// defer pubs.mux.Unlock()
	return pubs.list[servicename]
}

func (pubs *publishers) newPublisher(name ServiceName) (*Publisher, error) {
	// pubs.mux.Lock()
	// defer pubs.mux.Unlock()

	if len(name) == 0 {
		return nil, errors.New("empty pubname")
	}

	if _, ok := pubs.list[name]; !ok {
		p := &Publisher{servicename: name, addresses: make([]address, 0, 1), l: pubs.l.NewSubLogger(string(name))}
		pubs.list[name] = p
		return p, nil
	} else {
		return nil, errors.New("publisher already initated")
	}
}

func CreateHTTPRequestFrom(method suckhttp.HttpMethod, uri string, recievedRequest *suckhttp.Request) (*suckhttp.Request, error) {
	req, err := suckhttp.NewRequest(method, uri)
	if err != nil {
		return nil, err
	}
	if recievedRequest == nil {
		return nil, errors.New("not set recievedRequest")
	}
	if v := recievedRequest.GetHeader("cookie"); v != "" {
		req.AddHeader("cookie", v)
	}
	return req, nil
}
func CreateHTTPRequest(method suckhttp.HttpMethod) (*suckhttp.Request, error) {
	return suckhttp.NewRequest(method, "")
}

func (pub *Publisher) SendHTTP(request *suckhttp.Request) (*suckhttp.Response, error) {
	var conn net.Conn
	var err error
	if conn, err = pub.connect(); err != nil {
		return nil, err
	}
	defer conn.Close()

	return request.Send(context.Background(), conn)
}

// example of usage: sending long-handled requests
func (pub *Publisher) SendBasicMessageWithTimeout(message *basicmessage.BasicMessage, timeout time.Duration) (*basicmessage.BasicMessage, error) {
	var conn net.Conn
	var err error
	if conn, err = pub.connect(); err != nil {
		return nil, err
	}
	defer conn.Close()

	if _, err = conn.Write(message.ToByte()); err != nil {
		// pub.l.Error("Send", err)
		return nil, err
	}
	return basicmessage.ReadMessage(conn, timeout)
}

func (pub *Publisher) SendBasicMessage(message *basicmessage.BasicMessage) (*basicmessage.BasicMessage, error) {
	return pub.SendBasicMessageWithTimeout(message, time.Minute)
}

func (pub *Publisher) connect() (net.Conn, error) {
	pub.mux.RLock()
	defer pub.mux.RUnlock()

	var conn net.Conn
	var err error
	for i := 0; i < len(pub.addresses); i++ {
		if pub.current_ind == len(pub.addresses) {
			pub.current_ind = 0
		}
		if conn, err = dialer.Dial(pub.addresses[pub.current_ind].netw, pub.addresses[pub.current_ind].addr); err != nil {
			pub.l.Error("connect/Dial", err)
			pub.current_ind++
		} else {
			pub.l.Debug("connect/Dial", suckutils.ConcatTwo("Connected to ", conn.RemoteAddr().String()))
			return conn, nil
		}
	}
	return nil, errors.New("no available/alive pub's addresses")
}
