package main

import (
	"errors"
	"net"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/big-larry/suckutils"
	"github.com/okonma-violet/confdecoder"
	"github.com/okonma-violet/connector"
	"github.com/okonma-violet/services/basicmessage"
	"github.com/okonma-violet/services/logs/logger"
	"github.com/okonma-violet/services/types/configuratortypes"
)

type services struct {
	list map[ServiceName]service_state
	subs subscriptionsier
}

func newServices(l logger.Logger, settingspath string, subs subscriptionsier) *services {
	servs := &services{list: make(map[ServiceName]service_state), subs: subs}
	if err := servs.readSettings(l, settingspath); err != nil {
		panic(err)
	}
	return servs
}

func (s *services) readSettings(l logger.Logger, settingspath string) error {
	pfd, err := confdecoder.ParseFile(settingspath)
	if err != nil {
		return err
	}
	confdecoder.SliceDelimeter = " "
	for i := 0; i < len(pfd.Rows); i++ {
		values := pfd.Rows[i].SplitValue()
		name := ServiceName(strings.ToLower(pfd.Rows[i].Key))
		state, ok := s.list[name]
		if !ok {
			state = newServiceState(len(values))
		}

		for k := 0; k < len(values); k++ {
			addr := readAddress(values[k])
			if addr == nil {
				return errors.New(suckutils.ConcatFour("incorrect addr at line: ", pfd.Rows[i].Key, " ", pfd.Rows[i].Value))
			}
			if addr.random {
				if name == ServiceName(configuratortypes.ConfServiceName) {
					return errors.New("outer configurator address can't be random")
				}
				state = append(state, newService(name, *addr, l.NewSubLogger(suckutils.ConcatThree(string(name), "-", strconv.Itoa(len(state)))), s.subs))
				continue
			}
			for g := 0; g < len(state); g++ {
				if addr.equalAsListeningAddr(state[g].outerAddr) {
					return errors.New(suckutils.ConcatFour("service ", string(name), " address duplicetes: ", values[k]))
				}
			}
			newserv := newService(name, *addr, l.NewSubLogger(suckutils.ConcatThree(string(name), "-", strconv.Itoa(len(state)))), s.subs)
			state = append(state, newserv)
			if name == ServiceName(configuratortypes.ConfServiceName) {
				reconnectReq <- newserv
			}
		}
		s.list[name] = state
	}
	return nil
}

type service_state []*service

func newServiceState(conns_cap int) service_state {
	return make(service_state, 0, conns_cap)
}

func (state service_state) getAllOutsideAddrsWithStatus(status configuratortypes.ServiceStatus) []*Address {
	if state == nil {
		return nil
	}
	addrs := make([]*Address, 0, len(state))
	for i := 0; i < len(state); i++ {
		if state[i].isStatus(status) {
			addrs = append(addrs, &state[i].outerAddr)
		}
	}
	return addrs
}

func (state service_state) initNewConnection(conn net.Conn, isLocalhosted bool, isConf bool) error {
	if state == nil {
		return errors.New("unknown service") // да, неочевидно
	}

	if isConf && isLocalhosted {
		return errors.New(suckutils.ConcatThree("localhosted configurator trying to connect from: ", conn.RemoteAddr().String(), ", conn denied"))
	}

	var conn_host string
	if !isLocalhosted {
		conn_host = (conn.RemoteAddr().String())[:strings.Index(conn.RemoteAddr().String(), ":")]
	}

	for i := 0; i < len(state); i++ {
		state[i].statusmux.Lock()
		var con connector.Conn
		var err error

		if state[i].status == configuratortypes.StatusOff {
			if !isLocalhosted {
				if state[i].outerAddr.remotehost != conn_host {
					goto failure
				}
			}
			if con, err = connector.NewEpollConnector[basicmessage.BasicMessage](conn, state[i]); err != nil {
				state[i].l.Error("Connection/NewEpollConnector", err)
				goto failure
			}
			goto success
		}
	failure:
		state[i].statusmux.Unlock()
		if err != nil {
			return err
		}
		continue
	success:
		if err = con.StartServing(); err != nil {
			con.ClearFromCache()
			state[i].l.Error("Connection/StartServing", err)
			goto failure
		}
		state[i].connector = con
		if isConf {
			state[i].status = configuratortypes.StatusOn
		} else {
			state[i].status = configuratortypes.StatusSuspended
		}
		state[i].l.Debug("Connection", "service approved, serving started")
		state[i].statusmux.Unlock()
		if err := con.Send(basicmessage.FormatBasicMessage([]byte{byte(configuratortypes.OperationCodeOK)})); err != nil {
			state[i].connector.Close(err)
			return err
		}
		if isConf {
			time.Sleep(time.Second * 2) //я не помню зачем, но зачем то надо
			sendOuterConfAfterConnMessages(state[i])
		}
		return nil
	}
	// TODO: !!!!!!!!!!!!!!!! если не нашли конфигуратор в стейте, то добавляем его туда. Проблема: откуда взять логгер и подписки
	// if isConf {
	// 	state.connections = append(state.connections, newService(
	// 		ServiceName(configuratortypes.ConfServiceName),
	// 		Address{netw: configuratortypes.NetProtocolTcp, remotehost: conn_host},
	// 		*loggs,
	// 	))
	// }
	conn.Write(basicmessage.FormatBasicMessage([]byte{byte(configuratortypes.OperationCodeNOTOK)}))
	return errors.New("no free conns for this service available")
}

type service struct {
	name      ServiceName
	outerAddr Address // адрес на котором сервис будет торчать наружу
	status    configuratortypes.ServiceStatus
	statusmux sync.RWMutex
	connector connector.Conn
	l         logger.Logger

	subs subscriptionsier
}

func (s *service) isStatus(status configuratortypes.ServiceStatus) bool {
	s.statusmux.RLock()
	defer s.statusmux.RUnlock()
	return s.status == status
}

func newService(name ServiceName, outerAddr Address, l logger.Logger, subs subscriptionsier) *service {
	return &service{name: name, outerAddr: outerAddr, l: l, subs: subs}
}

func (s *service) changeStatus(newStatus configuratortypes.ServiceStatus) {
	s.statusmux.Lock()
	defer s.statusmux.Unlock()

	if s.status == newStatus {
		//s.l.Warning("changeStatus", "trying to change already changed status")
		return
	}

	if s.status == configuratortypes.StatusOn || newStatus == configuratortypes.StatusOn { // иначе уведомлять о смене сорта нерабочести не нужно(eg: с выкл на суспенд)
		var addr string
		if len(s.outerAddr.remotehost) == 0 {
			addr = suckutils.ConcatTwo("127.0.0.1:", s.outerAddr.port)
		} else {
			addr = suckutils.ConcatThree(s.outerAddr.remotehost, ":", s.outerAddr.port)
		}
		s.subs.updatePub([]byte(s.name), configuratortypes.FormatAddress(s.outerAddr.netw, addr), newStatus, true)
	}
	s.status = newStatus
	s.l.Info("status", suckutils.ConcatThree("updated to \"", newStatus.String(), "\""))
}
