package utils

import (
	"bytes"
	"encoding/hex"
	"encoding/json"
	"github.com/pkg/errors"
	"log"
	"net"
	"strconv"
)

func PanicIf(err error) {
	if err != nil {
		panic(err)
	}
}

func FatalIf(err error) {
	if err != nil {
		log.Fatal(err)
	}
}

func MustDecodeHex(s string) []byte {
	bin, err := hex.DecodeString(s)
	PanicIf(err)
	return bin
}

func NewBool(b bool) *bool {
	return &b
}

func ParseUdpAddr(addr string) (*net.UDPAddr, error) {
	host, port, err := net.SplitHostPort(addr)
	if err != nil {
		return nil, errors.Wrap(err, "parse addr error")
	}
	portInt, err := strconv.Atoi(port)
	if err != nil {
		return nil, errors.Wrap(err, "port not int")
	}
	return &net.UDPAddr{
		IP:   net.ParseIP(host),
		Port: portInt,
		Zone: "",
	}, nil
}

func ErrStr(err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
}

func Json(v interface{}) string {
	bin, err := json.Marshal(v)
	if err != nil {
		return ""
	}
	buf := bytes.NewBuffer([]byte{})
	err = json.Indent(buf, bin, "", " ")
	if err != nil {
		return ""
	}
	return buf.String()
}

func JsonRaw(v interface{}) string {
	bin, err := json.Marshal(v)
	if err != nil {
		return ""
	}
	return string(bin)
}
