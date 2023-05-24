package main

import (
	"errors"
	"net"
	"strconv"
	"strings"
	"time"

	"github.com/big-larry/suckutils"
	"github.com/okonma-violet/services/types/netprotocol"
)

type ServiceName string

// TODO: при подрубе удаленных сервисов 100% будут проблемы, поэтому так пока нельзя делать
// TODO: рассмотреть возможность дифференцировать адрес и порт, навскидку непонятно нужно ли
type Address struct {
	netw       netprotocol.NetProtocol
	remotehost string
	port       string
	random     bool
}

func readAddress(rawline string) *Address {
	last_sep := strings.LastIndex(rawline, ":")
	if last_sep == -1 {
		return nil
	}
	addr := &Address{port: (rawline)[last_sep+1:]}
	first_sep := strings.Index(rawline, ":")
	if last_sep != first_sep {
		addr.remotehost = (rawline)[first_sep+1 : last_sep]
	}
	if addr.port == "*" {
		addr.random = true
	}

	switch (rawline)[:first_sep] {
	case "tcp":
		addr.netw = netprotocol.NetProtocolTcp
		if addr.random {
			return addr
		}
		if len(addr.remotehost) != 0 {
			if !addr.netw.Verify((rawline)[first_sep+1:]) {
				return nil
			} else {
				break
			}
		}
		if _, err := strconv.ParseUint(addr.port, 10, 16); err != nil {
			return nil
		}
	case "unix":
		addr.netw = netprotocol.NetProtocolUnix
		if addr.random {
			return addr
		}
		if !addr.netw.Verify(addr.port) {
			return nil
		}
	case "nil":
		addr.netw = netprotocol.NetProtocolNil
		if !addr.netw.Verify(addr.port) {
			return nil
		}
	default:
		return nil
	}

	return addr
}

// TODO: переписать эту херь?
func (a *Address) getListeningAddr() (netprotocol.NetProtocol, string, error) {
	if a == nil {
		return 0, "", errors.New("nil address struct")
	}
	if a.random { // TODO: подумать, не будет ли затыков в плане жесткой очередности опкодов при уже записанном мертвом адресе сервиса: явсуспенде > дай арес для прослушания > вот мой новый адрес > ансуспенд
		if randport, err := getFreePort(a.netw); err != nil {
			return 0, "", err
		} else {
			a.port = randport
		}
	}
	if a.netw == netprotocol.NetProtocolUnix || a.netw == netprotocol.NetProtocolNil || a.netw == netprotocol.NetProtocolTcp {
		return a.netw, a.port, nil
	}
	return 0, "", errors.New("unknown protocol")
}

// если адреса рандомны то всегда true
func (addr1 *Address) equalAsListeningAddr(addr2 Address) bool {
	if addr1.random == addr2.random {
		if addr1.random {
			return true
		}
		return addr1.netw == addr2.netw && addr1.remotehost == addr2.remotehost && addr1.port == addr2.port
	}
	return false
}

func getFreePort(netw netprotocol.NetProtocol) (string, error) {
	switch netw {
	case netprotocol.NetProtocolTcp:
		ln, err := net.Listen("tcp", "127.0.0.1:0")
		if err != nil {
			return "", err
		}
		ln.Close()
		return strconv.Itoa(ln.Addr().(*net.TCPAddr).Port), nil
	case netprotocol.NetProtocolUnix:
		return suckutils.Concat("/tmp/", strconv.FormatInt(time.Now().UnixNano(), 10), ".sock"), nil
	case netprotocol.NetProtocolNil:
		return "", nil
	}
	return "", errors.New("unknown protocol")
}

// DOES NOT FORMAT THE MESSAGE
func sendToMany(message []byte, recievers []*service) {
	if len(recievers) == 0 {
		return
	}
	for _, reciever := range recievers {
		if reciever == nil || reciever.connector == nil || reciever.connector.IsClosed() {
			continue
		}
		if err := reciever.connector.Send(message); err != nil {
			reciever.connector.Close(err)
		}
	}
}
