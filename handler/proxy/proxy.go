// Package proxy implements proxyDHCP interactions.
package proxy

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
	UserClass option.UserClass
}

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
		h.Log.Error(err, "error from backend", "mac", mac.String())

		return nil, err
	}

	if n.AllowNetboot {
		if err := option.IsNetbootClient(input); err != nil {
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
		log = log.WithValues("type", dhcpv4.MessageTypeOffer.String())
		var err error
		reply, err = h.handleMsg(ctx, pkt.ClientHWAddr, pkt, dhcpv4.MessageTypeOffer)
		if err != nil {
			span.SetAttributes(attribute.String("error", err.Error()))
			span.SetStatus(codes.Error, err.Error())

			return
		}
	case dhcpv4.MessageTypeRequest:
		log = log.WithValues("type", dhcpv4.MessageTypeAck.String())
		var err error
		reply, err = h.handleMsg(ctx, pkt.ClientHWAddr, pkt, dhcpv4.MessageTypeAck)
		if err != nil {
			span.SetAttributes(attribute.String("error", err.Error()))
			span.SetStatus(codes.Error, err.Error())

			return
		}
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

// Name returns the name of the handler.
func (h *Handler) Name() string {
	return "proxyDHCP"
}

// readBackend encapsulates the backend read and opentelemetry handling.
func (h *Handler) readBackend(ctx context.Context, mac net.HardwareAddr) (*data.Netboot, error) {
	h.setDefaults()

	tracer := otel.Tracer(tracerName)
	ctx, span := tracer.Start(ctx, "Hardware data get")
	defer span.End()

	_, n, err := h.Backend.Read(ctx, mac)
	if err != nil {
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
		dhcpv4.WithServerIP(h.Netboot.IPXEBinServerTFTP.UDPAddr().IP),
		setOpt97(pkt.GetOneOption(dhcpv4.OptionClientMachineIdentifier)),
		// func(d *dhcpv4.DHCPv4) { d.ServerHostName = h.IPAddr.String() },
	}
	mods = append(mods, option.Conf{
		Log:               h.Log,
		IPXEScriptURL:     h.Netboot.IPXEScriptURL,
		UserClass:         h.Netboot.UserClass,
		IPXEBinServerTFTP: h.Netboot.IPXEBinServerTFTP,
		IPXEBinServerHTTP: h.Netboot.IPXEBinServerHTTP,
		OTELEnabled:       h.OTELEnabled,
	}.SetNetworkBootOpts(ctx, pkt, n)...)

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

func setOpt97(guid []byte) dhcpv4.Modifier {
	return func(d *dhcpv4.DHCPv4) {
		d.UpdateOption(dhcpv4.OptGeneric(dhcpv4.OptionClientMachineIdentifier, guid))
	}
}
