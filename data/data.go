// Package data is an interface between DHCP backend implementations and the DHCP server.
package data

import (
	"encoding/json"
	"fmt"
	"net"
	"net/url"
	"strings"

	"go.opentelemetry.io/otel/attribute"
	"inet.af/netaddr"
)

var errUnmarshal = fmt.Errorf("unable to unmarshal")

// DHCP holds the headers and options available to be set in a DHCP server response.
// This is the API between the DHCP server and a backend.
type DHCP struct {
	MACAddress       net.HardwareAddr // chaddr DHCP header.
	IPAddress        netaddr.IP       // yiaddr DHCP header.
	SubnetMask       net.IPMask       // DHCP option 1.
	DefaultGateway   netaddr.IP       // DHCP option 3.
	NameServers      []net.IP         // DHCP option 6.
	Hostname         string           // DHCP option 12.
	DomainName       string           // DHCP option 15.
	BroadcastAddress netaddr.IP       // DHCP option 28.
	NTPServers       []net.IP         // DHCP option 42.
	LeaseTime        uint32           // DHCP option 51.
	DomainSearch     []string         // DHCP option 119.
}

// Netboot holds info used in netbooting a client.
type Netboot struct {
	AllowNetboot  bool     // If true, the client will be provided netboot options in the DHCP offer/ack.
	IPXEScriptURL *url.URL // Overrides a default value that is passed into DHCP on startup.
}

// EncodeToAttributes returns a slice of opentelemetry attributes that can be used to set span.SetAttributes.
func (d *DHCP) EncodeToAttributes() []attribute.KeyValue {
	var result []attribute.KeyValue
	var ns []string
	for _, e := range d.NameServers {
		ns = append(ns, e.String())
	}
	if len(ns) > 0 {
		result = append(result, attribute.String("DHCP.NameServers", strings.Join(ns, ",")))
	}

	var ntp []string
	for _, e := range d.NTPServers {
		ntp = append(ntp, e.String())
	}
	if len(ntp) > 0 {
		result = append(result, attribute.String("DHCP.NTPServers", strings.Join(ntp, ",")))
	}

	if !d.IPAddress.IsZero() {
		result = append(result, attribute.String("DHCP.IPAddress", d.IPAddress.String()))
	}

	if d.SubnetMask != nil {
		result = append(result, attribute.String("DHCP.SubnetMask", net.IP(d.SubnetMask).String()))
	}

	if !d.DefaultGateway.IsZero() {
		result = append(result, attribute.String("DHCP.DefaultGateway", d.DefaultGateway.String()))
	}

	if !d.BroadcastAddress.IsZero() {
		result = append(result, attribute.String("DHCP.BroadcastAddress", d.BroadcastAddress.String()))
	}

	if d.MACAddress != nil {
		result = append(result, attribute.String("DHCP.MACAddress", d.MACAddress.String()))
	}

	if d.Hostname != "" {
		result = append(result, attribute.String("DHCP.Hostname", d.Hostname))
	}

	if d.DomainName != "" {
		result = append(result, attribute.String("DHCP.DomainName", d.DomainName))
	}

	if d.LeaseTime != 0 {
		result = append(result, attribute.Int64("DHCP.LeaseTime", int64(d.LeaseTime)))
	}

	if len(d.DomainSearch) > 0 {
		result = append(result, attribute.String("DHCP.DomainSearch", strings.Join(d.DomainSearch, ",")))
	}

	return result
}

// EncodeToAttributes returns a slice of opentelemetry attributes that can be used to set span.SetAttributes.
func (n *Netboot) EncodeToAttributes() []attribute.KeyValue {
	result := []attribute.KeyValue{
		attribute.Bool("Netboot.AllowNetboot", n.AllowNetboot),
	}
	if n.IPXEScriptURL != nil {
		result = append(result, attribute.String("Netboot.IPXEScriptURL", n.IPXEScriptURL.String()))
	}

	return result
}

// MarshalJSON is the custom marshaller for the DHCP struct.
func (d *DHCP) MarshalJSON() ([]byte, error) {
	dhcp := struct {
		MACAddress       string   `json:"MACAddress"`
		IPAddress        string   `json:"IPAddress"`
		SubnetMask       string   `json:"SubnetMask"`
		DefaultGateway   string   `json:"DefaultGateway"`
		NameServers      []string `json:"NameServers"`
		Hostname         string   `json:"Hostname"`
		DomainName       string   `json:"DomainName"`
		BroadcastAddress string   `json:"BroadcastAddress"`
		NTPServers       []string `json:"NTPServers"`
		LeaseTime        uint32   `json:"LeaseTime"`
		DomainSearch     []string `json:"DomainSearch"`
	}{
		MACAddress:       d.MACAddress.String(),
		IPAddress:        d.IPAddress.String(),
		SubnetMask:       net.IP(d.SubnetMask).String(),
		DefaultGateway:   d.DefaultGateway.String(),
		Hostname:         d.Hostname,
		DomainName:       d.DomainName,
		BroadcastAddress: d.BroadcastAddress.String(),
		LeaseTime:        d.LeaseTime,
		DomainSearch:     d.DomainSearch,
	}
	if d.IPAddress.IsZero() {
		dhcp.IPAddress = ""
	}
	if d.SubnetMask == nil {
		dhcp.SubnetMask = ""
	}
	if d.DefaultGateway.IsZero() {
		dhcp.DefaultGateway = ""
	}
	if d.BroadcastAddress.IsZero() {
		dhcp.BroadcastAddress = ""
	}

	for _, elem := range d.NameServers {
		dhcp.NameServers = append(dhcp.NameServers, elem.String())
	}
	for _, elem := range d.NTPServers {
		dhcp.NTPServers = append(dhcp.NTPServers, elem.String())
	}

	return json.Marshal(dhcp)
}

// UnmarshalJSON is the custom unmarshaller for the DHCP struct.
func (d *DHCP) UnmarshalJSON(j []byte) error { // nolint: cyclop // TODO(jacobweinstock): Can I refactor this?
	var rawStrings map[string]interface{}
	err := json.Unmarshal(j, &rawStrings)
	if err != nil {
		return fmt.Errorf("%v: %w", err, errUnmarshal)
	}

	for k, v := range rawStrings {
		if v == nil {
			continue
		}
		if uv, ok := v.(string); ok && uv == "" {
			continue
		}
		switch strings.ToLower(k) {
		case "macaddress":
			uv, ok := v.(string)
			if !ok {
				return fmt.Errorf("unable to type assert macaddress: %w", errUnmarshal)
			}
			if d.MACAddress, err = net.ParseMAC(uv); err != nil {
				return fmt.Errorf("%v: %w", err, errUnmarshal)
			}
		case "ipaddress":
			uv, ok := v.(string)
			if !ok {
				return fmt.Errorf("unable to type assert ipaddress: %w", errUnmarshal)
			}
			if d.IPAddress, err = netaddr.ParseIP(uv); err != nil {
				return fmt.Errorf("failed to parse ipaddress: %v: %w", err, errUnmarshal)
			}
		case "subnetmask":
			uv, ok := v.(string)
			if !ok {
				return fmt.Errorf("unable to type assert subnetmask: %w", errUnmarshal)
			}
			ip := net.ParseIP(uv)
			if ip == nil {
				return fmt.Errorf("failed parse subnetmask: %w", errUnmarshal)
			}
			if d.SubnetMask = net.IPMask(ip.To4()); d.SubnetMask == nil {
				return fmt.Errorf("failed to parse subnetmask: %v: %w", v, errUnmarshal)
			}
		case "defaultgateway":
			uv, ok := v.(string)
			if !ok {
				return fmt.Errorf("unable to type assert defaultgateway: %w", errUnmarshal)
			}
			if d.DefaultGateway, err = netaddr.ParseIP(uv); err != nil {
				return fmt.Errorf("failed to parse defaultgateway: %v: %w", v, errUnmarshal)
			}
		case "nameservers":
			for _, elem := range v.([]interface{}) {
				uv, ok := elem.(string)
				if !ok {
					return fmt.Errorf("unable to type assert nameserver: %w", errUnmarshal)
				}
				if uv == "" {
					continue
				}
				d.NameServers = append(d.NameServers, net.ParseIP(uv))
			}
		case "hostname":
			uv, ok := v.(string)
			if !ok {
				return fmt.Errorf("unable to type assert hostname: %w", errUnmarshal)
			}
			d.Hostname = uv
		case "domainname":
			uv, ok := v.(string)
			if !ok {
				return fmt.Errorf("unable to type assert domainname: %w", errUnmarshal)
			}
			d.DomainName = uv
		case "broadcastaddress":
			uv, ok := v.(string)
			if !ok {
				return fmt.Errorf("unable to type assert broadcastaddress: %w", errUnmarshal)
			}
			if d.BroadcastAddress, err = netaddr.ParseIP(uv); err != nil {
				return fmt.Errorf("failed to parse broadcastaddress: %v: %w", err, errUnmarshal)
			}
		case "ntpservers":
			for _, elem := range v.([]interface{}) {
				uv, ok := elem.(string)
				if !ok {
					return fmt.Errorf("unable to type assert ntpservers: %w", errUnmarshal)
				}
				if uv == "" {
					continue
				}
				d.NTPServers = append(d.NTPServers, net.ParseIP(uv))
			}
		case "leasetime":
			uv, ok := v.(float64)
			if !ok {
				return fmt.Errorf("unable to type assert leasetime: %w", errUnmarshal)
			}
			d.LeaseTime = uint32(uv)
		case "domainsearch":
			for _, elem := range v.([]interface{}) {
				uv, ok := elem.(string)
				if !ok {
					return fmt.Errorf("unable to type assert domainsearch: %w", errUnmarshal)
				}
				if uv == "" {
					continue
				}
				d.DomainSearch = append(d.DomainSearch, uv)
			}
		default:
			return fmt.Errorf("unknown key: %v: %w", k, errUnmarshal)
		}
	}

	return nil
}
