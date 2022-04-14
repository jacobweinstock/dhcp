package relay

import (
	"fmt"
	"net"

	"github.com/insomniacslk/dhcp/dhcpv4"
)

func (c *Config) handleFunc(conn net.PacketConn, peer net.Addr, m *dhcpv4.DHCPv4) {
	c.Logger.Info("received DHCP packet", "type", m.MessageType().String())

	reply, err := dhcpv4.FromBytes(m.ToBytes())
	if err != nil {
		c.Logger.Error(err, "failed to parse DHCPv4 packet")
		return
	}
	m.HopCount++ // this is required: https://datatracker.ietf.org/doc/html/rfc1542#section-4.1.1
	reply.GatewayIPAddr = net.IPv4zero
	// c.setGIADDR(reply) // always set the giaddr to the listener ip
	// validate
	if !c.hopsValid(reply) {
		c.Logger.Info("hop count exceeded", "maxHops", c.MaxHops, "hopCount", reply.HopCount)
		return
	}

	var dst net.Addr
	switch reply.OpCode {
	case dhcpv4.OpcodeBootRequest: // relay from a client to a DHCP server
		reply.SetUnicast()
		dst = c.DHCPServer
		ip := c.Listener.Addr().String()
		if !m.GatewayIPAddr.Equal(net.IPv4zero) && !m.GatewayIPAddr.Equal(net.ParseIP(ip)) {
			dst = &net.UDPAddr{IP: m.GatewayIPAddr, Port: 67}
		}
		c.setGIADDR(reply)
	case dhcpv4.OpcodeBootReply: // relay from a DHCP server to a client
		// if giaddr doesnt match c.Listener.Addr, then this we must discard. see https://datatracker.ietf.org/doc/html/rfc1542#section-4.1.2
		ip := c.Listener.Addr().String()
		if !m.GatewayIPAddr.Equal(net.ParseIP(ip)) {
			return
		}
		c.setCastType(reply)
		dst = c.setDest(m)
		if !m.GatewayIPAddr.Equal(net.IPv4zero) && !m.GatewayIPAddr.Equal(net.ParseIP(ip)) {
			c.setGIADDR(reply)
		}
	default: // drop the packet
		return
	}

	c.Logger.Info("DEBUGGING", "dst", dst)
	if _, err := conn.WriteTo(reply.ToBytes(), dst); err != nil {
		c.Logger.Error(err, "failed send DHCP packet")
		return
	}
	c.Logger.Info("sent DHCP packet", "type", reply.MessageType().String())
	fmt.Println(reply.SummaryWithVendor(nil))
}

// BOOTREQUEST message checks
// https://datatracker.ietf.org/doc/html/rfc1542#section-4.1.1

// hopsValid checks if the hop count is less than the maximum hops.
// max hops is 16, default is 4.
// https://datatracker.ietf.org/doc/html/rfc1542#section-4.1.1
func (c *Config) hopsValid(d *dhcpv4.DHCPv4) bool {
	if d.HopCount > uint8(16) {
		return false
	}
	return d.HopCount < c.MaxHops
}

func (c *Config) setGIADDR(d *dhcpv4.DHCPv4) {
	ip := c.Listener.Addr().String()
	d.GatewayIPAddr = net.ParseIP(ip)
}

// BOOTREPLY message checks
// https://datatracker.ietf.org/doc/html/rfc1542#section-4.1.2

// 1. GIADDR field must match the c.Listener.Addr() or the packet is silently dropped.
// 2. whether to unicast or broadcast the DHCP message to a client. ref: https://datatracker.ietf.org/doc/html/rfc1542#section-5.4
//   Unicast if
//     1. the 'giaddr' field is non-zero
//     OR
//     2. the 'ciaddr' field is non-zero
// 3. Where (ip/port) to send the DHCP message?
//   1. the 'giaddr' field is non-zero -> ip: giaddr, port: 67
//   2. the 'ciaddr' field is non-zero -> ip: ciaddr, port: 68
//   3. the 'ciaddr' field is zero -> ip: net.IPv4bcast, port: 68

func (c *Config) setCastType(d *dhcpv4.DHCPv4) {
	d.SetUnicast()
	if d.GatewayIPAddr.Equal(net.IPv4zero) || d.ClientIPAddr.Equal(net.IPv4zero) {
		d.SetBroadcast()
	}
}

func (c *Config) setDest(d *dhcpv4.DHCPv4) net.Addr {
	var dst net.Addr
	fmt.Println("d.GatewayIPAddr", d.GatewayIPAddr.String())
	ip := c.Listener.Addr().String()
	if !d.GatewayIPAddr.Equal(net.IPv4zero) && !d.GatewayIPAddr.Equal(net.ParseIP(ip)) {
		dst = &net.UDPAddr{IP: d.GatewayIPAddr, Port: 67}
		// also set GIADDR
	} else if !d.ClientIPAddr.Equal(net.IPv4zero) {
		dst = &net.UDPAddr{IP: d.ClientIPAddr, Port: 68}
	} else {
		dst = &net.UDPAddr{IP: net.IPv4bcast, Port: 68}
	}

	return dst
}
