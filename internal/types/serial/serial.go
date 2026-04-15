package serial

import (
	"fmt"
	"strconv"
	"strings"
)

type Serial struct {
	num int64
}

func FromNum(numStr string) (Serial, error) {
	num, err := strconv.ParseInt(numStr, 10, 64)
	return Serial{num: num}, err
}

func FromHex(hexStr string) (Serial, error) {
	num, err := strconv.ParseInt(hexStr, 16, 64)
	return Serial{num: num}, err
}

func (s Serial) Hex() string {
	return strings.ToUpper(fmt.Sprintf("%08x", s.num))
}
