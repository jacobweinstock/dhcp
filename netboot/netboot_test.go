package netboot

import (
	"bytes"
	"context"
	"errors"
	"net"
	"net/netip"
	"net/url"
	"testing"

	"github.com/equinix-labs/otel-init-go/otelhelpers"
	"github.com/google/go-cmp/cmp"
	"github.com/insomniacslk/dhcp/dhcpv4"
	"github.com/insomniacslk/dhcp/iana"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/trace"
)

func TestBootfileAndNextServer(t *testing.T) {
	type args struct {
		mac             net.HardwareAddr
		uClass          UserClass
		customUserClass UserClass
		opt60           ClientType
		bin             string
		tftp            netip.AddrPort
		ipxe            *url.URL
		iscript         *url.URL
	}
	tests := map[string]struct {
		args         args
		otelEnabled  bool
		wantBootFile string
		wantNextSrv  net.IP
	}{
		"success bootfile only": {
			args: args{
				uClass:  Tinkerbell,
				iscript: &url.URL{Scheme: "http", Host: "localhost:8080", Path: "/auto.ipxe"},
			},
			wantBootFile: "http://localhost:8080/auto.ipxe",
			wantNextSrv:  net.IPv4(127, 0, 0, 1),
		},
		"success httpClient": {
			args: args{
				mac:   net.HardwareAddr{0x01, 0x02, 0x03, 0x04, 0x05, 0x06},
				opt60: HTTPClient,
				bin:   "snp.ipxe",
				ipxe:  &url.URL{Scheme: "http", Host: "localhost:8181"},
			},
			wantBootFile: "http://localhost:8181/snp.ipxe",
			wantNextSrv:  net.IPv4(0, 0, 0, 0),
		},
		"success userclass iPXE": {
			args: args{
				mac:    net.HardwareAddr{0x01, 0x02, 0x03, 0x04, 0x05, 0x07},
				uClass: IPXE,
				bin:    "unidonly.kpxe",
				tftp:   netip.AddrPortFrom(netip.AddrFrom4([4]byte{192, 168, 6, 5}), 69),
				ipxe:   &url.URL{Scheme: "tftp", Host: "192.168.6.5:69"},
			},
			wantBootFile: "tftp://192.168.6.5:69/unidonly.kpxe",
			wantNextSrv:  net.ParseIP("192.168.6.5"),
		},
		"success userclass iPXE with otel": {
			otelEnabled: true,
			args: args{
				mac:    net.HardwareAddr{0x01, 0x02, 0x03, 0x04, 0x05, 0x07},
				uClass: IPXE,
				bin:    "unidonly.kpxe",
				tftp:   netip.AddrPortFrom(netip.AddrFrom4([4]byte{192, 168, 6, 5}), 69),
				ipxe:   &url.URL{Scheme: "tftp", Host: "192.168.6.5:69"},
			},
			wantBootFile: "tftp://192.168.6.5:69/unidonly.kpxe-00-23b1e307bb35484f535a1f772c06910e-d887dc3912240434-01",
			wantNextSrv:  net.ParseIP("192.168.6.5"),
		},
		"success default": {
			args: args{
				mac:  net.HardwareAddr{0x01, 0x02, 0x03, 0x04, 0x05, 0x07},
				bin:  "unidonly.kpxe",
				tftp: netip.AddrPortFrom(netip.AddrFrom4([4]byte{192, 168, 6, 5}), 69),
				ipxe: &url.URL{Scheme: "tftp", Host: "192.168.6.5:69"},
			},
			wantBootFile: "unidonly.kpxe",
			wantNextSrv:  net.ParseIP("192.168.6.5"),
		},
		"success otel enabled, no traceparent": {
			args: args{
				mac:  net.HardwareAddr{0x01, 0x02, 0x03, 0x04, 0x05, 0x07},
				bin:  "unidonly.kpxe",
				tftp: netip.AddrPortFrom(netip.AddrFrom4([4]byte{192, 168, 6, 5}), 69),
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
			bootfile, nextServer := BootfileAndNextServer(ctx, tt.args.uClass, tt.args.customUserClass, tt.args.opt60, tt.args.bin, tt.args.tftp, tt.args.ipxe, tt.args.iscript, tt.otelEnabled)
			if diff := cmp.Diff(bootfile, tt.wantBootFile); diff != "" {
				t.Fatal("bootfile", diff)
			}
			if diff := cmp.Diff(nextServer, tt.wantNextSrv); diff != "" {
				t.Fatal("nextServer", diff)
			}
		})
	}
}

func TestIsNetbootClient(t *testing.T) {
	tests := map[string]struct {
		input *dhcpv4.DHCPv4
		want  error
	}{
		"fail invalid message type": {input: &dhcpv4.DHCPv4{Options: dhcpv4.OptionsFromList(dhcpv4.OptMessageType(dhcpv4.MessageTypeInform))}, want: errors.New("")},
		"fail no opt60":             {input: &dhcpv4.DHCPv4{Options: dhcpv4.OptionsFromList(dhcpv4.OptMessageType(dhcpv4.MessageTypeDiscover))}, want: errors.New("")},
		"fail bad opt60": {input: &dhcpv4.DHCPv4{Options: dhcpv4.OptionsFromList(
			dhcpv4.OptMessageType(dhcpv4.MessageTypeDiscover),
			dhcpv4.OptClassIdentifier("BadClient"),
		)}, want: errors.New("")},
		"fail no opt93": {input: &dhcpv4.DHCPv4{Options: dhcpv4.OptionsFromList(
			dhcpv4.OptMessageType(dhcpv4.MessageTypeDiscover),
			dhcpv4.OptClassIdentifier("HTTPClient:Arch:xxxxx:UNDI:yyyzzz"),
		)}, want: errors.New("")},
		"fail no opt94": {input: &dhcpv4.DHCPv4{Options: dhcpv4.OptionsFromList(
			dhcpv4.OptMessageType(dhcpv4.MessageTypeDiscover),
			dhcpv4.OptClassIdentifier("HTTPClient:Arch:xxxxx:UNDI:yyyzzz"),
			dhcpv4.OptClientArch(iana.EFI_ARM64_HTTP),
		)}, want: errors.New("")},
		"fail invalid opt97[0] != 0": {input: &dhcpv4.DHCPv4{Options: dhcpv4.OptionsFromList(
			dhcpv4.OptMessageType(dhcpv4.MessageTypeDiscover),
			dhcpv4.OptClassIdentifier("HTTPClient:Arch:xxxxx:UNDI:yyyzzz"),
			dhcpv4.OptClientArch(iana.EFI_ARM64_HTTP),
			dhcpv4.OptGeneric(dhcpv4.OptionClientNetworkInterfaceIdentifier, []byte{0x01, 0x02, 0x03, 0x04, 0x05, 0x06}),
			dhcpv4.OptGeneric(dhcpv4.OptionClientMachineIdentifier, []byte{0x01, 0x02, 0x03, 0x04, 0x05, 0x06, 0x00, 0x02, 0x03, 0x04, 0x05, 0x06, 0x00, 0x02, 0x03, 0x04, 0x05}),
		)}, want: errors.New("")},
		"fail invalid len(opt97)": {input: &dhcpv4.DHCPv4{Options: dhcpv4.OptionsFromList(
			dhcpv4.OptMessageType(dhcpv4.MessageTypeDiscover),
			dhcpv4.OptClassIdentifier("HTTPClient:Arch:xxxxx:UNDI:yyyzzz"),
			dhcpv4.OptClientArch(iana.EFI_ARM64_HTTP),
			dhcpv4.OptGeneric(dhcpv4.OptionClientNetworkInterfaceIdentifier, []byte{0x01, 0x02, 0x03, 0x04, 0x05, 0x06}),
			dhcpv4.OptGeneric(dhcpv4.OptionClientMachineIdentifier, []byte{0x01, 0x02}),
		)}, want: errors.New("")},
		"success len(opt97) == 0": {input: &dhcpv4.DHCPv4{Options: dhcpv4.OptionsFromList(
			dhcpv4.OptMessageType(dhcpv4.MessageTypeDiscover),
			dhcpv4.OptClassIdentifier("HTTPClient:Arch:xxxxx:UNDI:yyyzzz"),
			dhcpv4.OptClientArch(iana.EFI_ARM64_HTTP),
			dhcpv4.OptGeneric(dhcpv4.OptionClientNetworkInterfaceIdentifier, []byte{0x01, 0x02, 0x03, 0x04, 0x05, 0x06}),
			dhcpv4.OptGeneric(dhcpv4.OptionClientMachineIdentifier, []byte{}),
		)}, want: nil},
	}
	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			if err := IsNetbootClient(tt.input); err == nil && tt.want != nil {
				t.Errorf("isNetbootClient() = %v, want %v", err, tt.want)
			}
		})
	}
}

func TestTraceparentFromContext(t *testing.T) {
	want := []byte{0, 1, 2, 3, 4, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 5, 6, 7, 8, 0, 0, 0, 0, 1}
	sc := trace.NewSpanContext(trace.SpanContextConfig{
		TraceID:    trace.TraceID{0x01, 0x02, 0x03, 0x04},
		SpanID:     trace.SpanID{0x05, 0x06, 0x07, 0x08},
		TraceFlags: trace.TraceFlags(1),
	})
	rmSpan := trace.ContextWithRemoteSpanContext(context.Background(), sc)

	got := TraceparentFromContext(rmSpan)
	if !bytes.Equal(got, want) {
		t.Errorf("binaryTpFromContext() = %v, want %v", got, want)
	}
}
