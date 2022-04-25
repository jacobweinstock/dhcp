// Package relay provides DHCP relay functionality.
package relay

import (
	"context"
	"fmt"
	"net"
	"net/netip"
	"reflect"

	"github.com/go-logr/logr"
	"github.com/imdario/mergo"
	"github.com/insomniacslk/dhcp/dhcpv4/server4"
)

// Config for relaying DHCP.
type Config struct {
	Logger     logr.Logger
	Listener   netip.AddrPort
	DHCPServer *net.UDPAddr
	MaxHops    uint8
}

// ListenAndServe listens for DHCP request and starts the DHCP relay handler.
func (c *Config) ListenAndServe(ctx context.Context) error {
	defaults := &Config{
		Logger:   logr.Discard(),
		Listener: netip.AddrPortFrom(netip.AddrFrom4([4]byte{0, 0, 0, 0}), 67),
		MaxHops:  16,
	}
	err := mergo.Merge(c, defaults, mergo.WithTransformers(c))
	if err != nil {
		return err
	}
	// for broadcast traffic we need to listen on all IPs
	addr := &net.UDPAddr{
		IP:   net.ParseIP("0.0.0.0"),
		Port: int(c.Listener.Port()),
	}
	c.Logger.Info("debugging", "port", c.Listener.Port(), "ip", c.Listener.Addr().String(), "maxHops", c.MaxHops)
	c.Logger.Info("debugging", "dhcpServer", c.DHCPServer.String())

	i := getInterfaceByIP(c.Listener.Addr().String())
	conn, _ := net.ListenPacket("udp4", ":67")
	//i := ""
	srv, err := server4.NewServer(i, addr, c.handleFunc, server4.WithDebugLogger(), server4.WithConn(conn))
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

// Transformer for merging the netaddr.IPPort and logr.Logger structs.
func (c *Config) Transformer(typ reflect.Type) func(dst, src reflect.Value) error {
	switch typ { // nolint:gocritic,revive // There will almost certainly be more of these cases.
	case reflect.TypeOf(logr.Logger{}):
		return func(dst, src reflect.Value) error {
			if dst.CanSet() {
				isZero := dst.MethodByName("GetSink")
				result := isZero.Call(nil)
				if result[0].IsNil() {
					dst.Set(src)
				}
			}
			return nil
		}
	}

	return nil
}
