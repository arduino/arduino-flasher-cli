package service

import flasher "github.com/arduino/arduino-flasher-cli/rpc/cc/arduino/flasher/v1"

type flasherServerImpl struct {
	flasher.UnsafeFlasherServer // Force compile error for unimplemented methods
}

func NewFlasherServer() flasher.FlasherServer {
	return &flasherServerImpl{}
}
