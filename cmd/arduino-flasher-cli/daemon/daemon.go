// This file is part of arduino-flasher-cli.
//
// Copyright 2025 ARDUINO SA (http://www.arduino.cc/)
//
// This software is released under the GNU General Public License version 3,
// which covers the main part of arduino-flasher-cli.
// The terms of this license can be found at:
// https://www.gnu.org/licenses/gpl-3.0.en.html
//
// You can be released from the requirements of the above licenses by purchasing
// a commercial license. Buying such a license is mandatory if you want to
// modify or otherwise use the software for commercial activities involving the
// Arduino software without disclosing the source code of your own applications.
// To purchase a commercial license, send an email to license@arduino.cc.

package daemon

import (
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"os"
	"strings"
	"syscall"

	"github.com/spf13/cobra"
	"google.golang.org/grpc"

	"github.com/arduino/arduino-flasher-cli/cmd/feedback"
	"github.com/arduino/arduino-flasher-cli/cmd/i18n"
	flasher "github.com/arduino/arduino-flasher-cli/rpc/cc/arduino/flasher/v1"
)

func NewDaemonCommand(srv flasher.FlasherServer) *cobra.Command {
	var daemonPort string
	var maxGRPCRecvMsgSize int
	daemonCommand := &cobra.Command{
		Use:     "daemon",
		Short:   i18n.Tr("Run the Arduino Flasher CLI as a gRPC daemon."),
		Example: "  " + os.Args[0] + " daemon",
		Args:    cobra.NoArgs,
		Run: func(cmd *cobra.Command, args []string) {
			runDaemonCommand(srv, daemonPort, maxGRPCRecvMsgSize)
		},
	}

	daemonCommand.Flags().StringVar(&daemonPort,
		"port", "50052",
		i18n.Tr("The TCP port the daemon will listen to"))

	daemonCommand.Flags().IntVar(&maxGRPCRecvMsgSize,
		"max-grpc-recv-message-size", 16*1024*1024,
		i18n.Tr("Sets the maximum message size in bytes the daemon can receive"))

	return daemonCommand
}

func runDaemonCommand(srv flasher.FlasherServer, daemonPort string, maxGRPCRecvMsgSize int) {
	gRPCOptions := []grpc.ServerOption{}
	gRPCOptions = append(gRPCOptions, grpc.MaxRecvMsgSize(maxGRPCRecvMsgSize))
	s := grpc.NewServer(gRPCOptions...)

	// register the commands service
	flasher.RegisterFlasherServer(s, srv)

	daemonIP := "127.0.0.1"
	lis, err := net.Listen("tcp", fmt.Sprintf("%s:%s", daemonIP, daemonPort))
	if err != nil {
		// Invalid port, such as "Foo"
		var dnsError *net.DNSError
		if errors.As(err, &dnsError) {
			feedback.Fatal(i18n.Tr("Failed to listen on TCP port: %[1]s. %[2]s is unknown name.", daemonPort, dnsError.Name), feedback.ErrBadTCPPortArgument)
		}
		// Invalid port number, such as -1
		var addrError *net.AddrError
		if errors.As(err, &addrError) {
			feedback.Fatal(i18n.Tr("Failed to listen on TCP port: %[1]s. %[2]s is an invalid port.", daemonPort, addrError.Addr), feedback.ErrBadTCPPortArgument)
		}
		// Port is already in use
		var syscallErr *os.SyscallError
		if errors.As(err, &syscallErr) && errors.Is(syscallErr.Err, syscall.EADDRINUSE) {
			feedback.Fatal(i18n.Tr("Failed to listen on TCP port: %s. Address already in use.", daemonPort), feedback.ErrFailedToListenToTCPPort)
		}
		feedback.Fatal(i18n.Tr("Failed to listen on TCP port: %[1]s. Unexpected error: %[2]v", daemonPort, err), feedback.ErrFailedToListenToTCPPort)
	}

	// We need to retrieve the port used only if the user did not specify it
	// and let the OS choose it randomly, in all other cases we already know
	// which port is used.
	if daemonPort == "0" {
		address := lis.Addr()
		split := strings.Split(address.String(), ":")

		if len(split) <= 1 {
			feedback.Fatal(i18n.Tr("Invalid TCP address: port is missing"), feedback.ErrBadTCPPortArgument)
		}

		daemonPort = split[1]
	}

	feedback.PrintResult(daemonResult{
		IP:   daemonIP,
		Port: daemonPort,
	})

	if err := s.Serve(lis); err != nil {
		feedback.Fatal(fmt.Sprintf("Failed to serve: %v", err), feedback.ErrFailedToListenToTCPPort)
	}
}

type daemonResult struct {
	IP   string
	Port string
}

func (r daemonResult) Data() interface{} {
	return r
}

func (r daemonResult) String() string {
	j, _ := json.Marshal(r)
	return fmt.Sprintln(i18n.Tr("Daemon is now listening on %s:%s", r.IP, r.Port))
}
