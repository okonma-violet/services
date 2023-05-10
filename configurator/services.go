package main

import (
	"context"
	"errors"
	"net"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/big-larry/suckutils"
	"github.com/okonma-violet/connector"
	"github.com/okonma-violet/services/logs/logger"
	"github.com/okonma-violet/services/types/configuratortypes"
	"github.com/segmentio/fasthash/fnv1a"
)

type services struct {
	list  map[ServiceName]*service_state
	subs  subscriptionsier
	rwmux sync.RWMutex
}

type servicesier interface {
	getServiceState(ServiceName) service_stateier
}

func newServices(ctx context.Context, l logger.Logger, settingspath string, settingsCheckTicktime time.Duration, subs subscriptionsier) servicesier {
	servs := &services{list: make(map[ServiceName]*service_state), subs: subs}
	go servs.serveSettings(ctx, l, settingspath, settingsCheckTicktime)
	return servs
}

func (s *services) getServiceState(name ServiceName) service_stateier {
	s.rwmux.RLock()
	defer s.rwmux.RUnlock()
	return s.list[name]
}

func (s *services) serveSettings(ctx context.Context, l logger.Logger, settingspath string, ticktime time.Duration) {
	filestat, err := os.Stat(settingspath)
	if err != nil {
		panic("[os.stat] error: " + err.Error())
	}
	if err := s.readSettings(l, settingspath); err != nil {
		panic(suckutils.ConcatTwo("readsettings: ", err.Error()))
	}
	lastmodified := filestat.ModTime().Unix()
	ticker := time.NewTicker(ticktime)
	for {
		select {
		case <-ctx.Done():
			l.Debug("readsettings", "context done, exiting")
			ticker.Stop()
			return
		case <-ticker.C:
			fs, err := os.Stat(settingspath)
			if err != nil {
				l.Error("os.Stat", err)
			}
			lm := fs.ModTime().Unix()
			if lastmodified < lm {
				if err := s.readSettings(l, settingspath); err != nil {
					l.Error("readsettings", err)
					continue
				}
				lastmodified = lm
			}
		}
	}
}

// TODO: сервисы из мапы сейчас не трутся (возможна ситуация наличия в мапе сервиса с нулем разрешенных для запуска инстансов)
func (s *services) readSettings(l logger.Logger, settingspath string) error {
	data, err := os.ReadFile(settingspath)
	if err != nil {
		return err
	}

	lines := strings.Split(string(data), "\n")

	for n, line := range lines {
		if strings.HasPrefix(line, "#") {
			continue
		}
		ind := strings.Index(line, " ")

		settings_name := ServiceName(strings.ToLower((line)[:ind])) // перегоняем имя в строчные
		settings_hash := fnv1a.HashString32((line)[ind+1:])         // хэшим строку после имени сервиса
		var rawsettings_addrs []string

		state := s.list[settings_name]
		if state == nil { // система не знает про такой сервис
			if rawsettings_addrs = strings.Split((line)[ind+1:], " "); len(rawsettings_addrs) == 0 {
				continue
			}
			state = newServiceState(len(rawsettings_addrs))
			s.rwmux.Lock()
			s.list[settings_name] = state
			s.rwmux.Unlock()
		}
		if state.settings_hash == settings_hash { // если хэш в порядке, пропускаем эту строку
			continue
		}
		state.settings_hash = settings_hash

		if rawsettings_addrs = strings.Split((line)[ind+1:], " "); len(rawsettings_addrs) == 0 {
			continue
		}

		// TODO: оптимизировать эту всю херь ниже, если возможно

		settings_addrs := make([]*Address, 0, len(rawsettings_addrs)) // слайс для сверки адресов
		for _, a := range rawsettings_addrs {                         // читаем адреса из settings.conf = проверяем их на ошибки
			if addr := readAddress(a); addr != nil {
				settings_addrs = append(settings_addrs, addr)
			} else {
				l.Warning(settingspath, suckutils.ConcatFour("incorrect address at line ", strconv.Itoa(n+1), ": ", a))
			}
		}

		for i := 0; i < len(state.connections); i++ { // к нам приехал ревизор outer-портов
			var addrVerified bool

			for k := 0; k < len(settings_addrs); k++ {
				if state.connections[i].outerAddr.equalAsListeningAddr(*settings_addrs[k]) {
					addrVerified = true
					settings_addrs = append(settings_addrs[:k], settings_addrs[k+1:]...) // удаляем сошедшийся адрес
					break
				}
			}
			if !addrVerified { // адрес не нашелся в settings
				state.rwmux.Lock()
				state.connections[i].connector.Close(errors.New("settings.conf configuration changed"))
				state.connections = append(state.connections[:i], state.connections[i+1:]...)
				state.rwmux.Unlock()
				i--
			}
		}

		for i := 0; i < len(settings_addrs); i++ { // если остались адреса, еще не присутствующие в системе, мы их добавляем
			state.rwmux.Lock()
			newserv := newService(settings_name, *settings_addrs[i], l.NewSubLogger(suckutils.ConcatThree(string(settings_name), "-", strconv.Itoa(len(state.connections)))), s.subs)
			state.connections = append(state.connections, newserv)
			state.rwmux.Unlock()
			if newserv.name == ServiceName(configuratortypes.ConfServiceName) {
				reconnectReq <- newserv
			}
		}

	}

	return nil

}

type service_state struct {
	connections   []*service
	rwmux         sync.RWMutex
	settings_hash uint32 // хэш строки из файла настроек
}

type service_stateier interface {
	getAllOutsideAddrsWithStatus(configuratortypes.ServiceStatus) []*Address
	getAllServices() []*service
	initNewConnection(conn net.Conn, isLocalhosted bool, isConf bool) error
}

func newServiceState(conns_cap int) *service_state {
	return &service_state{connections: make([]*service, 0, conns_cap)}
}

func (state *service_state) getAllOutsideAddrsWithStatus(status configuratortypes.ServiceStatus) []*Address {
	if state == nil {
		return nil
	}
	addrs := make([]*Address, 0, len(state.connections))
	state.rwmux.RLock()
	defer state.rwmux.RUnlock()
	for i := 0; i < len(state.connections); i++ {
		if state.connections[i].isStatus(status) {
			addrs = append(addrs, &state.connections[i].outerAddr)
		}
	}
	return addrs
}

func (state *service_state) getAllServices() []*service {
	if state == nil {
		return nil
	}
	state.rwmux.RLock()
	defer state.rwmux.RUnlock()
	res := make([]*service, len(state.connections))
	copy(res, state.connections)
	return res
}

func (state *service_state) initNewConnection(conn net.Conn, isLocalhosted bool, isConf bool) error {
	if state == nil {
		return errors.New("unknown service") // да, неочевидно
	}

	if isConf && isLocalhosted {
		return errors.New(suckutils.ConcatThree("localhosted conf trying to connect from: ", conn.RemoteAddr().String(), ", conn denied"))
	}

	state.rwmux.Lock()
	defer state.rwmux.Unlock()

	var conn_host string
	if !isLocalhosted {
		conn_host = (conn.RemoteAddr().String())[:strings.Index(conn.RemoteAddr().String(), ":")]
	}

	for i := 0; i < len(state.connections); i++ {
		state.connections[i].statusmux.Lock()
		var con connector.Conn
		var err error

		if state.connections[i].status == configuratortypes.StatusOff {
			if !isLocalhosted {
				if state.connections[i].outerAddr.remotehost != conn_host {
					goto failure
				}
			}
			if con, err = connector.NewEpollConnector[connector.BasicMessage](conn, state.connections[i]); err != nil {
				goto failure
			}
			goto success
		}
	failure:
		state.connections[i].statusmux.Unlock()
		if err != nil {
			return err
		}
		continue
	success:
		if err = con.StartServing(); err != nil {
			con.ClearFromCache()
			goto failure
		}
		state.connections[i].connector = con
		if isConf {
			state.connections[i].status = configuratortypes.StatusOn
		} else {
			state.connections[i].status = configuratortypes.StatusSuspended
		}
		state.connections[i].l.Debug("Connection", "service approved, serving started")
		state.connections[i].statusmux.Unlock()
		if err := con.Send(connector.FormatBasicMessage([]byte{byte(configuratortypes.OperationCodeOK)})); err != nil {
			state.connections[i].connector.Close(err)
			return err
		}

		if isConf {
			time.Sleep(time.Second * 2)
			sendUpdateToOuterConf(state.connections[i])
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
	conn.Write(connector.FormatBasicMessage([]byte{byte(configuratortypes.OperationCodeNOTOK)}))
	return errors.New("no free conns for this service available") // TODO: где-то имя сервиса в логи вписать
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
	s.l.Debug("status", suckutils.ConcatThree("updated to \"", newStatus.String(), "\""))
}

func sendUpdateToOuterConf(serv *service) error {
	message := append(make([]byte, 0, 15), byte(configuratortypes.OperationCodeSubscribeToServices))
	if pubnames := serv.subs.getAllPubNames(); len(pubnames) != 0 { // шлем подписку на тошо у нас в пабах есть
		for _, pub_name := range pubnames {
			pub_name_byte := []byte(pub_name)
			message = append(append(message, byte(len(pub_name_byte))), pub_name_byte...)
		}
		if err := serv.connector.Send(connector.FormatBasicMessage(message)); err != nil {
			return err
		}
	}
	return serv.connector.Send(connector.FormatBasicMessage(append(append(make([]byte, 0, len(thisConfOuterPort)+1), byte(configuratortypes.OperationCodeMyOuterPort)), thisConfOuterPort...)))
}
