package relay

import (
	"fmt"
	"net"

	"github.com/insomniacslk/dhcp/dhcpv4"
)

func (c *Config) handleFunc(conn net.PacketConn, peer net.Addr, m *dhcpv4.DHCPv4) {
	fmt.Println("===============")
	c.Logger.Info("received DHCP packet", "type", m.MessageType().String())

	/*
		var reply *dhcpv4.DHCPv4
		switch mt := m.MessageType(); mt {
		case dhcpv4.MessageTypeDiscover:

		case dhcpv4.MessageTypeOffer:

		case dhcpv4.MessageTypeRequest:

		case dhcpv4.MessageTypeAck:

		case dhcpv4.MessageTypeRelease:

		case dhcpv4.MessageTypeInform:

		case dhcpv4.MessageTypeDecline:

		case dhcpv4.MessageTypeNak:

		case dhcpv4.MessageTypeNone:

		default:

			return
		}
	*/
	m.HopCount++
	m.GatewayIPAddr = net.IP(c.Listener.Addr().AsSlice())

	fmt.Println(m.SummaryWithVendor(nil))

	// Send to DHCP server
	if m.OpCode == dhcpv4.OpcodeBootRequest {
		if _, err := conn.WriteTo(m.ToBytes(), c.DHCPServer); err != nil {

			return
		}

		return
	}

	// Send to DHCP client
	if _, err := conn.WriteTo(m.ToBytes(), &net.UDPAddr{IP: net.IPv4bcast, Port: 68}); err != nil {

		return
	}
}
