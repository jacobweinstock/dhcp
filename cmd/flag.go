package main

import (
	"fmt"
	"net/url"
	"strings"

	"inet.af/netaddr"
)

type handlers []string

// String returns the string representation of the flag.
func (h *handlers) String() string {
	n := strings.Join(*h, ",")
	return fmt.Sprintf("%v", n)
}

// Set sets the value of the flag.
func (h *handlers) Set(value string) error {
	n := strings.Split(value, ",")
	*h = n
	return nil
}

type dhcpAddr netaddr.IP

// String returns the string representation of the flag.
func (d *dhcpAddr) String() string {
	n := netaddr.IP(*d)
	if n.IsZero() {
		return ""
	}

	return n.String()
}

// Set sets the value of the flag.
func (d *dhcpAddr) Set(value string) error {
	ip, err := netaddr.ParseIP(value)
	if err != nil {
		return err
	}
	*d = dhcpAddr(ip)
	return nil
}

// IPXETFTP is a flag.Value for a IPPort.
type IPXETFTP netaddr.IPPort

// String returns the string representation of the flag.
func (i *IPXETFTP) String() string {
	n := netaddr.IPPort(*i)
	return fmt.Sprintf("%v", n)
}

// Set sets the value of the flag.
func (i *IPXETFTP) Set(value string) error {
	ipport, err := netaddr.ParseIPPort(value)
	if err != nil {
		return err
	}
	*i = IPXETFTP(ipport)
	return nil
}

// IPXEHTTP is a flag.Value for a URL.
type IPXEHTTP url.URL

// String returns the string representation of the flag.
func (i *IPXEHTTP) String() string {
	u := url.URL(*i)
	return u.String()
}

// Set sets the value of the flag.
func (i *IPXEHTTP) Set(value string) error {
	u, err := url.Parse(value)
	if err != nil {
		return err
	}
	*i = IPXEHTTP(*u)

	return nil
}

// IPXEScript is a flag.Value for a URL.
type IPXEScript url.URL

// String returns the string representation of the flag.
func (i *IPXEScript) String() string {
	u := url.URL(*i)
	return fmt.Sprintf("%v", u)
}

// Set sets the value of the flag.
func (i *IPXEScript) Set(value string) error {
	u, err := url.Parse(value)
	if err != nil {
		return err
	}
	*i = IPXEScript(*u)
	return nil
}
