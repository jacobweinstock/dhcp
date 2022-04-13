// Package relay provides DHCP relay functionality.
package relay

import (
	"context"
	"fmt"
	"net"
	"net/netip"

	"github.com/go-logr/logr"
	"github.com/insomniacslk/dhcp/dhcpv4/server4"
)

type Config struct {
	Logger     logr.Logger
	Listener   netip.AddrPort
	DHCPServer *net.UDPAddr
}

func (c *Config) ListenAndServe(ctx context.Context) error {
	// for broadcast traffic we need to listen on all IPs
	addr := &net.UDPAddr{
		IP:   net.ParseIP("0.0.0.0"),
		Port: 67, //int(c.Listener.Port()),
	}
	c.Logger.Info("debugging", "port", c.Listener.Port(), "ip", c.Listener.Addr().String())
	c.Logger.Info("debugging", "dhcpServer", c.DHCPServer.String())

	i := getInterfaceByIP(c.Listener.Addr().String())
	srv, err := server4.NewServer(i, addr, c.handleFunc, server4.WithDebugLogger())
	if err != nil {
		return err
	}

	go func() {
		<-ctx.Done()
		fmt.Println("closing")
		_ = srv.Close()
	}()
	c.Logger.Info("starting DHCP server", "port", c.Listener.Port(), "interface", i)

	return srv.Serve()
}

// getInterfaceByIP returns the interface with the given IP address or an empty string.
func getInterfaceByIP(ip string) string {
	ifaces, err := net.Interfaces()
	if err != nil {
		return ""
	}
	for _, iface := range ifaces {
		addrs, err := iface.Addrs()
		if err != nil {
			continue
		}
		for _, addr := range addrs {
			if ipnet, ok := addr.(*net.IPNet); ok {
				if ipnet.IP.String() == ip {
					return iface.Name
				}
			}
		}
	}
	return ""
}
