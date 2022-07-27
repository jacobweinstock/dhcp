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

	proxyHandler := &proxy.Handler{
		Log:    l.WithValues("handler", "proxy"),
		IPAddr: netaddr.IPv4(192, 168, 2, 221),
		Netboot: proxy.Netboot{
			IPXEBinServerTFTP: netaddr.IPPortFrom(netaddr.IPv4(192, 168, 2, 221), 69),
			IPXEBinServerHTTP: &url.URL{Scheme: "http", Host: "192.168.2.221:8080"},
			IPXEScriptURL:     &url.URL{Scheme: "http", Host: "192.168.2.221:9090", Path: "/auto.ipxe"},
			Enabled:           true,
		},
		OTELEnabled: true,
		Backend:     backend,
	}
	reservationHandler := &reservation.Handler{
		Log:         l.WithValues("handler", "reservation"),
		IPAddr:      netaddr.IPv4(192, 168, 2, 221),
		OTELEnabled: true,
		Backend:     backend,
	}
	listener := &dhcp.Listener{Addr: netaddr.IPPortFrom(netaddr.IPv4(0, 0, 0, 0), 67)}
	go func() {
		<-ctx.Done()
		l.Error(listener.Shutdown(), "shutting down server")
	}()
	l.Info("starting server", "proxyHandler", proxyHandler.IPAddr, "reservationHandler", reservationHandler.IPAddr)
	l.Error(listener.ListenAndServe(proxyHandler, reservationHandler), "done")
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
