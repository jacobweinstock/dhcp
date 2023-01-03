// Package main provides a cmd line utility for the library.
package main

import (
	"context"
	"fmt"
	"net/url"

	"github.com/go-logr/logr"
	"github.com/tinkerbell/dhcp"
	"github.com/tinkerbell/dhcp/backend/file"
	"github.com/tinkerbell/dhcp/backend/noop"
	noopHandler "github.com/tinkerbell/dhcp/handler/noop"
	"github.com/tinkerbell/dhcp/handler/proxy"
	"github.com/tinkerbell/dhcp/handler/reservation"
	"inet.af/netaddr"
)

type cli struct {
	backend     string
	fileBackend backendFile
	handlers    []string
	// Addr is the ip and port to listen on.
	Addr netaddr.IPPort
	// Logger is the logger to use.
	Logger      logr.Logger
	opts        netboot
	DHCPEnabled bool
}

type backendFile struct {
	// Path is the path to the file.
	Path string
}

type netboot struct {
	NetbootEnabled bool
	// OTELEnabled is a flag to enable otel.
	OTELEnabled bool
	DHCPAddr    netaddr.IP
	IPXETFTP    netaddr.IPPort
	IPXEHTTP    *url.URL
	IPXEScript  *url.URL
}

func cliautomagic(ctx context.Context, c cli) ([]dhcp.Handler, error) {
	// 1. backend name, string
	// 2. handler name, string
	// 3. handler options:
	// logger
	// listen address
	// otel enabled
	// iPXE tftp
	// iPXE http
	// iPXE script url
	// netboot enabled
	var backend interface{}
	switch c.backend {
	case "noop":
		backend = noop.Handler{}
	case "file":
		fb, err := file.NewWatcher(c.Logger, c.fileBackend.Path)
		if err != nil {
			return nil, err
		}
		go fb.Start(ctx)
		backend = fb
	default:
		return nil, fmt.Errorf("unknown backend: %s", c.backend)
	}
	var h []dhcp.Handler
	for _, hdlr := range c.handlers {
		switch hdlr {
		case "noop":
			h = append(h, noopHandler.Handler{Log: c.Logger})
		case "reservation":
			be, ok := backend.(reservation.BackendReader)
			if !ok {
				return nil, fmt.Errorf("reservation handler requires a reservation backend")
			}
			r := &reservation.Handler{
				Log:         c.Logger.WithValues("handler", "reservation"),
				IPAddr:      c.opts.DHCPAddr,
				OTELEnabled: c.opts.OTELEnabled,
				Backend:     be,
				Netboot: reservation.Netboot{
					IPXEBinServerTFTP: c.opts.IPXETFTP,
					IPXEBinServerHTTP: c.opts.IPXEHTTP,
					IPXEScriptURL:     c.opts.IPXEScript,
					Enabled:           c.opts.NetbootEnabled,
				},
				DHCPEnabled: c.DHCPEnabled,
			}
			h = append(h, r)
		case "proxy":
			be, ok := backend.(proxy.BackendReader)
			if !ok {
				return nil, fmt.Errorf("proxy handler requires a proxy backend")
			}
			p := &proxy.Handler{
				Log:         c.Logger.WithValues("handler", "proxy"),
				IPAddr:      c.opts.DHCPAddr,
				OTELEnabled: c.opts.OTELEnabled,
				Backend:     be,
				Netboot: proxy.Netboot{
					IPXEBinServerTFTP: c.opts.IPXETFTP,
					IPXEBinServerHTTP: c.opts.IPXEHTTP,
					IPXEScriptURL:     c.opts.IPXEScript,
					Enabled:           c.opts.NetbootEnabled,
				},
			}
			h = append(h, p)
		default:
			return nil, fmt.Errorf("unknown handler: %s", hdlr)
		}
	}

	return h, nil
}
