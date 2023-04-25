package netprotocol

import (
	"net"
	"strconv"
	"strings"
)

type NetProtocol byte

const (
	NetProtocolUnix         NetProtocol = 1
	NetProtocolTcp          NetProtocol = 2
	NetProtocolNil          NetProtocol = 3 // for non-listeners
	NetProtocolNonlocalUnix NetProtocol = 4
)

func (np NetProtocol) String() string {
	switch np {
	case NetProtocolTcp:
		return "tcp"
	case NetProtocolUnix:
		return "unix"
	case NetProtocolNil:
		return "nil"
	case NetProtocolNonlocalUnix:
		return "nonlocalunix"
	}
	return ""
}

func (np NetProtocol) Verify(addr string) bool {
	switch np {
	case NetProtocolTcp:
		sep := strings.LastIndex(addr, ":")
		if sep == -1 {
			return false
		}
		if net.ParseIP((addr)[:sep]) == nil {
			return false
		}
		if _, err := strconv.ParseUint((addr)[sep+1:], 10, 16); err != nil {
			return false
		}
		return true
	case NetProtocolUnix:
		if (addr)[:5] == "/tmp/" && (addr)[len(addr)-5:] == ".sock" {
			return true
		}
	case NetProtocolNonlocalUnix:
		if (addr)[:5] == "/tmp/" && (addr)[len(addr)-5:] == ".sock" {
			return true
		}
	case NetProtocolNil:
		if len(addr) == 0 {
			return true
		}

	}
	return false
}
