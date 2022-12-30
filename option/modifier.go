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
func (c Conf) SetNetworkBootOpts(ctx context.Context, pkt *dhcpv4.DHCPv4, n *data.Netboot) []dhcpv4.Modifier {
	mods := []dhcpv4.Modifier{
		SetOpt60(pkt.ClassIdentifier()),
		c.setOpt43(ctx, pkt.ClientHWAddr, n.VLAN),
		c.setBootfileAndServerIP(ctx, pkt, n),
		dhcpv4.WithUserClass("Tinkerbell", true),
	}

	return mods
}

func (c Conf) setOpt43(ctx context.Context, mac net.HardwareAddr, vlan string) dhcpv4.Modifier {
	return func(d *dhcpv4.DHCPv4) {
		pxe := dhcpv4.Options{ // FYI, these are suboptions of option43. ref: https://datatracker.ietf.org/doc/html/rfc2132#section-8.4
			// PXE Boot Server Discovery Control - bypass, just boot from filename.
			6:  []byte{8},
			69: TraceparentFromContext(ctx),
		}
		if vlan != "" {
			pxe[116] = []byte(vlan) // vlan to use for iPXE
		}
		if isRPI(mac) {
			c.Log.Info("this is a Raspberry Pi", "mac", mac)
			addVendorOpts(pxe)
		}

		d.UpdateOption(dhcpv4.OptGeneric(dhcpv4.OptionVendorSpecificInformation, pxe.ToBytes()))
	}
}

func (c Conf) setBootfileAndServerIP(ctx context.Context, m *dhcpv4.DHCPv4, n *data.Netboot) dhcpv4.Modifier {
	return func(d *dhcpv4.DHCPv4) {
		a := GetArch(m)
		bin, found := ArchToBootFile[a]
		if !found {
			c.Log.Error(fmt.Errorf("unable to find bootfile for arch"), "network boot not allowed", "arch", a, "archInt", int(a), "mac", m.ClientHWAddr)
			return
		}
		uClass := UserClass(string(m.GetOneOption(dhcpv4.OptionUserClassInformation)))
		fmt.Println("=====================================")
		fmt.Println("uClass", uClass)
		fmt.Println("c.UserClass", string(m.GetOneOption(dhcpv4.OptionUserClassInformation)))
		fmt.Println("bytes", m.GetOneOption(dhcpv4.OptionUserClassInformation))
		fmt.Println("=====================================")
		ipxeScript := c.IPXEScriptURL
		if n.IPXEScriptURL != nil {
			ipxeScript = n.IPXEScriptURL
		}
		d.BootFileName, d.ServerIPAddr = BootfileAndNextServer(ctx, uClass, c.UserClass, GetClientType(m.ClassIdentifier()), bin, c.IPXEBinServerTFTP, c.IPXEBinServerHTTP, ipxeScript, c.OTELEnabled)
	}
}
