// Package dhcp providers UDP listening and serving functionality.
package dhcp

import (
	"errors"
	"fmt"
	"net"
	"net/netip"
	"reflect"
	"sync"

	"github.com/imdario/mergo"
	"github.com/insomniacslk/dhcp/dhcpv4"
	"github.com/insomniacslk/dhcp/dhcpv4/server4"
	"github.com/tinkerbell/dhcp/handler/noop"
)

// ErrNoConn is an error im still not sure i want to use.
var ErrNoConn = &noConnError{}

type noConnError struct{}

func (e *noConnError) Error() string {
	return "no connection specified"
}

// Listener is a DHCPv4 server.
type Listener struct {
	Addr     netip.AddrPort
	srvMu    sync.Mutex
	srv      *server4.Server
	handlers []Handler
}

// Handler is the interface is responsible for responding to DHCP messages.
type Handler interface {
	// Handle is used for how to respond to DHCP messages.
	Handle(net.PacketConn, net.Addr, *dhcpv4.DHCPv4)
}

// Handler is the main handler passed to the server4 function.
// Internally it allows for multiple handlers to be defined.
// Each handler in l.handlers then executed for every received packet.
func (l *Listener) Handler(conn net.PacketConn, peer net.Addr, pkt *dhcpv4.DHCPv4) {
	for _, h := range l.handlers {
		h.Handle(conn, peer, pkt)
	}
}

// Serve will listen for DHCP messages on the given net.PacketConn and call the handler for each.
func Serve(c net.PacketConn, h ...Handler) error {
	srv := &Listener{handlers: h}

	return srv.Serve(c)
}

// Serve will listen for DHCP messages on the given net.PacketConn and call the handler in *Listener for each.
// If no handler is specified, a Noop handler will be used.
func (l *Listener) Serve(c net.PacketConn) error {
	if len(l.handlers) == 0 {
		l.handlers = append(l.handlers, &noop.Handler{})
	}
	if c == nil {
		return ErrNoConn
	}
	dhcp, err := server4.NewServer("", nil, l.Handler, server4.WithConn(c))
	if err != nil {
		return fmt.Errorf("failed to create dhcpv4 server: %w", err)
	}
	l.srvMu.Lock()
	l.srv = dhcp
	l.srvMu.Unlock()

	return l.srv.Serve()
}

// ListenAndServe will listen for DHCP messages and call the given handler for each.
func (l *Listener) ListenAndServe(h ...Handler) error {
	if len(h) == 0 {
		l.handlers = append(l.handlers, &noop.Handler{})
	}
	l.handlers = h
	defaults := &Listener{
		Addr: netip.AddrPortFrom(netip.AddrFrom4([4]byte{0, 0, 0, 0}), 67),
	}
	if err := mergo.Merge(l, defaults, mergo.WithTransformers(l)); err != nil {
		return fmt.Errorf("failed to merge defaults: %w", err)
	}

	addr := &net.UDPAddr{
		IP:   l.Addr.Addr().AsSlice(),
		Port: int(l.Addr.Port()),
	}
	conn, err := server4.NewIPv4UDPConn("", addr)
	if err != nil {
		return fmt.Errorf("failed to create udp connection: %w", err)
	}

	return l.Serve(conn)
}

// Shutdown closes the listener.
func (l *Listener) Shutdown() error {
	l.srvMu.Lock()
	defer l.srvMu.Unlock()
	if l.srv == nil {
		return errors.New("no server to shutdown")
	}

	return l.srv.Close()
}

// Transformer is used in mergo for merging structs.
func (l *Listener) Transformer(typ reflect.Type) func(dst, src reflect.Value) error {
	//nolint:revive // the switch is place holder to show when multiple transformers.
	switch typ { //nolint:gocritic // the switch is place holder to show when multiple transformers.
	case reflect.TypeOf(netip.AddrPort{}):
		return func(dst, src reflect.Value) error {
			if dst.CanSet() {
				m := dst.MethodByName("IsValid")
				addr := m.Call([]reflect.Value{})
				if addr[0].Bool() {
					dst.Set(src)
				}
			}

			return nil
		}
	}

	return nil
}
