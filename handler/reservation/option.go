package reservation

import (
	"context"
	"fmt"
	"net"
	"strings"

	"github.com/insomniacslk/dhcp/dhcpv4"
	"github.com/tinkerbell/dhcp/data"
	"github.com/tinkerbell/dhcp/handler/option"
	"github.com/tinkerbell/dhcp/otel"
	"github.com/tinkerbell/dhcp/rpi"
)

// setNetworkBootOpts purpose is to set 3 or 4 values. 2 DHCP headers, option 43 and optionally option (60).
// These headers and options are returned as a dhcvp4.Modifier that can be used to modify a dhcp response.
// github.com/insomniacslk/dhcp uses this method to simplify packet manipulation.
//
// DHCP Headers (https://datatracker.ietf.org/doc/html/rfc2131#section-2)
// 'siaddr': IP address of next bootstrap server. represented below as `.ServerIPAddr`.
// 'file': Client boot file name. represented below as `.BootFileName`.
//
// DHCP option
// option 60: Class Identifier. https://www.rfc-editor.org/rfc/rfc2132.html#section-9.13
// option 60 is set if the client's option 60 (Class Identifier) starts with HTTPClient.
func (h *Handler) setNetworkBootOpts(ctx context.Context, m *dhcpv4.DHCPv4, n *data.Netboot) dhcpv4.Modifier {
	// m is a received DHCPv4 packet.
	// d is the reply packet we are building.
	withNetboot := func(d *dhcpv4.DHCPv4) {
		var opt60 option.ClientType
		// if the client sends opt 60 with HTTPClient then we need to respond with opt 60
		if val := m.Options.Get(dhcpv4.OptionClassIdentifier); val != nil {
			if strings.HasPrefix(string(val), option.HTTPClient.String()) {
				d.UpdateOption(dhcpv4.OptGeneric(dhcpv4.OptionClassIdentifier, []byte(option.HTTPClient)))
				opt60 = option.HTTPClient
			}
		}
		d.BootFileName = "/netboot-not-allowed"
		d.ServerIPAddr = net.IPv4(0, 0, 0, 0)
		if n.AllowNetboot {
			a := option.GetArch(m)
			bin, found := option.ArchToBootFile[a]
			if !found {
				h.Log.Error(fmt.Errorf("unable to find bootfile for arch"), "network boot not allowed", "arch", a, "archInt", int(a), "mac", m.ClientHWAddr)
				return
			}
			uClass := option.UserClass(string(m.GetOneOption(dhcpv4.OptionUserClassInformation)))
			ipxeScript := h.Netboot.IPXEScriptURL
			if n.IPXEScriptURL != nil {
				ipxeScript = n.IPXEScriptURL
			}
			d.BootFileName, d.ServerIPAddr = option.BootfileAndNextServer(ctx, uClass, h.Netboot.UserClass, opt60, bin, h.Netboot.IPXEBinServerTFTP, h.Netboot.IPXEBinServerHTTP, ipxeScript, h.OTELEnabled)
			pxe := dhcpv4.Options{ // FYI, these are suboptions of option43. ref: https://datatracker.ietf.org/doc/html/rfc2132#section-8.4
				6:   []byte{8}, // PXE Boot Server Discovery Control - bypass, just boot from filename.
				69:  otel.TraceparentFromContext(ctx),
			}
			if n.VLAN != "" {
				pxe[116] = []byte(n.VLAN) // vlan to use for iPXE
			}
			if rpi.IsRPI(m.ClientHWAddr) {
				rpi.AddVendorOpts(pxe)
			}

			d.UpdateOption(dhcpv4.OptGeneric(dhcpv4.OptionVendorSpecificInformation, pxe.ToBytes()))
		}
	}

	return withNetboot
}
