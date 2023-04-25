package configuratortypes

import (
	"errors"

	"github.com/okonma-violet/services/types/netprotocol"
)

type OperationCode byte

const (
	// []byte{opcode, statuscode}
	OperationCodeMyStatusChanged OperationCode = 2
	// []byte{opcode, len(pubname), pubname, len(pubname), pubname, ...}
	OperationCodeUnsubscribeFromServices OperationCode = 3
	// []byte{opcode, len(pubname), pubname, len(pubname), pubname, ...}
	OperationCodeSubscribeToServices OperationCode = 4
	// message := []byte{opcode, len(pubinfo), pubinfo, len(pubinfo), pubinfo, ...}
	// pubinfo := []byte{statuscode, len(addr), addr, pubname},
	OperationCodeUpdatePubs OperationCode = 6
	// []byte{opcode}
	OperationCodeGiveMeOuterAddr OperationCode = 8
	// []byte{opcode, len(addr), addr}
	OperationCodeSetOutsideAddr OperationCode = 9
	// []byte{opcode}
	OperationCodeImSupended OperationCode = 5
	// []byte{opcode}
	OperationCodePing OperationCode = 7
	// []byte{opcode}
	OperationCodeOK OperationCode = 1
	// []byte{opcode}
	OperationCodeNOTOK OperationCode = 10
	// []byte{opcode, port}
	OperationCodeMyOuterPort OperationCode = 11
)

func (op OperationCode) String() string {
	switch op {
	case OperationCodeMyStatusChanged:
		return "OperationCodeMyStatusChanged"
	case OperationCodeUnsubscribeFromServices:
		return "OperationCodeUnsubscribeFromServices"
	case OperationCodeSubscribeToServices:
		return "OperationCodeSubscribeToServices"
	case OperationCodeUpdatePubs:
		return "OperationCodeUpdatePubs"
	case OperationCodeGiveMeOuterAddr:
		return "OperationCodeGiveMeOuterAddr"
	case OperationCodeSetOutsideAddr:
		return "OperationCodeSetOutsideAddr"
	case OperationCodeImSupended:
		return "OperationCodeImSupended"
	case OperationCodePing:
		return "OperationCodePing"
	case OperationCodeOK:
		return "OperationCodeOK"
	case OperationCodeNOTOK:
		return "OperationCodeNOTOK"
	case OperationCodeMyOuterPort:
		return "OperationCodeMyOuterPort"
	}
	return ""
}

// no total length and opcode in return slice
func FormatOpcodeUpdatePubMessage(servname []byte, address []byte, newstatus ServiceStatus) []byte {
	if len(servname) == 0 || len(address) == 0 {
		return nil
	}
	return append(append(append(make([]byte, 0, len(address)+len(servname)+2), byte(newstatus), byte(len(address))), address...), servname...)
}

// servname, address, newstatus, error
func UnformatOpcodeUpdatePubMessage(raw []byte) ([]byte, []byte, ServiceStatus, error) {
	if len(raw) < 4 {
		return nil, nil, 0, errors.New("unformatted data")
	}
	status := ServiceStatus(raw[0])
	if len(raw) < (int(raw[1]) + 3) { // 1 for pubname
		return nil, nil, 0, errors.New("unformatted data")
	}
	return raw[2+int(raw[1]):], raw[2 : 2+int(raw[1])], status, nil
}

// with no len specified
func FormatAddress(netw netprotocol.NetProtocol, addr string) []byte {
	addr_byte := []byte(addr)
	formatted := make([]byte, 1, 1+len(addr_byte))
	formatted[0] = byte(netw)
	return append(formatted, addr_byte...)
}

// with no len specified
func UnformatAddress(raw []byte) (netprotocol.NetProtocol, string, error) {
	if len(raw) == 0 {
		return 0, "", errors.New("zero raw addr length")
	}
	return netprotocol.NetProtocol(raw[0]), string(raw[1:]), nil
}

type ServiceStatus byte

const (
	StatusOff       ServiceStatus = 0
	StatusSuspended ServiceStatus = 1
	StatusOn        ServiceStatus = 2
)

func (status ServiceStatus) String() string {
	switch status {
	case StatusOff:
		return "Off"
	case StatusOn:
		return "On"
	case StatusSuspended:
		return "Suspended"
	default:
		return "Undefined"
	}
}

type ServiceName string

const ConfServiceName ServiceName = "conf"

func SeparatePayload(payload []byte) [][]byte {
	if len(payload) == 0 {
		return nil
	}
	items := make([][]byte, 0, 4)
	for i := 0; i < len(payload); {
		length := int(payload[i])
		if i+length+1 > len(payload) {
			return nil
		}
		items = append(items, payload[i+1:i+length+1])
		i = length + 1 + i
	}
	return items
}

func ConcatPayload(pieces ...[]byte) []byte {
	if len(pieces) == 0 {
		return nil
	}
	length := 0
	for i := 0; i < len(pieces); i++ {
		length += len(pieces[i]) + 1
	}
	payload := make([]byte, 0, length)
	for _, piece := range pieces {
		if len(piece) == 0 {
			continue
		}
		payload = append(append(payload, byte(len(piece))), piece...)
	}
	return payload
}
