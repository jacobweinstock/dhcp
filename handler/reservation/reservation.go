package reservation

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/url"

	"github.com/go-logr/logr"
	"github.com/insomniacslk/dhcp/dhcpv4"
	"github.com/tinkerbell/dhcp/backend/noop"
	"github.com/tinkerbell/dhcp/data"
	"github.com/tinkerbell/dhcp/option"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"
	"inet.af/netaddr"
)

const tracerName = "github.com/tinkerbell/dhcp/reservation"

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

	// DHCPEnabled turns on and off serving of generic IP data.
	DHCPEnabled bool

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
	UserClass option.UserClass
}

// BackendReader is the interface that wraps the Read method.
//
// Backends implement this interface to provide DHCP data to the DHCP server.
type BackendReader interface {
	// Read data (from a backend) based on a mac address
	// and return DHCP headers and options, including netboot info.
	Read(context.Context, net.HardwareAddr) (*data.DHCP, *data.Netboot, error)
	// Name returns the name of the backend.
	Name() string
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

// Handle responds to DHCP messages with DHCP server options.
func (h *Handler) Handle(conn net.PacketConn, peer net.Addr, pkt *dhcpv4.DHCPv4) {
	h.setDefaults()
	if pkt == nil {
		h.Log.Error(errors.New("incoming packet is nil"), "not able to respond when the incoming packet is nil")
		return
	}

	log := h.Log.WithValues("mac", pkt.ClientHWAddr.String(), "receivedMsgType", pkt.MessageType())
	log.Info("received DHCP packet")
	tracer := otel.Tracer(tracerName)
	ctx, span := tracer.Start(context.Background(),
		fmt.Sprintf("DHCP Packet Received: %v", pkt.MessageType().String()),
		trace.WithAttributes(h.encodeToAttributes(pkt, "request")...),
	)
	defer span.End()

	switch mt := pkt.MessageType(); mt {
	case dhcpv4.MessageTypeRelease, dhcpv4.MessageTypeDecline, dhcpv4.MessageTypeNak:
		// Since the design of this DHCP server is that all IP addresses are
		// Host reservations, when a client releases, declines, nacks an address, the server
		// doesn't have anything to do. The DHCP spec also indicates that for a release no response is sent in reply.
		// This case is included for clarity of this design decision.
		log.Info("no response required")
		span.SetStatus(codes.Ok, fmt.Sprintf("received %v, no response required", mt.String()))

		return
	case dhcpv4.MessageTypeInform:
		// TODO: should this do something? Look up the DHCP spec and see.
		log.Info("no response required")
		span.SetStatus(codes.Ok, fmt.Sprintf("received %v, no response required", mt.String()))

		return
	case dhcpv4.MessageTypeDiscover, dhcpv4.MessageTypeRequest:
		// continue
	default:
		log.Info("received unknown message type")
		span.SetStatus(codes.Error, fmt.Sprintf("received unknown message type: %v", mt.String()))

		return
	}

	d, n, err := h.readBackend(ctx, pkt.ClientHWAddr)
	if err != nil {
		log.Error(err, "error from backend")
		span.SetStatus(codes.Error, err.Error())

		return
	}

	var reply *dhcpv4.DHCPv4
	if pkt.MessageType() == dhcpv4.MessageTypeRequest {
		reply = h.updateMsg(ctx, pkt, d, n, dhcpv4.MessageTypeAck)
		log = log.WithValues("sentMsgtype", dhcpv4.MessageTypeAck.String())
	} else {
		reply = h.updateMsg(ctx, pkt, d, n, dhcpv4.MessageTypeOffer)
		log = log.WithValues("sentMsgtype", dhcpv4.MessageTypeOffer.String())
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

// Name returns the name of the handler.
func (h *Handler) Name() string {
	return "reservation"
}

// readBackend encapsulates the backend read and opentelemetry handling.
func (h *Handler) readBackend(ctx context.Context, mac net.HardwareAddr) (*data.DHCP, *data.Netboot, error) {
	h.setDefaults()

	tracer := otel.Tracer(tracerName)
	ctx, span := tracer.Start(ctx, "Hardware data get")
	defer span.End()

	d, n, err := h.Backend.Read(ctx, mac)
	if err != nil {
		//h.Log.Info("error getting DHCP data from backend", "mac", mac, "error", err)
		span.SetStatus(codes.Error, err.Error())

		return nil, nil, err
	}

	span.SetAttributes(d.EncodeToAttributes()...)
	span.SetAttributes(n.EncodeToAttributes()...)
	span.SetStatus(codes.Ok, "done reading from backend")

	return d, n, nil
}

// updateMsg handles updating DHCP packets with the data from the backend.
func (h *Handler) updateMsg(ctx context.Context, pkt *dhcpv4.DHCPv4, d *data.DHCP, n *data.Netboot, msgType dhcpv4.MessageType) *dhcpv4.DHCPv4 {
	h.setDefaults()
	mods := []dhcpv4.Modifier{
		dhcpv4.WithMessageType(msgType),
		dhcpv4.WithGeneric(dhcpv4.OptionServerIdentifier, h.IPAddr.IPAddr().IP),
		// option.SetOpt60(pkt.ClassIdentifier()), // this is needed if running a proxyDHCP server on port 4011 on the same IP as the DHCP server.
	}
	if h.DHCPEnabled {
		mods = append(mods, d.ToDHCPMods()...)
	}

	// if n.AllowNetboot is false, we might want to sent bootfile to "/not-allowed"?
	// trade off of not doing this is the machine will have to wait for the DHCP timeout to move to the next boot device.
	if h.Netboot.Enabled && n.AllowNetboot {
		if err := option.IsNetbootClient(pkt); err == nil {
			nb := option.Conf{
				Log:               h.Log,
				IPXEScriptURL:     h.Netboot.IPXEScriptURL,
				UserClass:         h.Netboot.UserClass,
				IPXEBinServerTFTP: h.Netboot.IPXEBinServerTFTP,
				IPXEBinServerHTTP: h.Netboot.IPXEBinServerHTTP,
				OTELEnabled:       h.OTELEnabled,
			}.SetNetworkBootOpts(ctx, pkt, n)
			mods = append(mods, nb...)
		}
	}
	reply, err := dhcpv4.NewReplyFromRequest(pkt, mods...)
	if err != nil {
		return nil
	}

	return reply
}

// encodeToAttributes takes a DHCP packet and returns opentelemetry key/value attributes.
func (h *Handler) encodeToAttributes(d *dhcpv4.DHCPv4, namespace string) []attribute.KeyValue {
	h.setDefaults()
	a := &option.Otel{Log: h.Log}

	return a.Encode(d, namespace, option.AllOtelEncoders()...)
}
