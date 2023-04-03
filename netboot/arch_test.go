package netboot

import (
	"net"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/insomniacslk/dhcp/dhcpv4"
	"github.com/insomniacslk/dhcp/iana"
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
		"found RPI": {
			pkt:  &dhcpv4.DHCPv4{ClientHWAddr: net.HardwareAddr{0x28, 0xCD, 0xC1, 0x04, 0x05, 0x06}, Options: dhcpv4.OptionsFromList(dhcpv4.OptClientArch(iana.Arch(41)))},
			want: iana.Arch(41),
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
			got := GetArch(tt.pkt)
			if diff := cmp.Diff(tt.want, got); diff != "" {
				t.Fatal(diff)
			}
		})
	}
}
