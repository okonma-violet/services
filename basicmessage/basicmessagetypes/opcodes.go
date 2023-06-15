package basicmessagetypes

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
