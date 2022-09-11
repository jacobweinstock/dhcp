package reservation

import (
	"context"
	"errors"
	"fmt"
	"net"

	"github.com/go-logr/logr"
	"github.com/insomniacslk/dhcp/dhcpv4"
	"github.com/tinkerbell/dhcp/backend/noop"
	"github.com/tinkerbell/dhcp/data"
	"github.com/tinkerbell/dhcp/handler/option"
	oteldhcp "github.com/tinkerbell/dhcp/otel"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"
)

const tracerName = "github.com/tinkerbell/dhcp/reservation"

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
		d, n, err := h.readBackend(ctx, pkt.ClientHWAddr)
		if err != nil {
			log.Error(err, "error reading from backend")
			span.SetStatus(codes.Error, err.Error())

			return
		}

		reply = h.updateMsg(ctx, pkt, d, n, dhcpv4.MessageTypeOffer)
		log = log.WithValues("type", dhcpv4.MessageTypeOffer.String())
	case dhcpv4.MessageTypeRequest:
		d, n, err := h.readBackend(ctx, pkt.ClientHWAddr)
		if err != nil {
			log.Error(err, "error reading from backend")
			span.SetStatus(codes.Error, err.Error())

			return
		}
		reply = h.updateMsg(ctx, pkt, d, n, dhcpv4.MessageTypeAck)
		log = log.WithValues("type", dhcpv4.MessageTypeAck.String())
	case dhcpv4.MessageTypeRelease:
		// Since the design of this DHCP server is that all IP addresses are
		// Host reservations, when a client releases an address, the server
		// doesn't have anything to do. This case is included for clarity of this
		// design decision.
		log.Info("received release, no response required")
		span.SetStatus(codes.Ok, "received release, no response required")

		return
	default:
		log.Info("received unknown message type")
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
func (h *Handler) readBackend(ctx context.Context, mac net.HardwareAddr) (*data.DHCP, *data.Netboot, error) {
	h.setDefaults()

	tracer := otel.Tracer(tracerName)
	ctx, span := tracer.Start(ctx, "Hardware data get")
	defer span.End()

	d, n, err := h.Backend.Read(ctx, mac)
	if err != nil {
		h.Log.Info("error getting DHCP data from backend", "mac", mac, "error", err)
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
	/*vlan := dhcpv4.Options{
		38:  []byte{12},
		190: []byte("myuser"),
	}*/
	mods := []dhcpv4.Modifier{
		dhcpv4.WithMessageType(msgType),
		dhcpv4.WithGeneric(dhcpv4.OptionServerIdentifier, h.IPAddr.IPAddr().IP),
		dhcpv4.WithServerIP(h.IPAddr.IPAddr().IP),
		// dhcpv4.WithGeneric(dhcpv4.Option8021PVLANID, []byte{0x00, 0x00, 0x00, 0xC}),
		// dhcpv4.WithGeneric(dhcpv4.OptionEtherboot, vlan.ToBytes()),
	}
	mods = append(mods, d.ToDHCPMods()...)

	if h.Netboot.Enabled {
		if err := option.IsNetbootClient(pkt); err == nil {
			mods = append(mods, h.setNetworkBootOpts(ctx, pkt, n))
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
	a := &oteldhcp.Encoder{Log: h.Log}

	return a.Encode(d, namespace, oteldhcp.AllEncoders()...)
}
