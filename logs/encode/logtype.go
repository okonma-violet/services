package encode

type LogType byte

const (
	Debug   LogType = 1
	Info    LogType = 2
	Warning LogType = 3
	Error   LogType = 4
)

func (lt LogType) String() string {
	switch lt {
	case Debug:
		return "DBG"
	case Info:
		return "INF"
	case Warning:
		return "WRN"
	case Error:
		return "ERR"
	}
	return "UNK"
}
func (lt LogType) Byte() byte {
	return byte(lt)
}
func (lt LogType) ByteStr() []byte {
	switch lt {
	case Debug:
		return []byte("DBG")
	case Info:
		return []byte("INF")
	case Warning:
		return []byte("WRN")
	case Error:
		return []byte("ERR")
	}
	return []byte("UNK")
}

const (
	ColorReset = "\033[0m"
	ColorWhite = "\033[97m"
)

func (lt LogType) Colorize() string {
	switch lt {
	case Debug:
		return "\033[32m" // Green
	case Info:
		return "\033[36m" // Gay
	case Warning:
		return "\033[33m" // Yellow
	case Error:
		return "\033[31m" // Red
	}
	return "\033[35m" // Purple
}
