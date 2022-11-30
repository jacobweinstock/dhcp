package option

import (
	"context"
	"fmt"
	"net"
	"net/url"

	"github.com/go-logr/logr"
	"github.com/insomniacslk/dhcp/dhcpv4"
	"github.com/tinkerbell/dhcp/data"
	"inet.af/netaddr"
)

type Conf struct {
	Log               logr.Logger
	IPXEScriptURL     *url.URL
	UserClass         UserClass
	IPXEBinServerTFTP netaddr.IPPort
	IPXEBinServerHTTP *url.URL
	OTELEnabled       bool
}

// SetNetworkBootOpts sets the network boot options for the DHCP reply, based on the PXE spec for a proxyDHCP server
// found here: http://www.pix.net/software/pxeboot/archive/pxespec.pdf
// set the following DHCP options:
// opt43, opt97, opt60, opt54
// set the following DHCP headers:
// siaddr, sname, bootfile.
func (c Conf) SetNetworkBootOpts(ctx context.Context, m *dhcpv4.DHCPv4, n *data.Netboot) dhcpv4.Modifier {
	// m is a received DHCPv4 packet.
	// d is the reply packet we are building.
	withNetboot := func(d *dhcpv4.DHCPv4) {
		// var opt60 string
		// if the client sends opt 60 with HTTPClient then we need to respond with opt 60
		// one, opt60 := setOpt54And60AndSNAME(d.ClassIdentifier(), h.Netboot.IPXEBinServerTFTP.IP().IPAddr().IP, net.ParseIP(h.Netboot.IPXEBinServerHTTP.Host))
		// one(d)
		o60 := SetOpt60(m.ClassIdentifier())
		o60(d)
		// sname := SetHeaderSNAME(d.ClassIdentifier(), h.Netboot.IPXEBinServerTFTP.IP().IPAddr().IP, net.ParseIP(h.Netboot.IPXEBinServerHTTP.Host))
		// sname(d)
		d.BootFileName = "/netboot-not-allowed"
		d.ServerIPAddr = net.IPv4(0, 0, 0, 0)
		a := GetArch(m)
		bin, found := ArchToBootFile[a]
		if !found {
			c.Log.Error(fmt.Errorf("unable to find bootfile for arch"), "network boot not allowed", "arch", a, "archInt", int(a), "mac", m.ClientHWAddr)
			return
		}
		uClass := UserClass(string(m.GetOneOption(dhcpv4.OptionUserClassInformation)))
		ipxeScript := c.IPXEScriptURL
		if n.IPXEScriptURL != nil {
			ipxeScript = n.IPXEScriptURL
		}
		d.BootFileName, d.ServerIPAddr = BootfileAndNextServer(ctx, uClass, c.UserClass, GetClientType(m.ClassIdentifier()), bin, c.IPXEBinServerTFTP, c.IPXEBinServerHTTP, ipxeScript, c.OTELEnabled)
		pxe := dhcpv4.Options{ // FYI, these are suboptions of option43. ref: https://datatracker.ietf.org/doc/html/rfc2132#section-8.4
			// PXE Boot Server Discovery Control - bypass, just boot from filename.
			6:  []byte{8},
			69: TraceparentFromContext(ctx),
		}
		if n.VLAN != "" {
			pxe[116] = []byte(n.VLAN) // vlan to use for iPXE
		}
		if isRPI(m.ClientHWAddr) {
			c.Log.Info("this is a Raspberry Pi", "mac", m.ClientHWAddr)
			addVendorOpts(pxe)
		}

		d.UpdateOption(dhcpv4.OptGeneric(dhcpv4.OptionVendorSpecificInformation, pxe.ToBytes()))
	}

	return withNetboot
}
