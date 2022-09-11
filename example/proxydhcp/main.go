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

	// listener := &dhcp.Listener{Addr: netaddr.IPPortFrom(netaddr.IPv4(0, 0, 0, 0), 67), Reuseport: true}
	listener2 := &dhcp.Listener{Addr: netaddr.IPPortFrom(netaddr.IPv4(0, 0, 0, 0), 67), IFName: ""}

	go func() {
		/*
			reservationHandler := &reservation.Handler{
				Log:         l.WithValues("handler", "reservation"),
				IPAddr:      netaddr.IPv4(192, 168, 1, 94),
				OTELEnabled: true,
				Backend:     backend,
			}
			l.Info("starting server", "handler", "reservationHandler", "addr", listener.Addr)
			l.Error(listener.ListenAndServe(reservationHandler), "done")
		*/

	}()
	go func() {
		proxyHandler := &proxy.Handler{
			Log:    l.WithValues("handler", "proxy"),
			IPAddr: netaddr.IPv4(192, 168, 1, 94),
			Netboot: proxy.Netboot{
				IPXEBinServerTFTP: netaddr.IPPortFrom(netaddr.IPv4(192, 168, 1, 94), 69),
				IPXEBinServerHTTP: &url.URL{Scheme: "http", Host: "192.168.1.94:8080"},
				IPXEScriptURL:     &url.URL{Scheme: "http", Host: "192.168.1.94:9090", Path: "/auto.ipxe"},
				Enabled:           true,
			},
			OTELEnabled: true,
			Backend:     backend,
		}
		l.Info("starting server", "handler", "proxyHandler", "addr", listener2.Addr)
		l.Error(listener2.ListenAndServe(proxyHandler), "done")
		done()
	}()

	<-ctx.Done()
	l.Info("shutting down")
	// l.Error(listener.Shutdown(), "shutting down server")
	l.Error(listener2.Shutdown(), "shutting down server")
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
