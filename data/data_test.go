package data

import (
	"net"
	"net/url"
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
	"github.com/insomniacslk/dhcp/dhcpv4"
	"github.com/insomniacslk/dhcp/iana"
	"github.com/insomniacslk/dhcp/rfc1035label"
	"go.opentelemetry.io/otel/attribute"
	"inet.af/netaddr"
)

func TestToDHCPMods(t *testing.T) {
	type args struct {
		m *dhcpv4.DHCPv4
		d *DHCP
	}
	tests := map[string]struct {
		args args
		want *dhcpv4.DHCPv4
	}{
		"success": {
			args: args{
				m: &dhcpv4.DHCPv4{Options: dhcpv4.OptionsFromList(dhcpv4.OptParameterRequestList(dhcpv4.OptionSubnetMask))},
				d: &DHCP{
					MACAddress:     net.HardwareAddr{0x00, 0x00, 0x00, 0x00, 0x00, 0x00},
					IPAddress:      netaddr.IPv4(192, 168, 4, 4),
					SubnetMask:     []byte{255, 255, 255, 0},
					DefaultGateway: netaddr.IPv4(192, 168, 4, 1),
					NameServers: []net.IP{
						{8, 8, 8, 8},
						{8, 8, 4, 4},
					},
					Hostname:         "test-server",
					DomainName:       "mynet.local",
					BroadcastAddress: netaddr.IPv4(192, 168, 4, 255),
					NTPServers: []net.IP{
						{132, 163, 96, 2},
						{132, 163, 96, 3},
					},
					LeaseTime: 84600,
					DomainSearch: []string{
						"mynet.local",
					},
				},
			},
			want: &dhcpv4.DHCPv4{
				OpCode:        dhcpv4.OpcodeBootRequest,
				HWType:        iana.HWTypeEthernet,
				ClientHWAddr:  net.HardwareAddr{0x00, 0x00, 0x00, 0x00, 0x00, 0x00},
				ClientIPAddr:  []byte{0, 0, 0, 0},
				YourIPAddr:    []byte{192, 168, 4, 4},
				ServerIPAddr:  []byte{0, 0, 0, 0},
				GatewayIPAddr: []byte{0, 0, 0, 0},
				Options: dhcpv4.OptionsFromList(
					dhcpv4.OptSubnetMask(net.IPMask{255, 255, 255, 0}),
					dhcpv4.OptBroadcastAddress(net.IP{192, 168, 4, 255}),
					dhcpv4.OptIPAddressLeaseTime(time.Duration(84600)*time.Second),
					dhcpv4.OptDomainName("mynet.local"),
					dhcpv4.OptHostName("test-server"),
					dhcpv4.OptRouter(net.IP{192, 168, 4, 1}),
					dhcpv4.OptDNS([]net.IP{
						{8, 8, 8, 8},
						{8, 8, 4, 4},
					}...),
					dhcpv4.OptNTPServers([]net.IP{
						{132, 163, 96, 2},
						{132, 163, 96, 3},
					}...),
					dhcpv4.OptDomainSearch(&rfc1035label.Labels{
						Labels: []string{"mynet.local"},
					}),
				),
			},
		},
	}
	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			mods := tt.args.d.ToDHCPMods()
			finalPkt, err := dhcpv4.New(mods...)
			if err != nil {
				t.Fatalf("setDHCPOpts() error = %v, wantErr nil", err)
			}
			if diff := cmp.Diff(tt.want, finalPkt, cmpopts.IgnoreFields(dhcpv4.DHCPv4{}, "TransactionID")); diff != "" {
				t.Fatal(diff)
			}
		})
	}
}

func TestDHCPEncodeToAttributes(t *testing.T) {
	tests := map[string]struct {
		dhcp *DHCP
		want []attribute.KeyValue
	}{
		"successful encode of zero value DHCP struct": {
			dhcp: &DHCP{},
			want: []attribute.KeyValue{
				attribute.String("DHCP.MACAddress", ""),
				attribute.String("DHCP.IPAddress", ""),
				attribute.String("DHCP.Hostname", ""),
				attribute.String("DHCP.SubnetMask", ""),
				attribute.String("DHCP.DefaultGateway", ""),
				attribute.String("DHCP.NameServers", ""),
				attribute.String("DHCP.DomainName", ""),
				attribute.String("DHCP.BroadcastAddress", ""),
				attribute.String("DHCP.NTPServers", ""),
				attribute.Int64("DHCP.LeaseTime", 0),
				attribute.String("DHCP.DomainSearch", ""),
			},
		},
		"successful encode of populated DHCP struct": {
			dhcp: &DHCP{
				MACAddress:       []byte{0x00, 0x01, 0x02, 0x03, 0x04, 0x05},
				IPAddress:        netaddr.IPv4(192, 168, 2, 150),
				SubnetMask:       []byte{255, 255, 255, 0},
				DefaultGateway:   netaddr.IPv4(192, 168, 2, 1),
				NameServers:      []net.IP{{1, 1, 1, 1}, {8, 8, 8, 8}},
				Hostname:         "test",
				DomainName:       "example.com",
				BroadcastAddress: netaddr.IPv4(192, 168, 2, 255),
				NTPServers:       []net.IP{{132, 163, 96, 2}},
				LeaseTime:        86400,
				DomainSearch:     []string{"example.com", "example.org"},
			},
			want: []attribute.KeyValue{
				attribute.String("DHCP.MACAddress", "00:01:02:03:04:05"),
				attribute.String("DHCP.IPAddress", "192.168.2.150"),
				attribute.String("DHCP.Hostname", "test"),
				attribute.String("DHCP.SubnetMask", "255.255.255.0"),
				attribute.String("DHCP.DefaultGateway", "192.168.2.1"),
				attribute.String("DHCP.NameServers", "1.1.1.1,8.8.8.8"),
				attribute.String("DHCP.DomainName", "example.com"),
				attribute.String("DHCP.BroadcastAddress", "192.168.2.255"),
				attribute.String("DHCP.NTPServers", "132.163.96.2"),
				attribute.Int64("DHCP.LeaseTime", 86400),
				attribute.String("DHCP.DomainSearch", "example.com,example.org"),
			},
		},
	}
	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			want := attribute.NewSet(tt.want...)
			got := attribute.NewSet(tt.dhcp.EncodeToAttributes()...)
			enc := attribute.DefaultEncoder()
			if diff := cmp.Diff(got.Encoded(enc), want.Encoded(enc)); diff != "" {
				t.Fatal(diff)
			}
		})
	}
}

func TestNetbootEncodeToAttributes(t *testing.T) {
	tests := map[string]struct {
		netboot *Netboot
		want    []attribute.KeyValue
	}{
		"successful encode of zero value Netboot struct": {
			netboot: &Netboot{},
			want: []attribute.KeyValue{
				attribute.Bool("Netboot.AllowNetboot", false),
				attribute.String("Netboot.IPXEScriptURL", ""),
			},
		},
		"successful encode of populated Netboot struct": {
			netboot: &Netboot{
				AllowNetboot:  true,
				IPXEScriptURL: &url.URL{Scheme: "http", Host: "example.com"},
			},
			want: []attribute.KeyValue{
				attribute.Bool("Netboot.AllowNetboot", true),
				attribute.String("Netboot.IPXEScriptURL", "http://example.com"),
			},
		},
	}
	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			want := attribute.NewSet(tt.want...)
			got := attribute.NewSet(tt.netboot.EncodeToAttributes()...)
			enc := attribute.DefaultEncoder()
			if diff := cmp.Diff(got.Encoded(enc), want.Encoded(enc)); diff != "" {
				t.Fatal(diff)
			}
		})
	}
}
