package main

import (
	"context"
	"flag"
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

	fs := flag.NewFlagSet("dhcprelay", flag.ExitOnError)
	var addr, relayTo string
	fs.StringVar(&addr, "addr", "192.168.2.225", "listen address")
	fs.StringVar(&relayTo, "relayto", "192.168.2.150", "relay to address")
	if err := fs.Parse(os.Args[1:]); err != nil {
		l.Error(err, "failed to parse flags")
		os.Exit(1)
	}
	ls, err := netip.ParseAddr(addr)
	if err != nil {
		l.Error(err, "failed to parse address")
		os.Exit(1)
	}

	r := &relay.Config{
		Logger:     l,
		Listener:   netip.AddrPortFrom(ls, 67),
		DHCPServer: &net.UDPAddr{IP: net.ParseIP(relayTo), Port: 6767},
	}
	ctx, done := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGHUP, syscall.SIGTERM)
	defer done()

	l.Info("starting server", "addr", r.Listener)
	l.Error(r.ListenAndServe(ctx), "done")
	l.Info("done")
}
