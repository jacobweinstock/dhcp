package reservation

import (
	"context"
	"net"
	"net/url"
	"testing"

	"github.com/equinix-labs/otel-init-go/otelhelpers"
	"github.com/go-logr/logr"
	"github.com/google/go-cmp/cmp"
	"github.com/insomniacslk/dhcp/dhcpv4"
	"github.com/insomniacslk/dhcp/iana"
	"github.com/tinkerbell/dhcp/data"
	oteldhcp "github.com/tinkerbell/dhcp/otel"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/propagation"
	"inet.af/netaddr"
)

func TestArch(t *testing.T) {
	tests := map[string]struct {
		pkt  *dhcpv4.DHCPv4
		want iana.Arch
	}{
		"found": {
			pkt:  &dhcpv4.DHCPv4{Options: dhcpv4.OptionsFromList(dhcpv4.OptClientArch(iana.INTEL_X86PC))},
			want: iana.INTEL_X86PC,
		},
		"unknown": {
			pkt:  &dhcpv4.DHCPv4{Options: dhcpv4.OptionsFromList(dhcpv4.OptClientArch(iana.Arch(255)))},
			want: iana.Arch(255),
		},
		"unknown: opt 93 len 0": {
			pkt:  &dhcpv4.DHCPv4{},
			want: iana.Arch(255),
		},
	}
	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			got := arch(tt.pkt)
			if diff := cmp.Diff(tt.want, got); diff != "" {
				t.Fatal(diff)
			}
		})
	}
}

func TestBootfileAndNextServer(t *testing.T) {
	type args struct {
		mac     net.HardwareAddr
		uClass  UserClass
		opt60   string
		bin     string
		tftp    netaddr.IPPort
		ipxe    *url.URL
		iscript *url.URL
	}
	tests := map[string]struct {
		server       *Handler
		args         args
		otelEnabled  bool
		wantBootFile string
		wantNextSrv  net.IP
	}{
		"success bootfile only": {
			server: &Handler{Log: logr.Discard()},
			args: args{
				uClass:  Tinkerbell,
				iscript: &url.URL{Scheme: "http", Host: "localhost:8080", Path: "/auto.ipxe"},
			},
			wantBootFile: "http://localhost:8080/auto.ipxe",
			wantNextSrv:  nil,
		},
		"success httpClient": {
			server: &Handler{Log: logr.Discard()},
			args: args{
				mac:   net.HardwareAddr{0x01, 0x02, 0x03, 0x04, 0x05, 0x06},
				opt60: httpClient.String(),
				bin:   "snp.ipxe",
				ipxe:  &url.URL{Scheme: "http", Host: "localhost:8181"},
			},
			wantBootFile: "http://localhost:8181/snp.ipxe",
			wantNextSrv:  net.IPv4(0, 0, 0, 0),
		},
		"success userclass iPXE": {
			server: &Handler{Log: logr.Discard()},
			args: args{
				mac:    net.HardwareAddr{0x01, 0x02, 0x03, 0x04, 0x05, 0x07},
				uClass: IPXE,
				bin:    "unidonly.kpxe",
				tftp:   netaddr.IPPortFrom(netaddr.IPv4(192, 168, 6, 5), 69),
				ipxe:   &url.URL{Scheme: "tftp", Host: "192.168.6.5:69"},
			},
			wantBootFile: "tftp://192.168.6.5:69/unidonly.kpxe",
			wantNextSrv:  net.ParseIP("192.168.6.5"),
		},
		"success userclass iPXE with otel": {
			server:      &Handler{Log: logr.Discard(), OTELEnabled: true},
			otelEnabled: true,
			args: args{
				mac:    net.HardwareAddr{0x01, 0x02, 0x03, 0x04, 0x05, 0x07},
				uClass: IPXE,
				bin:    "unidonly.kpxe",
				tftp:   netaddr.IPPortFrom(netaddr.IPv4(192, 168, 6, 5), 69),
				ipxe:   &url.URL{Scheme: "tftp", Host: "192.168.6.5:69"},
			},
			wantBootFile: "tftp://192.168.6.5:69/unidonly.kpxe-00-23b1e307bb35484f535a1f772c06910e-d887dc3912240434-01",
			wantNextSrv:  net.ParseIP("192.168.6.5"),
		},
		"success default": {
			server: &Handler{Log: logr.Discard()},
			args: args{
				mac:  net.HardwareAddr{0x01, 0x02, 0x03, 0x04, 0x05, 0x07},
				bin:  "unidonly.kpxe",
				tftp: netaddr.IPPortFrom(netaddr.IPv4(192, 168, 6, 5), 69),
				ipxe: &url.URL{Scheme: "tftp", Host: "192.168.6.5:69"},
			},
			wantBootFile: "unidonly.kpxe",
			wantNextSrv:  net.ParseIP("192.168.6.5"),
		},
		"success otel enabled, no traceparent": {
			server: &Handler{Log: logr.Discard(), OTELEnabled: true},
			args: args{
				mac:  net.HardwareAddr{0x01, 0x02, 0x03, 0x04, 0x05, 0x07},
				bin:  "unidonly.kpxe",
				tftp: netaddr.IPPortFrom(netaddr.IPv4(192, 168, 6, 5), 69),
				ipxe: &url.URL{Scheme: "tftp", Host: "192.168.6.5:69"},
			},
			wantBootFile: "unidonly.kpxe",
			wantNextSrv:  net.ParseIP("192.168.6.5"),
		},
	}
	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			ctx := context.Background()
			if tt.otelEnabled {
				// set global propagator to tracecontext (the default is no-op).
				prop := propagation.NewCompositeTextMapPropagator(propagation.TraceContext{}, propagation.Baggage{})
				otel.SetTextMapPropagator(prop)
				ctx = otelhelpers.ContextWithTraceparentString(ctx, "00-23b1e307bb35484f535a1f772c06910e-d887dc3912240434-01")
			}
			bootfile, nextServer := tt.server.bootfileAndNextServer(ctx, tt.args.uClass, tt.args.opt60, tt.args.bin, tt.args.tftp, tt.args.ipxe, tt.args.iscript)
			if diff := cmp.Diff(tt.wantBootFile, bootfile); diff != "" {
				t.Fatal("bootfile", diff)
			}
			if diff := cmp.Diff(tt.wantNextSrv, nextServer); diff != "" {
				t.Fatal("nextServer", diff)
			}
		})
	}
}

func TestSetNetworkBootOpts(t *testing.T) {
	type args struct {
		in0 context.Context
		m   *dhcpv4.DHCPv4
		n   *data.Netboot
	}
	tests := map[string]struct {
		server *Handler
		args   args
		want   *dhcpv4.DHCPv4
	}{
		"netboot not allowed": {
			server: &Handler{Log: logr.Discard()},
			args: args{
				in0: context.Background(),
				m:   &dhcpv4.DHCPv4{},
				n:   &data.Netboot{AllowNetboot: false},
			},
			want: &dhcpv4.DHCPv4{ServerIPAddr: net.IPv4(0, 0, 0, 0), BootFileName: "/netboot-not-allowed"},
		},
		"netboot allowed": {
			server: &Handler{Log: logr.Discard(), Netboot: Netboot{IPXEScriptURL: &url.URL{Scheme: "http", Host: "localhost:8181", Path: "/01:02:03:04:05:06/auto.ipxe"}}},
			args: args{
				in0: context.Background(),
				m: &dhcpv4.DHCPv4{
					ClientHWAddr: net.HardwareAddr{0x01, 0x02, 0x03, 0x04, 0x05, 0x06},
					Options: dhcpv4.OptionsFromList(
						dhcpv4.OptUserClass(Tinkerbell.String()),
						dhcpv4.OptClassIdentifier("HTTPClient:xxxxx"),
						dhcpv4.OptClientArch(iana.EFI_X86_64_HTTP),
					),
				},
				n: &data.Netboot{AllowNetboot: true, IPXEScriptURL: &url.URL{Scheme: "http", Host: "localhost:8181", Path: "/01:02:03:04:05:06/auto.ipxe"}},
			},
			want: &dhcpv4.DHCPv4{BootFileName: "http://localhost:8181/01:02:03:04:05:06/auto.ipxe", Options: dhcpv4.OptionsFromList(
				dhcpv4.OptGeneric(dhcpv4.OptionVendorSpecificInformation, dhcpv4.Options{
					6:  []byte{8},
					69: oteldhcp.TraceparentFromContext(context.Background()),
				}.ToBytes()),
				dhcpv4.OptClassIdentifier("HTTPClient"),
			)},
		},
		"netboot not allowed, arch unknown": {
			server: &Handler{Log: logr.Discard(), Netboot: Netboot{IPXEScriptURL: &url.URL{Scheme: "http", Host: "localhost:8181", Path: "/01:02:03:04:05:06/auto.ipxe"}}},
			args: args{
				in0: context.Background(),
				m: &dhcpv4.DHCPv4{
					ClientHWAddr: net.HardwareAddr{0x01, 0x02, 0x03, 0x04, 0x05, 0x06},
					Options: dhcpv4.OptionsFromList(
						dhcpv4.OptUserClass(Tinkerbell.String()),
						dhcpv4.OptClientArch(iana.UBOOT_ARM64),
					),
				},
				n: &data.Netboot{AllowNetboot: true},
			},
			want: &dhcpv4.DHCPv4{ServerIPAddr: net.IPv4(0, 0, 0, 0), BootFileName: "/netboot-not-allowed"},
		},
	}
	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			s := &Handler{
				Log: tt.server.Log,
				Netboot: Netboot{
					IPXEBinServerTFTP: tt.server.Netboot.IPXEBinServerTFTP,
					IPXEBinServerHTTP: tt.server.Netboot.IPXEBinServerHTTP,
					IPXEScriptURL:     tt.server.Netboot.IPXEScriptURL,
					Enabled:           tt.server.Netboot.Enabled,
					UserClass:         tt.server.Netboot.UserClass,
				},
				IPAddr:  tt.server.IPAddr,
				Backend: tt.server.Backend,
			}
			gotFunc := s.setNetworkBootOpts(tt.args.in0, tt.args.m, tt.args.n)
			got := new(dhcpv4.DHCPv4)
			gotFunc(got)
			if diff := cmp.Diff(tt.want, got); diff != "" {
				t.Fatal(diff)
			}
		})
	}
}
