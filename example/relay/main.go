package main

import (
	"context"
	"log"
	"net"
	"net/netip"
	"os"
	"os/signal"
	"syscall"

	"github.com/go-logr/stdr"
	"github.com/tinkerbell/dhcp/relay"
)

func main() {
	l := stdr.New(log.New(os.Stdout, "", log.Lshortfile))
	l = l.WithName("github.com/tinkerbell/dhcp")
	relay := &relay.Config{
		Logger:     l,
		Listener:   netip.AddrPortFrom(netip.AddrFrom4([4]byte{192, 168, 2, 225}), 67),
		DHCPServer: &net.UDPAddr{IP: net.IPv4(192, 168, 0, 240), Port: 6767},
	}
	ctx, done := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGHUP, syscall.SIGTERM)
	defer done()

	l.Info("starting server", "addr", relay.Listener)
	l.Error(relay.ListenAndServe(ctx), "done")
	l.Info("done")
}
