package data

import (
	"errors"
	"net"
	"net/url"
	"testing"

	"github.com/google/go-cmp/cmp"
	"go.opentelemetry.io/otel/attribute"
	"inet.af/netaddr"
)

func TestDHCPEncodeToAttributes(t *testing.T) {
	tests := map[string]struct {
		dhcp *DHCP
		want []attribute.KeyValue
	}{
		"successful encode of zero value DHCP struct": {
			dhcp: &DHCP{},
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

func TestMarshalJSON(t *testing.T) {
	tests := map[string]struct {
		dhcp *DHCP
		want string
		err  error
	}{
		"successful marshal of zero value DHCP struct": {
			dhcp: &DHCP{},
			want: `{"MACAddress":"","IPAddress":"","SubnetMask":"","DefaultGateway":"","NameServers":null,"Hostname":"","DomainName":"","BroadcastAddress":"","NTPServers":null,"LeaseTime":0,"DomainSearch":null}`,
		},
		"successful": {
			dhcp: &DHCP{NameServers: []net.IP{{1, 1, 1, 1}, {8, 8, 8, 8}}, NTPServers: []net.IP{{132, 163, 96, 2}}},
			want: `{"MACAddress":"","IPAddress":"","SubnetMask":"","DefaultGateway":"","NameServers":["1.1.1.1","8.8.8.8"],"Hostname":"","DomainName":"","BroadcastAddress":"","NTPServers":["132.163.96.2"],"LeaseTime":0,"DomainSearch":null}`,
		},
	}
	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			got, err := tt.dhcp.MarshalJSON()
			t.Log(string(got))
			t.Log(tt.want)
			if !errors.Is(err, tt.err) {
				t.Fatal(err)
			}
			if diff := cmp.Diff(string(got), tt.want); diff != "" {
				t.Fatal(diff)
			}
		})
	}
}

func TestUnmarshalJSON(t *testing.T) {
	tests := map[string]struct {
		json string
		dhcp DHCP
		err  error
	}{
		"successful unmarshal of zero value DHCP struct": {
			json: `{"MACAddress":"","IPAddress":"","SubnetMask":"","DefaultGateway":"","NameServers":null,"Hostname":"","DomainName":"","BroadcastAddress":"","NTPServers":null,"LeaseTime":0,"DomainSearch":null}`,
			dhcp: DHCP{},
		},
		"successful": {
			json: `{"MACAddress":"00:01:02:03:04:05","IPAddress":"192.168.2.150","SubnetMask":"255.255.255.0","DefaultGateway":"192.168.2.1","NameServers":["1.1.1.1","8.8.8.8"],"Hostname":"test","DomainName":"example.com","BroadcastAddress":"192.168.2.255","NTPServers":["132.163.96.2"],"LeaseTime":86400,"DomainSearch":["example.com","example.org"]}`,
			dhcp: DHCP{
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
		},
		"failure raw unmarshal": {
			json: "",
			err:  errUnmarshal,
		},
		"failure type assert error mac address": {
			json: `{"MACAddress":1}`,
			err:  errUnmarshal,
		},
		"failure bad mac address": {
			json: `{"MACAddress":"bad"}`,
			err:  errUnmarshal,
		},
		"failure type assert error ip address": {
			json: `{"MACAddress":"00:01:02:03:04:05","IPAddress":1}`,
			err:  errUnmarshal,
		},
		"failure bad ip address": {
			json: `{"MACAddress":"00:01:02:03:04:05","IPAddress":"bad"}`,
			err:  errUnmarshal,
		},
		"failure type assert error subnetmask": {
			json: `{"MACAddress":"00:01:02:03:04:05","IPAddress":"192.168.2.100","SubnetMask":1}`,
			err:  errUnmarshal,
		},
		"failure bad subnetmask": {
			json: `{"MACAddress":"00:01:02:03:04:05","IPAddress":"192.168.2.100","SubnetMask":"bad"}`,
			err:  errUnmarshal,
		},
		"failure type assert error defaultgateway": {
			json: `{"MACAddress":"00:01:02:03:04:05","IPAddress":"192.168.2.100","SubnetMask":"255.255.255.0","DefaultGateway":1}`,
			err:  errUnmarshal,
		},
		"failure bad defaultgateway": {
			json: `{"MACAddress":"00:01:02:03:04:05","IPAddress":"192.168.2.100","SubnetMask":"255.255.255.0","DefaultGateway":"bad"}`,
			err:  errUnmarshal,
		},
		"failure type assert error nameservers": {
			json: `{"MACAddress":"00:01:02:03:04:05","IPAddress":"192.168.2.100","SubnetMask":"255.255.255.0","DefaultGateway":"192.168.2.1","NameServers":[1]}`,
			err:  errUnmarshal,
		},
		"failure type assert error hostname": {
			json: `{"MACAddress":"00:01:02:03:04:05","IPAddress":"192.168.2.100","SubnetMask":"255.255.255.0","DefaultGateway":"192.168.2.1","NameServers":["1.1.1.1"],"Hostname":1}`,
			err:  errUnmarshal,
		},
		"failure type assert error domainname": {
			json: `{"MACAddress":"00:01:02:03:04:05","IPAddress":"192.168.2.100","SubnetMask":"255.255.255.0","DefaultGateway":"192.168.2.1","NameServers":["1.1.1.1"],"Hostname":"test","DomainName":1}`,
			err:  errUnmarshal,
		},
		"failure type assert error broadcastaddress": {
			json: `{"MACAddress":"00:01:02:03:04:05","IPAddress":"192.168.2.100","SubnetMask":"255.255.255.0","DefaultGateway":"192.168.2.1","NameServers":["1.1.1.1"],"Hostname":"test","DomainName":"example.com","BroadcastAddress":1}`,
			err:  errUnmarshal,
		},
		"failure failed to parse broadcastaddress": {
			json: `{"MACAddress":"00:01:02:03:04:05","IPAddress":"192.168.2.100","SubnetMask":"255.255.255.0","DefaultGateway":"192.168.2.1","NameServers":["1.1.1.1"],"Hostname":"test","DomainName":"example.com","BroadcastAddress":"bad"}`,
			err:  errUnmarshal,
		},
		"failure type assert error ntpservers": {
			json: `{"MACAddress":"00:01:02:03:04:05","IPAddress":"192.168.2.100","SubnetMask":"255.255.255.0","DefaultGateway":"192.168.2.1","NameServers":["1.1.1.1"],"Hostname":"test","DomainName":"example.com","BroadcastAddress":"192.168.2.255","NTPServers":[1]}`,
			err:  errUnmarshal,
		},
		"failure type assert error leasetime": {
			json: `{"MACAddress":"00:01:02:03:04:05","IPAddress":"192.168.2.100","SubnetMask":"255.255.255.0","DefaultGateway":"192.168.2.1","NameServers":["1.1.1.1"],"Hostname":"test","DomainName":"example.com","BroadcastAddress":"192.168.2.255","NTPServers":["1.1.1.1"], "LeaseTime":"1"}`,
			err:  errUnmarshal,
		},
		"failure type assert error domainsearch": {
			json: `{"MACAddress":"00:01:02:03:04:05","IPAddress":"192.168.2.100","SubnetMask":"255.255.255.0","DefaultGateway":"192.168.2.1","NameServers":["1.1.1.1"],"Hostname":"test","DomainName":"example.com","BroadcastAddress":"192.168.2.255","NTPServers":["1.1.1.1"], "LeaseTime":60, "DomainSearch":[1]}`,
			err:  errUnmarshal,
		},
		"failure unknown key": {
			json: `{"badKey":"00:01:02:03:04:05"}`,
			err:  errUnmarshal,
		},
	}
	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			dhcp := new(DHCP)
			err := dhcp.UnmarshalJSON([]byte(tt.json))
			if !errors.Is(err, tt.err) {
				t.Fatal(err)
			}
			if tt.err == nil {
				if diff := cmp.Diff(dhcp, &tt.dhcp, netaddrComparer); diff != "" {
					t.Fatal(diff)
				}
			}
		})
	}
}

var netaddrComparer = cmp.Comparer(func(x, y netaddr.IP) bool {
	return x.Compare(y) == 0
})
