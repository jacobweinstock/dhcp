// Package main provides a simple example of how to use multiple handlers.
package main

import (
	"context"
	"log"
	"net/url"
	"os"
	"os/signal"
	"syscall"

	"github.com/equinix-labs/otel-init-go/otelinit"
	"github.com/go-logr/logr"
	"github.com/go-logr/stdr"
	"github.com/tinkerbell/dhcp"
	"github.com/tinkerbell/dhcp/backend/file"
	"github.com/tinkerbell/dhcp/handler/proxy"
	"github.com/tinkerbell/dhcp/handler/reservation"
	"inet.af/netaddr"
)

func main() {
	ctx, done := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGHUP, syscall.SIGTERM)
	defer done()
	ctx, otelShutdown := otelinit.InitOpenTelemetry(ctx, "github.com/tinkerbell/dhcp")
	defer otelShutdown(ctx)

	l := stdr.New(log.New(os.Stdout, "", log.Lshortfile))
	l = l.WithName("github.com/tinkerbell/dhcp")
	// 1. create the backend
	// 2. create the handler(backend)
	// 3. create the listener(handler)
	backend, err := fileBackend(ctx, l, "./backend/file/testdata/example.yaml")
	if err != nil {
		panic(err)
	}
	l = l.WithValues("backend", backend.Name())

	reservationHandler := &reservation.Handler{
		Log:         l.WithValues("handler", "reservation"),
		IPAddr:      netaddr.IPv4(192, 168, 2, 59),
		OTELEnabled: true,
		Backend:     backend,
		Netboot: reservation.Netboot{
			IPXEBinServerTFTP: netaddr.IPPortFrom(netaddr.IPv4(192, 168, 2, 59), 69),
			IPXEBinServerHTTP: &url.URL{Scheme: "http", Host: "192.168.2.59:8080"},
			IPXEScriptURL:     &url.URL{Scheme: "http", Host: "192.168.1.94:9090", Path: "/auto.ipxe"},
			Enabled:           true,
		},
	}
	proxyHandler := &proxy.Handler{
		Log:    l.WithValues("handler", "proxyDHCP"),
		IPAddr: netaddr.IPv4(192, 168, 2, 59),
		Netboot: proxy.Netboot{
			IPXEBinServerTFTP: netaddr.IPPortFrom(netaddr.IPv4(192, 168, 2, 59), 69),
			IPXEBinServerHTTP: &url.URL{Scheme: "http", Host: "192.168.2.59:8080"},
			IPXEScriptURL:     &url.URL{Scheme: "http", Host: "192.168.1.94:9090", Path: "/auto.ipxe"},
			Enabled:           true,
		},
		OTELEnabled: true,
		Backend:     backend,
	}
	l = l.WithValues("handlers", []string{"reservation", "proxyDHCP"})

	l.Info("starting listener and server")
	listener := &dhcp.Listener{Addr: netaddr.IPPortFrom(netaddr.IPv4(0, 0, 0, 0), 67), IFName: "eno1"}
	if err := listener.ListenAndServe(ctx, reservationHandler, proxyHandler); err != nil {
		l.Error(err, "listener failed")
	}
	l.Info("shutting down")
	l.Info("done")
}

func fileBackend(ctx context.Context, l logr.Logger, f string) (reservation.BackendReader, error) {
	fb, err := file.NewWatcher(l, f)
	if err != nil {
		return nil, err
	}
	go fb.Start(ctx)
	return fb, nil
}
