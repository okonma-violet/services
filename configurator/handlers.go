package main

import (
	"errors"
	"strconv"
	"strings"

	"github.com/big-larry/suckutils"
	"github.com/okonma-violet/connector"
	"github.com/okonma-violet/services/basicmessage"
	"github.com/okonma-violet/services/types/configuratortypes"
	"github.com/okonma-violet/services/types/netprotocol"
)

func (s *service) Handle(message *basicmessage.BasicMessage) error {

	if len(message.Payload) == 0 {
		return connector.ErrEmptyPayload
	}
	switch configuratortypes.OperationCode(message.Payload[0]) {
	case configuratortypes.OperationCodePing:
		s.l.Debug("New message", "OperationCodePing")
		if err := s.connector.Send(basicmessage.FormatBasicMessage([]byte{byte(configuratortypes.OperationCodePong)})); err != nil {
			return err
		}
		return nil
	case configuratortypes.OperationCodeGiveMeOuterAddr:
		s.l.Debug("New message", "OperationCodeGiveMeOuterAddr")
		if netw, addr, err := s.outerAddr.getListeningAddr(); err != nil {
			return errors.New(suckutils.ConcatTwo("getlisteningaddr err: ", err.Error()))
		} else {
			formatted_addr := configuratortypes.FormatAddress(netw, addr)
			if err := s.connector.Send(basicmessage.FormatBasicMessage(append(append(make([]byte, 0, len(formatted_addr)+2), byte(configuratortypes.OperationCodeSetOutsideAddr), byte(len(formatted_addr))), formatted_addr...))); err != nil {
				return err
			}
			return nil
		}
	case configuratortypes.OperationCodeSubscribeToServices:
		s.l.Debug("New message", "OperationCodeSubscribeToServices")
		raw_pubnames := configuratortypes.SeparatePayload(message.Payload[1:])
		if raw_pubnames == nil {
			return connector.ErrWeirdData
		}
		pubnames := make([]ServiceName, 0, len(raw_pubnames))
		for _, raw_pubname := range raw_pubnames {
			if len(raw_pubname) == 0 {
				return connector.ErrWeirdData
			}
			pubnames = append(pubnames, ServiceName(raw_pubname))
		}
		return s.subs.subscribe(s, pubnames...)

	case configuratortypes.OperationCodeUpdatePubs:
		s.l.Debug("New message", "OperationCodeUpdatePubs")
		if s.name == ServiceName(configuratortypes.ConfServiceName) {
			updates := configuratortypes.SeparatePayload(message.Payload[1:])
			if len(updates) != 0 {
				foo := s.connector.RemoteAddr().String()
				external_ip := (foo)[:strings.Index(foo, ":")]
				for _, update := range updates {
					pubname, raw_addr, status, err := configuratortypes.UnformatOpcodeUpdatePubMessage(update)
					if err != nil {
						s.l.Error("UnformatOpcodeUpdatePubMessage", err)
						return connector.ErrWeirdData
					}
					netw, addr, err := configuratortypes.UnformatAddress(raw_addr)
					if err != nil {
						s.l.Error("UnformatAddress", err)
						return connector.ErrWeirdData
					}
					switch netw {
					case netprotocol.NetProtocolUnix:
						netw = netprotocol.NetProtocolNonlocalUnix
					case netprotocol.NetProtocolTcp:
						if (addr)[:strings.Index(addr, ":")] == "127.0.0.1" {
							addr = suckutils.ConcatTwo(external_ip, (addr)[strings.Index(addr, ":"):])
						}
					}
					s.subs.updatePub(pubname, configuratortypes.FormatAddress(netw, addr), status, false)
				}
			}
		} else {
			return errors.New("not configurator, but sent OperationCodeUpdatePubs")
		}
	case configuratortypes.OperationCodeMyStatusChanged:
		s.l.Debug("New message", "OperationCodeMyStatusChanged")
		if len(message.Payload) < 2 {
			return connector.ErrWeirdData
		}
		s.changeStatus(configuratortypes.ServiceStatus(message.Payload[1]))
	case configuratortypes.OperationCodeMyOuterPort:
		s.l.Debug("New message", "OperationCodeMyOuterPort")
		if s.name == ServiceName(configuratortypes.ConfServiceName) {
			if len(message.Payload) < 2 {
				return connector.ErrWeirdData
			}
			if p, err := strconv.Atoi(string(message.Payload[1:])); err != nil || p == 0 {
				return connector.ErrWeirdData
			} else {
				s.statusmux.Lock()
				s.outerAddr.port = string(message.Payload)
				s.statusmux.Unlock()
			}
		} else {
			return errors.New("not configurator, but sent OperationCodeMyOuterPort")
		}
	default:
		s.l.Error("New message", connector.ErrWeirdData)
		return connector.ErrWeirdData
	}
	return nil
}

func (s *service) HandleClose(reason error) {
	if reason != nil {
		s.l.Warning("Connection", suckutils.ConcatTwo("closed, reason err: ", reason.Error()))
	} else {
		s.l.Warning("Connection", "closed, no reason")
	}
	s.changeStatus(configuratortypes.StatusOff)

	if s.name == ServiceName(configuratortypes.ConfServiceName) {
		reconnectReq <- s
	}
}
