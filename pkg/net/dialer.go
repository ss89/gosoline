package net

import (
	"fmt"
	"github.com/applike/gosoline/pkg/mon"
	"net"
	"strings"
)

func LookupHostDialer(logger mon.Logger, lookupAddress string) func() (net.Conn, error) {
	return func() (net.Conn, error) {
		lookupParts := strings.Split(lookupAddress, "://")
		if len(lookupParts) != 2 {
			return nil, fmt.Errorf("lookupAddress should be formatted like this: protocol://ip:port")
		}

		protocol := lookupParts[0]
		hostWithPort := lookupParts[1]

		portParts := strings.Split(hostWithPort, ":")
		if len(portParts) != 2 {
			return nil, fmt.Errorf("lookupAddress should be formatted like this: protocol://ip:port")
		}

		host := portParts[0]
		port := portParts[1]

		addresses, err := net.LookupHost(host)
		if err != nil {
			return nil, fmt.Errorf("can't lookup srv query for address %s: %w", hostWithPort, err)
		}
		if len(addresses) < 1 {
			return nil, fmt.Errorf("instance count mismatch. there should be at least one instance, found: %v", len(addresses))
		}

		address := fmt.Sprintf("%v:%v", addresses[0], port)
		logger.Infof("using address %s for host %s", address, lookupAddress)

		return net.Dial(protocol, address)
	}
}
