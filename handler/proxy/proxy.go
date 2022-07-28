// Package proxy implements proxyDHCP interactions.
package proxy

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/url"
	"strings"

	"github.com/equinix-labs/otel-init-go/otelhelpers"
	"github.com/go-logr/logr"
	"github.com/insomniacslk/dhcp/dhcpv4"
	"github.com/insomniacslk/dhcp/iana"
	"github.com/tinkerbell/dhcp/backend/noop"
	"github.com/tinkerbell/dhcp/data"
	oteldhcp "github.com/tinkerbell/dhcp/otel"
	"github.com/tinkerbell/dhcp/rpi"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"
	"inet.af/netaddr"
)

const tracerName = "github.com/tinkerbell/dhcp/proxy"

// Handler holds the configuration details for the running the DHCP server.
type Handler struct {
	// Backend is the backend to use for getting DHCP data.
	Backend BackendReader

	// IPAddr is the IP address to use in DHCP responses.
	// Option 54 and the sname DHCP header.
	// This could be a load balancer IP address or an ingress IP address or a local IP address.
	IPAddr netaddr.IP

	// Log is used to log messages.
	// `logr.Discard()` can be used if no logging is desired.
	Log logr.Logger

	// Netboot configuration
	Netboot Netboot

	// OTELEnabled is used to determine if netboot options include otel naming.
	// When true, the netboot filename will be appended with otel information.
	// For example, the filename will be "snp.efi-00-23b1e307bb35484f535a1f772c06910e-d887dc3912240434-01".
	// <original filename>-00-<trace id>-<span id>-<trace flags>
	OTELEnabled bool
}

// Netboot holds the netboot configuration details used in running a DHCP server.
type Netboot struct {
	// iPXE binary server IP:Port serving via TFTP.
	IPXEBinServerTFTP netaddr.IPPort

	// IPXEBinServerHTTP is the URL to the IPXE binary server serving via HTTP(s).
	IPXEBinServerHTTP *url.URL

	// IPXEScriptURL is the URL to the IPXE script to use.
	IPXEScriptURL *url.URL

	// Enabled is whether to enable sending netboot DHCP options.
	Enabled bool

	// UserClass (for network booting) allows a custom DHCP option 77 to be used to break out of an iPXE loop.
	UserClass UserClass
}

// machine describes a device that is requesting a network boot.
/*
type machine struct {
	mac    net.HardwareAddr
	arch   iana.Arch
	uClass UserClass
	cType  clientType
}
*/

// BackendReader is the interface that wraps the Read method.
//
// Backends implement this interface to provide DHCP data to the DHCP server.
type BackendReader interface {
	// Read data (from a backend) based on a mac address
	// and return DHCP headers and options, including netboot info.
	Read(context.Context, net.HardwareAddr) (*data.DHCP, *data.Netboot, error)
}

// setDefaults will update the Handler struct to have default values so as
// to avoid panic for nil pointers and such.
func (h *Handler) setDefaults() {
	if h.Backend == nil {
		h.Backend = noop.Handler{}
	}
	if h.Log.GetSink() == nil {
		h.Log = logr.Discard()
	}
}

func (h *Handler) handleMsg(ctx context.Context, mac net.HardwareAddr, input *dhcpv4.DHCPv4, mt dhcpv4.MessageType) (*dhcpv4.DHCPv4, error) {
	if !h.Netboot.Enabled {
		return nil, errors.New("serving netboot clients is not enabled")
	}
	n, err := h.readBackend(ctx, mac)
	if err != nil {
		h.Log.Error(err, "error reading from backend", "mac", mac.String())

		return nil, err
	}

	if n.AllowNetboot {
		if err := h.isNetbootClient(input); err != nil {
			h.Log.Error(err, "not a netboot client", "mac", mac.String())

			return nil, err
		}
		return h.updateMsg(ctx, input, n, mt), nil
	}

	msg := "client is not allowed to netboot"
	h.Log.V(1).Info(msg, "allowNetboot", n.AllowNetboot, "mac", mac.String())

	return nil, errors.New(msg)
}

// Handle responds to DHCP messages with DHCP server options.
func (h *Handler) Handle(conn net.PacketConn, peer net.Addr, pkt *dhcpv4.DHCPv4) {
	h.setDefaults()
	if pkt == nil {
		h.Log.Error(errors.New("incoming packet is nil"), "not able to respond when the incoming packet is nil")
		return
	}

	log := h.Log.WithValues("mac", pkt.ClientHWAddr.String())
	log.Info("received DHCP packet", "type", pkt.MessageType().String())
	tracer := otel.Tracer(tracerName)
	ctx, span := tracer.Start(context.Background(),
		fmt.Sprintf("DHCP Packet Received: %v", pkt.MessageType().String()),
		trace.WithAttributes(h.encodeToAttributes(pkt, "request")...),
	)
	defer span.End()

	var reply *dhcpv4.DHCPv4
	switch mt := pkt.MessageType(); mt {
	case dhcpv4.MessageTypeDiscover:
		var err error
		reply, err = h.handleMsg(ctx, pkt.ClientHWAddr, pkt, dhcpv4.MessageTypeOffer)
		if err != nil {
			return
		}
	case dhcpv4.MessageTypeRequest:
		var err error
		reply, err = h.handleMsg(ctx, pkt.ClientHWAddr, pkt, dhcpv4.MessageTypeAck)
		if err != nil {
			span.SetAttributes(attribute.String("error", err.Error()))
			span.SetStatus(codes.Error, err.Error())

			return
		}
	case dhcpv4.MessageTypeRelease:
		// Since the design of this DHCP server is that all IP addresses are
		// Host reservations, when a client releases an address, the server
		// doesn't have anything to do. This case is included for clarity of this
		// design decision.
		log.Info("received release message, no response required")
		span.SetStatus(codes.Ok, "received release message, no response required")

		return
	default:
		log.Info("received unknown/unsupported message type", "type", mt.String())
		span.SetAttributes(attribute.String("type", mt.String()))
		span.SetStatus(codes.Error, "received unknown message type")

		return
	}

	if _, err := conn.WriteTo(reply.ToBytes(), peer); err != nil {
		log.Error(err, "failed to send DHCP")
		span.SetStatus(codes.Error, err.Error())

		return
	}
	log.Info("sent DHCP response")
	span.SetAttributes(h.encodeToAttributes(reply, "reply")...)
	span.SetStatus(codes.Ok, "sent DHCP response")
}

// readBackend encapsulates the backend read and opentelemetry handling.
func (h *Handler) readBackend(ctx context.Context, mac net.HardwareAddr) (*data.Netboot, error) {
	h.setDefaults()

	tracer := otel.Tracer(tracerName)
	ctx, span := tracer.Start(ctx, "Hardware data get")
	defer span.End()

	_, n, err := h.Backend.Read(ctx, mac)
	if err != nil {
		h.Log.Error(err, "error getting DHCP data from backend", "mac", mac.String())
		span.SetStatus(codes.Error, err.Error())

		return nil, err
	}

	span.SetAttributes(n.EncodeToAttributes()...)
	span.SetStatus(codes.Ok, "done reading from backend")

	return n, nil
}

// updateMsg handles updating DHCP packets with the data from the backend.
func (h *Handler) updateMsg(ctx context.Context, pkt *dhcpv4.DHCPv4, n *data.Netboot, msgType dhcpv4.MessageType) *dhcpv4.DHCPv4 {
	mods := []dhcpv4.Modifier{
		dhcpv4.WithMessageType(msgType),
		dhcpv4.WithGeneric(dhcpv4.OptionServerIdentifier, h.IPAddr.IPAddr().IP),
		dhcpv4.WithServerIP(h.IPAddr.IPAddr().IP),
	}

	mods = append(mods, h.setNetworkBootOpts(ctx, pkt, n), setOpt97(pkt.GetOneOption(dhcpv4.OptionClientMachineIdentifier)))

	reply, err := dhcpv4.NewReplyFromRequest(pkt, mods...)
	if err != nil {
		return nil
	}

	return reply
}

// isNetbootClient returns true if the client is a valid netboot client.
// A valid netboot client will have the following in its DHCP request:
// http://www.pix.net/software/pxeboot/archive/pxespec.pdf
//
// 1. is a DHCP discovery/request message type.
// 2. option 93 is set.
// 3. option 94 is set.
// 4. option 97 is correct length.
// 5. option 60 is set with this format: "PXEClient:Arch:xxxxx:UNDI:yyyzzz" or "HTTPClient:Arch:xxxxx:UNDI:yyyzzz".
func (h *Handler) isNetbootClient(pkt *dhcpv4.DHCPv4) error {
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
	if !strings.HasPrefix(opt60, string(pxeClient)) && !strings.HasPrefix(opt60, string(httpClient)) {
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
		h.Log.Info("not a netboot client", "reason", "option 97 has invalid length (0 or 17)", "mac", pkt.ClientHWAddr.String(), "option 97", string(guid))
		return fmt.Errorf("option 97 has invalid length (must be 0 or 17): %v", len(guid))
	}

	return nil
}

// setNetworkBootOpts sets the network boot options for the DHCP reply, based on the PXE spec for a proxyDHCP server
// found here: http://www.pix.net/software/pxeboot/archive/pxespec.pdf
// set the following DHCP options:
// opt43, opt97, opt60, opt54
// set the following DHCP headers:
// siaddr, sname, bootfile.
func (h *Handler) setNetworkBootOpts(ctx context.Context, m *dhcpv4.DHCPv4, n *data.Netboot) dhcpv4.Modifier {
	// return dhcpv4.Modifier(func(pkt *dhcpv4.DHCPv4) {})
	// m is a received DHCPv4 packet.
	// d is the reply packet we are building.
	withNetboot := func(d *dhcpv4.DHCPv4) {
		// var opt60 string
		// if the client sends opt 60 with HTTPClient then we need to respond with opt 60
		one, opt60 := setOpt54And60AndSNAME(d.ClassIdentifier(), h.Netboot.IPXEBinServerTFTP.IP().IPAddr().IP, net.ParseIP(h.Netboot.IPXEBinServerHTTP.Host))
		one(d)
		d.BootFileName = "/netboot-not-allowed"
		d.ServerIPAddr = net.IPv4(0, 0, 0, 0)
		if n.AllowNetboot {
			a := arch(m)
			bin, found := ArchToBootFile[a]
			if !found {
				h.Log.Error(fmt.Errorf("unable to find bootfile for arch"), "network boot not allowed", "arch", a, "archInt", int(a), "mac", m.ClientHWAddr)
				return
			}
			uClass := UserClass(string(m.GetOneOption(dhcpv4.OptionUserClassInformation)))
			ipxeScript := h.Netboot.IPXEScriptURL
			if n.IPXEScriptURL != nil {
				ipxeScript = n.IPXEScriptURL
			}
			d.BootFileName, d.ServerIPAddr = h.bootfileAndNextServer(ctx, uClass, opt60, bin, h.Netboot.IPXEBinServerTFTP, h.Netboot.IPXEBinServerHTTP, ipxeScript)
			pxe := dhcpv4.Options{ // FYI, these are suboptions of option43. ref: https://datatracker.ietf.org/doc/html/rfc2132#section-8.4
				// PXE Boot Server Discovery Control - bypass, just boot from filename.
				6:  []byte{8},
				69: oteldhcp.TraceparentFromContext(ctx),
			}
			if rpi.IsRPI(m.ClientHWAddr) {
				h.Log.Info("this is a Raspberry Pi", "mac", m.ClientHWAddr)
				rpi.AddVendorOpts(pxe)
			}

			d.UpdateOption(dhcpv4.OptGeneric(dhcpv4.OptionVendorSpecificInformation, pxe.ToBytes()))
		}
	}

	return withNetboot
}

// bootfileAndNextServer returns the bootfile (string) and next server (net.IP).
// input arguments `tftp`, `ipxe` and `iscript` use non string types so as to attempt to be more clear about the expectation around what is wanted for these values.
// It also helps us avoid having to validate a string in multiple ways.
func (h *Handler) bootfileAndNextServer(ctx context.Context, uClass UserClass, opt60 clientType, bin string, tftp netaddr.IPPort, ipxe, iscript *url.URL) (string, net.IP) {
	var nextServer net.IP
	var bootfile string
	if tp := otelhelpers.TraceparentStringFromContext(ctx); h.OTELEnabled && tp != "" {
		bin = fmt.Sprintf("%s-%v", bin, tp)
	}
	// If a machine is in an ipxe boot loop, it is likely to be that we aren't matching on IPXE or Tinkerbell userclass (option 77).
	switch { // order matters here.
	case uClass == Tinkerbell, (h.Netboot.UserClass != "" && uClass == h.Netboot.UserClass): // this case gets us out of an ipxe boot loop.
		bootfile = "/no-ipxe-script-defined"
		if iscript != nil {
			bootfile = iscript.String()
		}
		nextServer = tftp.UDPAddr().IP
	case opt60 == httpClient: // Check the client type from option 60.
		bootfile = fmt.Sprintf("%s/%s", ipxe, bin)
		ihost := strings.Split(ipxe.Host, ":")[0]
		ns := net.ParseIP(ihost)
		if ns == nil {
			h.Log.Error(fmt.Errorf("unable to parse ipxe host"), "ipxe", ipxe.Host)
			ns = net.ParseIP("0.0.0.0")
		}
		nextServer = ns
	case uClass == IPXE: // if the "iPXE" user class is found it means we aren't in our custom version of ipxe, but because of the option 43 we're setting we need to give a full tftp url from which to boot.
		bootfile = fmt.Sprintf("tftp://%v/%v", tftp.String(), bin)
		nextServer = tftp.UDPAddr().IP
	default:
		bootfile = bin
		nextServer = tftp.UDPAddr().IP
	}

	return bootfile, nextServer
}

// encodeToAttributes takes a DHCP packet and returns opentelemetry key/value attributes.
func (h *Handler) encodeToAttributes(d *dhcpv4.DHCPv4, namespace string) []attribute.KeyValue {
	h.setDefaults()
	a := &oteldhcp.Encoder{Log: h.Log}

	return a.Encode(d, namespace, oteldhcp.AllEncoders()...)
}

// arch returns the arch of the client pulled from DHCP option 93.
func arch(d *dhcpv4.DHCPv4) iana.Arch {
	// get option 93 ; arch
	fwt := d.ClientArch()
	if len(fwt) == 0 {
		return iana.Arch(255) // unknown arch
	}
	if rpi.IsRPI(d.ClientHWAddr) {
		return iana.Arch(41) // rpi
	}
	var archKnown bool
	var a iana.Arch
	for _, elem := range fwt {
		if !strings.Contains(elem.String(), "unknown") {
			archKnown = true
			// Basic architecture identification, based purely on the PXE architecture option.
			// https://www.iana.org/assignments/dhcpv6-parameters/dhcpv6-parameters.xhtml#processor-architecture
			a = elem
			break
		}
	}
	if !archKnown {
		return iana.Arch(255) // unknown arch
	}

	return a
}

/*
// Handle is responsible for responding to netboot requests.
// It endeavors to satisfy the spec from section 2.5(?) of http://www.pix.net/software/pxeboot/archive/pxespec.pdf
func (h *Handler) Handle(conn net.PacketConn, peer net.Addr, m *dhcpv4.DHCPv4) {

	log := h.Log.WithValues("hwaddr", m.ClientHWAddr, "listenAddr", conn.LocalAddr())
	reply, err := dhcpv4.New(dhcpv4.WithReply(m),
		dhcpv4.WithGatewayIP(m.GatewayIPAddr),
		dhcpv4.WithOptionCopied(m, dhcpv4.OptionRelayAgentInformation),
	)
	if err != nil {
		log.Info("Generating a new transaction id failed, not a problem as we're passing one in, but if this message is showing up a lot then something could be up with github.com/insomniacslk/dhcp")
	}
	if m.OpCode != dhcpv4.OpcodeBootRequest { // TODO(jacobweinstock): dont understand this, found it in an example here: https://github.com/insomniacslk/dhcp/blob/c51060810aaab9c8a0bd1b0fcbf72bc0b91e6427/dhcpv4/server4/server_test.go#L31
		log.Info("Ignoring packet", "OpCode", m.OpCode)
		return
	}
	rp := replyPacket{DHCPv4: reply, log: log}

	if err := rp.validatePXE(m); err != nil {
		log.Info("Ignoring packet: not from a PXE enabled client", "error", err)
		return
	}

	if err := rp.setMessageType(m); err != nil {
		log.Info("Ignoring packet", "error", err.Error())
		return
	}

	mach, err := processMachine(m)
	if err != nil {
		log.Info("unable to parse arch or user class: unusable packet", "error", err.Error(), "mach", mach)
		return
	}

	// Set option 43
	rp.setOpt43(m.ClientHWAddr)

	// Set option 97
	if err := rp.setOpt97(m.GetOneOption(dhcpv4.OptionClientMachineIdentifier)); err != nil {
		log.Info("Ignoring packet", "error", err.Error())
		return
	}

	// set broadcast header to true
	reply.SetBroadcast()

	// Set option 60
	// The PXE spec says the server should identify itself as a PXEClient or HTTPCient
	if opt60 := m.GetOneOption(dhcpv4.OptionClassIdentifier); strings.HasPrefix(string(opt60), string(handler.PXEClient)) {
		reply.UpdateOption(dhcpv4.OptClassIdentifier(string(handler.PXEClient)))
	} else {
		reply.UpdateOption(dhcpv4.OptClassIdentifier(string(handler.HTTPClient)))
	}

	// Set option 54
	opt54 := rp.setOpt54(m.GetOneOption(dhcpv4.OptionClassIdentifier), h.Netboot.IPXEBinServerTFTP.UDPAddr().IP, h.HTTPAddr.TCPAddr().IP)

	// add the siaddr (IP address of next server) dhcp packet header to a given packet pkt.
	// see https://datatracker.ietf.org/doc/html/rfc2131#section-2
	// without this the pxe client will try to broadcast a request message to port 4011
	reply.ServerIPAddr = opt54

	// set sname header
	// see https://datatracker.ietf.org/doc/html/rfc2131#section-2
	rp.setSNAME(m.GetOneOption(dhcpv4.OptionClassIdentifier), h.TFTPAddr.UDPAddr().IP, h.HTTPAddr.TCPAddr().IP)

	// set bootfile header
	if err := rp.setBootfile(mach, h.UserClass, h.TFTPAddr, h.IPXEAddr, h.IPXEScript); err != nil {
		log.Info("Ignoring packet", "error", err.Error())
		return
	}
	// check the backend, if PXE is NOT allowed, set the boot file name to "/<mac address>/not-allowed"
	if !h.Allower.Allow(h.Ctx, mach.mac) {
		rp.BootFileName = fmt.Sprintf("/%v/not-allowed", mach.mac)
	}

	// send the DHCP packet
	if _, err := conn.WriteTo(reply.ToBytes(), peer); err != nil {
		log.Error(err, "failed to send ProxyDHCP offer")
		return
	}
	log.V(1).Info("DHCP packet received", "pkt", *m)
	log.Info("Sent ProxyDHCP message", "arch", mach.arch, "userClass", mach.uClass, "receivedMsgType", m.MessageType(), "replyMsgType", rp.MessageType(), "unicast", rp.IsUnicast(), "peer", peer, "bootfile", rp.BootFileName)
}

// validatePXE determines if the DHCP packet meets qualifications of a being a PXE enabled client.
// http://www.pix.net/software/pxeboot/archive/pxespec.pdf
// 1. is a DHCP discovery/request message type
// 2. option 93 is set
// 3. option 94 is set
// 4. option 97 is correct length.
// 5. option 60 is set with this format: "PXEClient:Arch:xxxxx:UNDI:yyyzzz" or "HTTPClient:Arch:xxxxx:UNDI:yyyzzz"
// 6. option 55 is set; only warn if not set
// 7. options 128-135 are set; only warn if not set.
func (r replyPacket) validatePXE(pkt *dhcpv4.DHCPv4) error {
	// only response to DISCOVER and REQUEST packets
	if pkt.MessageType() != dhcpv4.MessageTypeDiscover && pkt.MessageType() != dhcpv4.MessageTypeRequest {
		return ErrInvalidMsgType{Invalid: pkt.MessageType()}
	}
	// option 55 must be set
	if !pkt.Options.Has(dhcpv4.OptionParameterRequestList) {
		// just warn for the moment because we don't actually do anything with this option
		r.log.V(1).Info("warning: missing option 55")
	}
	// option 60 must be set
	if !pkt.Options.Has(dhcpv4.OptionClassIdentifier) {
		return ErrOpt60Missing
	}
	// option 60 must start with PXEClient or HTTPClient
	opt60 := pkt.GetOneOption(dhcpv4.OptionClassIdentifier)
	if !strings.HasPrefix(string(opt60), string(pxeClient)) && !strings.HasPrefix(string(opt60), string(httpClient)) {
		return ErrInvalidOption60{Opt60: string(opt60)}
	}
	// option 93 must be set
	if !pkt.Options.Has(dhcpv4.OptionClientSystemArchitectureType) {
		return ErrOpt93Missing
	}

	// option 94 must be set
	if !pkt.Options.Has(dhcpv4.OptionClientNetworkInterfaceIdentifier) {
		return ErrOpt94Missing
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
			return ErrOpt97LeadingByteError
		}
	default:
		return ErrOpt97WrongSize
	}
	// the pxe spec seems to indicate that options 128-135 must be set.
	// these show up as required in https://www.rfc-editor.org/rfc/rfc4578.html#section-2.4
	// We're just warning on them for now as we're not using them.
	opts := []dhcpv4.OptionCode{
		dhcpv4.OptionTFTPServerIPAddress,
		dhcpv4.OptionCallServerIPAddress,
		dhcpv4.OptionDiscriminationString,
		dhcpv4.OptionRemoteStatisticsServerIPAddress,
		dhcpv4.Option8021PVLANID,
		dhcpv4.Option8021QL2Priority,
		dhcpv4.OptionDiffservCodePoint,
		dhcpv4.OptionHTTPProxyForPhoneSpecificApplications,
	}
	for _, opt := range opts {
		if v := pkt.GetOneOption(opt); v == nil {
			r.log.V(1).Info("warning: missing option", "opt", opt)
		}
	}

	return nil
}

// processMachine takes a DHCP packet and returns a populated machine struct.
func processMachine(pkt *dhcpv4.DHCPv4) (machine, error) {
	mach := machine{}
	// get option 93 ; arch
	fwt := pkt.ClientArch()
	if len(fwt) == 0 {
		return mach, ErrUnknownArch
	}
	// TODO(jacobweinstock): handle unknown arch, better?
	var archKnown bool
	for _, elem := range fwt {
		if !strings.Contains(elem.String(), "unknown") {
			archKnown = true
			// Basic architecture identification, based purely on
			// the PXE architecture option.
			// https://www.iana.org/assignments/dhcpv6-parameters/dhcpv6-parameters.xhtml#processor-architecture
			mach.arch = elem
			break
		}
	}
	if !archKnown {
		return mach, ErrUnknownArch
	}

	// set option 77 from received packet
	mach.uClass = UserClass(string(pkt.GetOneOption(dhcpv4.OptionUserClassInformation)))
	// set the client type based off of option 60
	opt60 := pkt.GetOneOption(dhcpv4.OptionClassIdentifier)
	if strings.HasPrefix(string(opt60), string(pxeClient)) {
		mach.cType = pxeClient
	} else if strings.HasPrefix(string(opt60), string(httpClient)) {
		mach.cType = httpClient
	}
	mach.mac = pkt.ClientHWAddr

	return mach, nil
}

// Transformer for merging the netaddr.IPPort and logr.Logger structs.
func (h *Handler) Transformer(typ reflect.Type) func(dst, src reflect.Value) error {
	switch typ {
	case reflect.TypeOf(logr.Logger{}):
		return func(dst, src reflect.Value) error {
			if dst.CanSet() {
				isZero := dst.MethodByName("GetSink")
				result := isZero.Call(nil)
				if result[0].IsNil() {
					dst.Set(src)
				}
			}
			return nil
		}
	case reflect.TypeOf(netaddr.IPPort{}):
		return func(dst, src reflect.Value) error {
			if dst.CanSet() {
				isZero := dst.MethodByName("IsZero")
				result := isZero.Call([]reflect.Value{})
				if result[0].Bool() {
					dst.Set(src)
				}
			}
			return nil
		}
	case reflect.TypeOf(netaddr.IP{}):
		return func(dst, src reflect.Value) error {
			if dst.CanSet() {
				isZero := dst.MethodByName("IsZero")
				result := isZero.Call([]reflect.Value{})
				if result[0].Bool() {
					dst.Set(src)
				}
			}
			return nil
		}
	case reflect.TypeOf(h.Allower):
		return func(dst, src reflect.Value) error {
			return nil
		}
	}

	return nil
}
*/

func setOpt97(guid []byte) dhcpv4.Modifier {
	return func(d *dhcpv4.DHCPv4) {
		d.UpdateOption(dhcpv4.OptGeneric(dhcpv4.OptionClientMachineIdentifier, guid))
	}
}

// setOpt54 based on option 60.
func setOpt54And60AndSNAME(opt60FromClient string, tftp net.IP, http net.IP) (dhcpv4.Modifier, clientType) {
	opt54 := tftp
	opt60 := pxeClient
	if strings.HasPrefix(opt60FromClient, string(httpClient)) {
		opt54 = http
		opt60 = httpClient
	}

	return func(d *dhcpv4.DHCPv4) {
		d.UpdateOption(dhcpv4.OptClassIdentifier(opt60.String()))
		d.ServerHostName = opt54.String()
	}, opt60
}
