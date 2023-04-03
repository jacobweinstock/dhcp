// Package netboot provides functions for setting DHCP options for network booting.
package netboot

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/netip"
	"net/url"
	"strings"

	"github.com/equinix-labs/otel-init-go/otelhelpers"
	"github.com/go-logr/logr"
	"github.com/insomniacslk/dhcp/dhcpv4"
	"github.com/tinkerbell/dhcp/data"
	"go.opentelemetry.io/otel/trace"
)

// Conf for setting DHCP options. Might need to rethink this. Doesn't feel right.
type Conf struct {
	Log               logr.Logger
	IPXEScriptURL     *url.URL
	UserClass         UserClass
	IPXEBinServerTFTP netip.AddrPort
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
		// setOpt97(pkt.GetOneOption(dhcpv4.OptionClientMachineIdentifier)),
		c.setOpt43(ctx, pkt.ClientHWAddr, n.VLAN),
		c.setBootfileAndServerIP(ctx, pkt, n),
		// dhcpv4.WithUserClass("Tinkerbell", true),
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
		ipxeScript := c.IPXEScriptURL
		if n.IPXEScriptURL != nil {
			ipxeScript = n.IPXEScriptURL
		}
		d.BootFileName, d.ServerIPAddr = BootfileAndNextServer(ctx, uClass, c.UserClass, GetClientType(m.ClassIdentifier()), bin, c.IPXEBinServerTFTP, c.IPXEBinServerHTTP, ipxeScript, c.OTELEnabled)
	}
}

/*func setOpt97(guid []byte) dhcpv4.Modifier {
	return func(d *dhcpv4.DHCPv4) {
		d.UpdateOption(dhcpv4.OptGeneric(dhcpv4.OptionClientMachineIdentifier, guid))
	}
}*/

const (
	// IPXE known user-class types. must correspond to DHCP option 77 - User-Class
	// https://www.rfc-editor.org/rfc/rfc3004.html
	// If the client has had iPXE burned into its ROM (or is a VM
	// that uses iPXE as the PXE "ROM"), special handling is
	// needed because in this mode the client is using iPXE native
	// drivers and chainloading to a UNDI stack won't work.
	IPXE UserClass = "iPXE"
	// Tinkerbell If the client identifies as "Tinkerbell", we've already
	// chainloaded this client to the full-featured copy of iPXE
	// we supply. We have to distinguish this case so we don't
	// loop on the chainload step.
	Tinkerbell UserClass = "Tinkerbell"
	// PXEClient for pxe enabled netboot clients.
	PXEClient ClientType = "PXEClient"
	// HTTPClient for http enabled netboot clients.
	HTTPClient ClientType = "HTTPClient"
)

// UserClass is DHCP option 77 (https://www.rfc-editor.org/rfc/rfc3004.html).
type UserClass string

// ClientType is from DHCP option 60. Normally only PXEClient or HTTPClient.
type ClientType string

// String function for clientType.
func (c ClientType) String() string {
	return string(c)
}

// String function for UserClass.
func (u UserClass) String() string {
	return string(u)
}

// BootfileAndNextServer returns the bootfile (string) and next server (net.IP).
// input arguments `tftp`, `ipxe` and `iscript` use non string types so as to attempt to be more clear about the expectation around what is wanted for these values.
// It also helps us avoid having to validate a string in multiple ways.
func BootfileAndNextServer(ctx context.Context, pktUserClass UserClass, customUserClass UserClass, opt60 ClientType, bin string, tftp netip.AddrPort, ipxe, iscript *url.URL, otelEnabled bool) (string, net.IP) {
	var bootfile string
	nextServer := net.IP(tftp.Addr().AsSlice())
	if tp := otelhelpers.TraceparentStringFromContext(ctx); otelEnabled && tp != "" {
		bin = fmt.Sprintf("%s-%v", bin, tp)
	}

	// If a machine is in an iPXE boot loop, it is likely to be that we aren't matching on iPXE or Tinkerbell user class (option 77).
	switch { // order matters here.
	case pktUserClass == Tinkerbell, (customUserClass != "" && pktUserClass == customUserClass): // this case gets us out of an ipxe boot loop.
		bootfile = "/no-ipxe-script-defined"
		if iscript != nil {
			bootfile = iscript.String()
		}
		// For proxyDHCP, on the same server as the DHCP server, nextServer needs to be non-nil and not 0.0.0.0. a non-nil iscript value. Any non nil nor 0.0.0.0 IP will do.
		if len(nextServer) == 0 || nextServer.IsUnspecified() {
			ihost := strings.Split(iscript.Host, ":")[0]
			nextServer = net.ParseIP(ihost)
			if nextServer == nil {
				nextServer = net.ParseIP("127.0.0.1")
			}
		}
	case opt60 == HTTPClient: // Check the client type from option 60.
		bootfile = fmt.Sprintf("%s/%s", ipxe, bin)
		ihost := strings.Split(ipxe.Host, ":")[0]
		ns := net.ParseIP(ihost)
		if ns == nil {
			// h.Log.Error(fmt.Errorf("unable to parse ipxe host"), "ipxe", ipxe.Host)
			ns = net.ParseIP("0.0.0.0")
		}
		nextServer = ns
	case pktUserClass == IPXE: // if the "iPXE" user class is found it means we aren't in our custom version of ipxe, but because of the option 43.6 we're setting we need to give a full tftp url from which to boot.
		bootfile = fmt.Sprintf("tftp://%v/%v", tftp.String(), bin)
	default:
		bootfile = bin
	}

	return bootfile, nextServer
}

// IsNetbootClient returns true if the client is a valid netboot client.
// A valid netboot client will have the following in its DHCP request:
// http://www.pix.net/software/pxeboot/archive/pxespec.pdf
//
// 1. is a DHCP discovery or request message type.
// 2. option 93 is set.
// 3. option 94 is set.
// 4. option 97 is correct length.
// 5. option 60 is set with this format: "PXEClient:Arch:xxxxx:UNDI:yyyzzz" or "HTTPClient:Arch:xxxxx:UNDI:yyyzzz".
func IsNetbootClient(pkt *dhcpv4.DHCPv4) error {
	// only response to DISCOVER and REQUEST packets
	if pkt.MessageType() != dhcpv4.MessageTypeDiscover && pkt.MessageType() != dhcpv4.MessageTypeRequest {
		return fmt.Errorf("message type (%q) must be either Discover or Request", pkt.MessageType())
	}
	// option 60 must be set
	if !pkt.Options.Has(dhcpv4.OptionClassIdentifier) {
		return errors.New("option 60 not set")
	}
	// option 60 must start with PXEClient or HTTPClient
	opt60 := pkt.ClassIdentifier()
	if !strings.HasPrefix(opt60, string(PXEClient)) && !strings.HasPrefix(opt60, string(HTTPClient)) {
		return fmt.Errorf("option 60 (%q) must start with PXEClient or HTTPClient", opt60)
	}

	// option 93 must be set
	if !pkt.Options.Has(dhcpv4.OptionClientSystemArchitectureType) {
		return errors.New("option 93 not set")
	}

	// option 94 must be set
	if !pkt.Options.Has(dhcpv4.OptionClientNetworkInterfaceIdentifier) {
		return errors.New("option 94 not set")
	}

	// option 97 must be have correct length or not be set
	guid := pkt.GetOneOption(dhcpv4.OptionClientMachineIdentifier)
	switch len(guid) {
	case 0:
		// A missing GUID is invalid according to the spec, however
		// there are PXE ROMs in the wild that omit the GUID and still
		// expect to boot. The only thing we do with the GUID is
		// mirror it back to the client if it's there, so we might as
		// well accept these buggy ROMs.
	case 17:
		if guid[0] != 0 {
			return fmt.Errorf("option 97 (%q) does not start with 0", string(guid))
		}
	default:
		// h.Log.Info("not a netboot client", "reason", "option 97 has invalid length (0 or 17)", "mac", pkt.ClientHWAddr.String(), "option 97", string(guid))
		return fmt.Errorf("option 97 has invalid length (must be 0 or 17): %v", len(guid))
	}

	return nil
}

// SetOpt60AndSNAME based on option 60.
func SetOpt60AndSNAME(opt60FromClient string, tftp net.IP, http net.IP) (dhcpv4.Modifier, ClientType) {
	opt54 := tftp
	opt60 := PXEClient
	if strings.HasPrefix(opt60FromClient, string(HTTPClient)) {
		opt54 = http
		opt60 = HTTPClient
	}

	return func(d *dhcpv4.DHCPv4) {
		d.UpdateOption(dhcpv4.OptClassIdentifier(opt60.String()))
		d.ServerHostName = opt54.String()
	}, opt60
}

// SetOpt60 mirrors back option 60 from a client request.
func SetOpt60(opt60FromClient string) dhcpv4.Modifier {
	opt60 := PXEClient
	if strings.HasPrefix(opt60FromClient, string(HTTPClient)) {
		opt60 = HTTPClient
	}

	return func(d *dhcpv4.DHCPv4) {
		d.UpdateOption(dhcpv4.OptClassIdentifier(opt60.String()))
	}
}

// GetClientType returns the client type based on option 60.
func GetClientType(opt60 string) ClientType {
	if strings.HasPrefix(opt60, string(HTTPClient)) {
		return HTTPClient
	}
	return PXEClient
}

// TraceparentFromContext extracts the binary trace id, span id, and trace flags
// from the running span in ctx and returns a 26 byte []byte with the traceparent
// encoded and ready to pass into a suboption (most likely 69) of opt43.
func TraceparentFromContext(ctx context.Context) []byte {
	sc := trace.SpanContextFromContext(ctx)
	tpBytes := make([]byte, 0, 26)

	// the otel spec says 16 bytes for trace id and 8 for spans are good enough
	// for everyone copy them into a []byte that we can deliver over option43
	tid := [16]byte(sc.TraceID()) // type TraceID [16]byte
	sid := [8]byte(sc.SpanID())   // type SpanID [8]byte

	tpBytes = append(tpBytes, 0x00)      // traceparent version
	tpBytes = append(tpBytes, tid[:]...) // trace id
	tpBytes = append(tpBytes, sid[:]...) // span id
	if sc.IsSampled() {
		tpBytes = append(tpBytes, 0x01) // trace flags
	} else {
		tpBytes = append(tpBytes, 0x00)
	}

	return tpBytes
}

/*
// SetNetworkBootOpts sets the network boot options for the DHCP reply, based on the PXE spec for a proxyDHCP server
// found here: http://www.pix.net/software/pxeboot/archive/pxespec.pdf
// set the following DHCP options:
// opt43, opt97, opt60, opt54
// set the following DHCP headers:
// siaddr, sname, bootfile.
func SetNetworkBootOpts(ctx context.Context, m *dhcpv4.DHCPv4, n *data.Netboot) dhcpv4.Modifier {
	// return dhcpv4.Modifier(func(pkt *dhcpv4.DHCPv4) {})
	// m is a received DHCPv4 packet.
	// d is the reply packet we are building.
	withNetboot := func(d *dhcpv4.DHCPv4) {
		// var opt60 string
		// if the client sends opt 60 with HTTPClient then we need to respond with opt 60
		// one, opt60 := setOpt54And60AndSNAME(d.ClassIdentifier(), h.Netboot.IPXEBinServerTFTP.IP().AsSlice(), net.ParseIP(h.Netboot.IPXEBinServerHTTP.Host))
		// one(d)
		o60 := SetOpt60(d.ClassIdentifier())
		o60(d)
		sname := SetHeaderSNAME(d.ClassIdentifier(), h.Netboot.IPXEBinServerTFTP.IP().AsSlice(), net.ParseIP(h.Netboot.IPXEBinServerHTTP.Host))
		sname(d)
		d.BootFileName = "/netboot-not-allowed"
		d.ServerIPAddr = net.IPv4(0, 0, 0, 0)
		if n.AllowNetboot {
			a := GetArch(m)
			bin, found := ArchToBootFile[a]
			if !found {
				// h.Log.Error(fmt.Errorf("unable to find bootfile for arch"), "network boot not allowed", "arch", a, "archInt", int(a), "mac", m.ClientHWAddr)
				return
			}
			uClass := UserClass(string(m.GetOneOption(dhcpv4.OptionUserClassInformation)))
			ipxeScript := h.Netboot.IPXEScriptURL
			if n.IPXEScriptURL != nil {
				ipxeScript = n.IPXEScriptURL
			}
			d.BootFileName, d.ServerIPAddr = BootfileAndNextServer(ctx, uClass, h.Netboot.UserClass, GetClientType(d.ClassIdentifier()), bin, h.Netboot.IPXEBinServerTFTP, h.Netboot.IPXEBinServerHTTP, ipxeScript, h.OTELEnabled)
			pxe := dhcpv4.Options{ // FYI, these are suboptions of option43. ref: https://datatracker.ietf.org/doc/html/rfc2132#section-8.4
				// PXE Boot Server Discovery Control - bypass, just boot from filename.
				6:  []byte{8},
				69: oteldhcp.TraceparentFromContext(ctx),
			}
			if rpi.IsRPI(m.ClientHWAddr) {
				// h.Log.Info("this is a Raspberry Pi", "mac", m.ClientHWAddr)
				rpi.AddVendorOpts(pxe)
			}

			d.UpdateOption(dhcpv4.OptGeneric(dhcpv4.OptionVendorSpecificInformation, pxe.ToBytes()))
		}
	}

	return withNetboot
}
*/
