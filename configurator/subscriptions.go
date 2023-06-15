package main

import (
	"context"
	"errors"
	"strconv"
	"sync"

	"github.com/big-larry/suckutils"
	"github.com/okonma-violet/services/basicmessage"
	"github.com/okonma-violet/services/logs/logger"
	"github.com/okonma-violet/services/types/configuratortypes"
	"github.com/okonma-violet/services/types/netprotocol"
)

type subscriptions struct {
	subs_list map[ServiceName][]*service
	sync.RWMutex
	servs       *services
	pubsUpdates chan pubStatusUpdate

	l logger.Logger
}

type subscriptionsier interface {
	updatePub([]byte, []byte, configuratortypes.ServiceStatus, bool) error
	subscribe(*service, ...ServiceName) error
	unsubscribe(ServiceName, *service) error
	getSubscribers(ServiceName) []*service
	getAllPubNames() []ServiceName
}

func newSubscriptions(ctx context.Context, l logger.Logger, pubsUpdatesQueue int, servs *services, ownSubscriptions ...ServiceName) *subscriptions {
	if pubsUpdatesQueue == 0 {
		panic("pubsUpdatesQueue must be > 0")
	}
	subs := &subscriptions{subs_list: make(map[ServiceName][]*service), servs: servs, pubsUpdates: make(chan pubStatusUpdate, pubsUpdatesQueue), l: l}
	go subs.pubsUpdatesSendingWorker(ctx)
	return subs
}

type pubStatusUpdate struct {
	servicename []byte
	address     []byte
	status      configuratortypes.ServiceStatus
	sendToConfs bool
}

func (subs *subscriptions) pubsUpdatesSendingWorker(ctx context.Context) {
	if subs.pubsUpdates == nil {
		panic("subs.pubUpdates chan is nil")
	}
	for {
		select {
		case <-ctx.Done():
			subs.l.Debug("pubStatusUpdater", suckutils.ConcatTwo("context done, exiting. unhandled updates: ", strconv.Itoa(len(subs.pubsUpdates))))
			// можно вычистить один раз канал апдейтов для разблокировки хэндлеров, но зачем
			return
		case update := <-subs.pubsUpdates:
			if len(update.servicename) == 0 || len(update.address) == 0 {
				subs.l.Error("pubStatusUpdater", errors.New("unformatted update, skipped"))
				continue
			}
			if subscriptors := subs.getSubscribers(ServiceName(update.servicename)); len(subscriptors) != 0 {
				payload := configuratortypes.ConcatPayload(configuratortypes.FormatOpcodeUpdatePubMessage(update.servicename, update.address, update.status))
				message := append(append(make([]byte, 0, len(payload)+1), byte(configuratortypes.OperationCodeUpdatePubs)), payload...)
				if update.sendToConfs {
					sendToMany(basicmessage.FormatBasicMessage(message), subscriptors)
				} else {
					for _, subscriptor := range subscriptors {
						if subscriptor == nil || subscriptor.connector == nil || subscriptor.connector.IsClosed() || subscriptor.name == ServiceName(configuratortypes.ConfServiceName) {
							continue
						}
						if err := subscriptor.connector.Send(basicmessage.FormatBasicMessage(message)); err != nil {
							subscriptor.connector.Close(err)
						}
					}
				}
			}
		}
	}
}

func (subs *subscriptions) updatePub(servicename []byte, address []byte, newstatus configuratortypes.ServiceStatus, sendUpdateToConfs bool) error {
	if len(servicename) == 0 || len(address) == 0 {
		return errors.New("empty/nil servicename/address")
	}
	subs.pubsUpdates <- pubStatusUpdate{servicename: servicename, address: address, status: newstatus, sendToConfs: sendUpdateToConfs}
	return nil
}

const maxFreeSpace int = 3 // reslice subscriptions.services, when cap - len > maxFreeSpace

// no err when already subscribed
func (subs *subscriptions) subscribe(sub *service, pubnames ...ServiceName) error {
	if sub == nil {
		return errors.New("nil sub")
	}

	if len(pubnames) == 0 {
		return errors.New("empty pubnames")
	}
	sendToConfs := sub.name != ServiceName(configuratortypes.ConfServiceName)

	formatted_updateinfos := make([][]byte, 0, len(pubnames)+2)                                                          // alive local pubs
	confs_message := append(make([]byte, 0, len(pubnames)*15), byte(configuratortypes.OperationCodeSubscribeToServices)) // subscription to send to other confs

	subs.Lock()

	for _, pubname := range pubnames {
		if sub.name == pubname && sub.name != ServiceName(configuratortypes.ConfServiceName) { // avoiding self-subscription
			subs.Unlock()
			return errors.New("service trying subscribe to itself")
		}

		pubname_byte := []byte(pubname)
		if sendToConfs {
			confs_message = append(append(confs_message, byte(len(pubname_byte))), pubname_byte...)
		}

		if _, ok := subs.subs_list[pubname]; ok {
			for _, alreadysubbed := range subs.subs_list[pubname] {
				if alreadysubbed == sub { // already subscribed
					goto sending_pubaddrs_to_sub
				}
			}
			if cap(subs.subs_list[pubname]) == len(subs.subs_list[pubname]) || (cap(subs.subs_list[pubname])-len(subs.subs_list[pubname])) > maxFreeSpace { // reslice, also allocation here if not yet
				subs.subs_list[pubname] = append(make([]*service, 0, len(subs.subs_list[pubname])+maxFreeSpace+1), subs.subs_list[pubname]...)
			}
			subs.subs_list[pubname] = append(subs.subs_list[pubname], sub)

			sub.l.Debug("subs", suckutils.ConcatThree("subscribed to \"", string(pubname), "\""))
		} else {
			subs.subs_list[pubname] = append(make([]*service, 1), sub)
		}
	sending_pubaddrs_to_sub:
		if state, ok := subs.servs.list[pubname]; ok { // getting alive local pubs
			addrs := state.getAllOutsideAddrsWithStatus(configuratortypes.StatusOn)
			if len(addrs) != 0 {
				for _, addr := range addrs {
					var concatted_address string
					if addr.netw == netprotocol.NetProtocolTcp {
						if len(addr.remotehost) == 0 {
							concatted_address = suckutils.ConcatTwo("127.0.0.1:", addr.port)
						} else {
							concatted_address = suckutils.ConcatThree(addr.remotehost, ":", addr.port)
						}
					} else {
						concatted_address = addr.port
					}

					formatted_updateinfos = append(formatted_updateinfos, configuratortypes.FormatOpcodeUpdatePubMessage(pubname_byte, configuratortypes.FormatAddress(addr.netw, concatted_address), configuratortypes.StatusOn))
				}
			} else {
				sub.l.Warning("subs", suckutils.ConcatThree("no alive local services with name \"", string(pubname), "\""))
			}
		}
	}

	subs.Unlock()

	if sendToConfs {
		if confs_state, ok := subs.servs.list[ServiceName(configuratortypes.ConfServiceName)]; ok { // sending subscription to other confs
			if len(confs_state) != 0 {
				sendToMany(basicmessage.FormatBasicMessage(confs_message), confs_state)
			}
		}
	}

	if len(formatted_updateinfos) != 0 {
		updateinfos := configuratortypes.ConcatPayload(formatted_updateinfos...)
		message := append(make([]byte, 1, len(updateinfos)+1), updateinfos...)
		message[0] = byte(configuratortypes.OperationCodeUpdatePubs)
		return sub.connector.Send(basicmessage.FormatBasicMessage(message))
	}
	return nil
}

// no err when not subscribed
func (subs *subscriptions) unsubscribe(pubName ServiceName, sub *service) error {
	if sub == nil {
		return errors.New("nil sub")
	}

	if len(pubName) == 0 {
		return errors.New("empty pubname")
	}

	subs.Lock()
	defer subs.Unlock()

	if _, ok := subs.subs_list[pubName]; ok {
		for i := 0; i < len(subs.subs_list[pubName]); i++ {
			if subs.subs_list[pubName][i] == sub {
				subs.subs_list[pubName] = append(subs.subs_list[pubName][:i], subs.subs_list[pubName][i+1:]...)
				sub.l.Debug("subs", suckutils.ConcatTwo("unsubscribed from service ", string(pubName)))
				break
			}
		}
	}
	if len(subs.subs_list[pubName]) == 0 { // delete pub from map if no subs here
		delete(subs.subs_list, pubName)
	} else if cap(subs.subs_list[pubName])-len(subs.subs_list[pubName]) > maxFreeSpace { // reslice after overflow
		subs.subs_list[pubName] = append(make([]*service, 0, len(subs.subs_list[pubName])+maxFreeSpace), subs.subs_list[pubName]...)
	}
	return nil
}

func (subs *subscriptions) getSubscribers(pubName ServiceName) []*service {
	if len(pubName) == 0 {
		return nil
	}

	subs.RLock()
	defer subs.RUnlock()

	if _, ok := subs.subs_list[pubName]; ok {
		ressubs := make([]*service, 0, len(subs.subs_list[pubName]))
		for _, sub := range subs.subs_list[pubName] {
			if sub != nil {
				ressubs = append(ressubs, sub)
			}
		}
		return ressubs
	}
	return nil
}

func (subs *subscriptions) getAllPubNames() []ServiceName {
	subs.RLock()
	defer subs.RUnlock()

	pubnames := make([]ServiceName, 0, len(subs.subs_list))
	for pubname := range subs.subs_list {
		pubnames = append(pubnames, pubname)
	}
	return pubnames
}
