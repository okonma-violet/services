package basicmessagetypes

import "github.com/okonma-violet/services/basicmessage"

type OperationCode byte

const (
	//
	OperationCodeOK OperationCode = 255
	//
	OperationCodeBadRequest OperationCode = 254
	//
	OperationCodeForbidden OperationCode = 253
	//
	OperationCodeInternalServerError OperationCode = 252
	//
)

func (op OperationCode) String() string {
	switch op {
	case OperationCodeBadRequest:
		return "OperationCodeBadRequest"
	case OperationCodeForbidden:
		return "OperationCodeForbidden"
	case OperationCodeInternalServerError:
		return "OperationCodeInternalServerError"
	case OperationCodeOK:
		return "OperationCodeOK"
	}
	return ""
}

func IsOpOK(msg *basicmessage.BasicMessage) bool {
	return msg != nil && len(msg.Payload) > 0 && msg.Payload[0] == byte(OperationCodeOK)
}

func GetOpCode(msg *basicmessage.BasicMessage) OperationCode {
	if msg != nil && len(msg.Payload) > 0 {
		return OperationCode(msg.Payload[0])
	}
	return OperationCode(0)
}
