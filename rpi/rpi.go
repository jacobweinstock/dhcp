// Package rpi handles the DHCPv4 messages for Raspberry Pi's.
package rpi

import (
	"encoding/hex"
	"net"
	"strings"

	"github.com/insomniacslk/dhcp/dhcpv4"
)

// IsRPI returns true if the given MAC address contains a Raspberry Pi assigned prefix.
func IsRPI(hw net.HardwareAddr) bool {
	// The best way at the moment to figure out if a DHCP request is coming from a Raspberry PI is to
	// check the MAC address. We could reach out to some external server to tell us if the MAC address should
	// use these extra Raspberry PI options but that would require a dependency on some external service and all the trade-offs that
	// come with that. See https://udger.com/resources/mac-address-vendor-detail?name=raspberry_pi_foundation.
	// TODO:(jacobweinstock) look into using OPT97 to detect if a request is from a Raspberry Pi.
	// see https://www.raspberrypi.com/documentation/computers/raspberry-pi.html#DHCP_OPTION97.
	switch strings.ToLower(hw.String()) {
	case "28:cd:c1", "b8:27:eb", "dc:a6:32", "e4:5f:01":
		return true
	}

	return false
}

// AddVendorOpts updates a given dhcpv4.Options map with Raspberry pi specific options and returns an encoded DHCP option 43.
func AddVendorOpts(opt43 dhcpv4.Options) {
	// Raspberry PI's need sub options 9 and 10 of parent option 43.
	// TODO: provide doc link for why these options are needed.
	// TODO document what these hex strings are and why they are needed.
	// https://www.raspberrypi.com/documentation/computers/raspberry-pi.html#DHCP_OPTION97
	// https://www.raspberrypi.org/documentation/computers/raspberry-pi.html#PXE_OPTION43
	// https://www.raspberrypi.com/documentation/computers/raspberry-pi.html#dhcp-request-reply
	// https://www.rfc-editor.org/rfc/rfc2132.html#section-8.4
	// tested with Raspberry Pi 4 using UEFI from here: https://github.com/pftf/RPi4/releases/tag/v1.31
	// all files were served via a tftp server and lived at the top level dir of the tftp server (i.e tftp://server/)
	// "\x00\x00\x11" is equal to NUL(Null), NUL(Null), DC1(Device Control 1)
	opt43[9], _ = hex.DecodeString("00001152617370626572727920506920426f6f74") // "\x00\x00\x11Raspberry Pi Boot"
	// "\x0a\x04\x00" is equal to LF(Line Feed), EOT(End of Transmission), NUL(Null)
	opt43[10], _ = hex.DecodeString("00505845") // "\x0a\x04\x00PXE"
}
