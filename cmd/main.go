package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"net"
	"net/url"
	"os"
	"os/signal"
	"syscall"

	"github.com/equinix-labs/otel-init-go/otelinit"
	"github.com/go-logr/logr"
	"github.com/go-logr/zerologr"
	"github.com/go-playground/validator/v10"
	"github.com/peterbourgon/ff/v3"
	"github.com/peterbourgon/ff/v3/ffcli"
	"github.com/rs/zerolog"
	"github.com/tinkerbell/dhcp"
	"inet.af/netaddr"
)

type command struct {
	log      logr.Logger
	logLevel string
	backend  string
	Handlers handlers
	filePath string
	// Addr is the ip and port to listen on.
	Addr           IPXETFTP
	IFace          string
	NetbootEnabled bool
	// OTELEnabled is a flag to enable otel.
	OTELEnabled bool
	DHCPAddr    dhcpAddr
	IPXETFTP    IPXETFTP
	IPXEHTTP    IPXEHTTP
	IPXEScript  IPXEScript
	DHCPEnabled bool
}

func main() {
	exitCode := 0
	defer func() {
		os.Exit(exitCode)
	}()

	ctx, done := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGHUP, syscall.SIGTERM)
	defer done()
	ctx, otelShutdown := otelinit.InitOpenTelemetry(ctx, "github.com/tinkerbell/dhcp")
	defer otelShutdown(ctx)

	if err := execute(ctx, os.Args[1:]); err != nil && !errors.Is(err, context.Canceled) {
		fmt.Fprintf(os.Stderr, "{\"err\":\"%v\"}\n", err)
		exitCode = 1
	}
}

func commandDefaults() *command {
	var defaultDHCPAddr netaddr.IP
	if ip, err := netaddr.ParseIP(publicIPv4()); err == nil {
		defaultDHCPAddr = ip
	}
	return &command{
		logLevel: "info",
		backend:  "noop",
		Handlers: handlers{"noop"},
		DHCPAddr: dhcpAddr(defaultDHCPAddr),
	}
}

func execute(ctx context.Context, args []string) error {
	c := commandDefaults()
	fs := flag.NewFlagSet("dhcp", flag.ExitOnError)
	c.RegisterFlags(fs)
	cmd := &ffcli.Command{
		Name:       "dhcp",
		ShortUsage: "Run DHCP server",
		FlagSet:    fs,
		Options:    []ff.Option{ff.WithEnvVarPrefix("DHCP")},
		Exec: func(ctx context.Context, args []string) error {
			c.log = defaultLogger(c.logLevel)
			c.log = c.log.WithName("dhcp")
			if err := c.Validate(); err != nil {
				return err
			}

			return c.Run(ctx)
		},
	}
	if err := cmd.Parse(args); err != nil {
		return err
	}

	return cmd.Run(ctx)
}

func (c *command) Run(ctx context.Context) error {
	l := c.log.WithValues()
	// 1. create the backend
	// 2. create the handler(backend)
	// 3. create the listenerServer(handler)
	// 4. start the listenerServer
	ih := url.URL(c.IPXEHTTP)
	is := url.URL(c.IPXEScript)
	cl := cli{
		backend:     c.backend,
		fileBackend: backendFile{Path: c.filePath},
		handlers:    c.Handlers,
		Logger:      l,
		Addr:        netaddr.IPPort(c.Addr),
		opts: netboot{
			NetbootEnabled: c.NetbootEnabled,
			OTELEnabled:    false,
			DHCPAddr:       netaddr.IP(c.DHCPAddr),
			IPXETFTP:       netaddr.IPPort(c.IPXETFTP),
			IPXEHTTP:       &ih,
			IPXEScript:     &is,
		},
		DHCPEnabled: c.DHCPEnabled,
	}
	handlers, err := cliautomagic(ctx, cl)
	if err != nil {
		return err
	}
	if cl.Addr.IsZero() {
		cl.Addr = netaddr.IPPortFrom(netaddr.IPv4(0, 0, 0, 0), 67)
	}
	listener := &dhcp.Listener{Addr: cl.Addr, IFName: c.IFace}
	names := []string{}
	for _, h := range handlers {
		names = append(names, h.Name())
	}
	l.Info("starting dhcp server", "handlers", names)
	err = listener.ListenAndServe(ctx, handlers...)
	l.Info("shutting down dhcp server", "handlers", names)
	return err
}

// Validate checks the Command struct for validation errors.
func (c *command) Validate() error {
	return validator.New().Struct(c)
}

// "0.0.0.0:69",
// "0.0.0.0:8080",

// RegisterFlags registers a flag set for the ipxe command.
func (c *command) RegisterFlags(f *flag.FlagSet) {

	f.Var(&c.Handlers, "handlers", "comma separated list of handlers to use")
	f.Var(&c.IPXETFTP, "tftp-addr", "TFTP server address")
	f.Var(&c.Addr, "addr", "ip:port to listen on")
	f.Var(&c.IPXEHTTP, "http-addr", "HTTP server address")
	f.Var(&c.IPXEScript, "ipxe-script", "IPXE script to serve")
	f.BoolVar(&c.NetbootEnabled, "netboot-enabled", true, "Enable netboot")
	f.BoolVar(&c.DHCPEnabled, "dhcp-enabled", true, "Enable DHCP")
	f.BoolVar(&c.OTELEnabled, "otel-enabled", true, "Enable OpenTelemetry")
	f.Var(&c.DHCPAddr, "dhcp-addr", "DHCP server address, used in DHCP requests")
	f.StringVar(&c.backend, "backend", "file", "backend from which to pull DHCP data")
	f.StringVar(&c.filePath, "file-path", "dhcp.yaml", "path to data file for the file backend")
	// f.StringVar(&c.handlers, "handlers", "reservation", "comma separated string of handlers to use for DHCP requests")
	f.StringVar(&c.logLevel, "log-level", "info", "Log level")
	f.StringVar(&c.IFace, "iface", "eno1", "Interface to listen on")
	//f.Func("handlers", "comma separated string of handlers to use for DHCP requests", c.stringSliceVar)

}

// defaultLogger is a zerolog logr implementation.
func defaultLogger(level string) logr.Logger {
	zerolog.TimeFieldFormat = zerolog.TimeFormatUnixMs
	zerologr.NameFieldName = "logger"
	zerologr.NameSeparator = "/"

	zl := zerolog.New(os.Stdout)
	zl = zl.With().Caller().Timestamp().Logger()
	var l zerolog.Level
	switch level {
	case "debug":
		l = zerolog.DebugLevel
	default:
		l = zerolog.InfoLevel
	}
	zl = zl.Level(l)

	return zerologr.New(&zl)
}

func publicIPv4() string {
	if s, ok := os.LookupEnv("PUBLIC_IP"); ok {
		if a := net.ParseIP(s).To4(); a != nil {
			return a.String()
		}
		return ""
	}
	addrs, err := net.InterfaceAddrs()
	if err != nil {
		return ""
	}
	for _, addr := range addrs {
		ip, ok := addr.(*net.IPNet)
		if !ok {
			continue
		}
		v4 := ip.IP.To4()
		if v4 == nil || !v4.IsGlobalUnicast() {
			continue
		}

		return v4.String()
	}

	return ""
}
