package reservation

import (
	"context"
	"net"
	"net/url"
	"testing"

	"github.com/go-logr/logr"
	"github.com/google/go-cmp/cmp"
	"github.com/insomniacslk/dhcp/dhcpv4"
	"github.com/insomniacslk/dhcp/iana"
	"github.com/tinkerbell/dhcp/data"
	"github.com/tinkerbell/dhcp/handler/option"
	oteldhcp "github.com/tinkerbell/dhcp/otel"
)

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
						dhcpv4.OptUserClass(option.Tinkerbell.String()),
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
						dhcpv4.OptUserClass(option.Tinkerbell.String()),
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
