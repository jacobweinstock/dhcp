package main

import (
	"fmt"
	"net/url"
	"strings"

	"inet.af/netaddr"
)

type handlers []string

func (h *handlers) String() string {
	n := strings.Join(*h, ",")
	return fmt.Sprintf("%v", n)
}

func (h *handlers) Set(value string) error {
	n := strings.Split(value, ",")
	*h = n
	return nil
}

type dhcpAddr netaddr.IP

func (d *dhcpAddr) String() string {
	n := netaddr.IP(*d)
	if n.IsZero() {
		return ""
	}

	return n.String()
}

func (d *dhcpAddr) Set(value string) error {
	ip, err := netaddr.ParseIP(value)
	if err != nil {
		return err
	}
	*d = dhcpAddr(ip)
	return nil
}

type IPXETFTP netaddr.IPPort

func (i *IPXETFTP) String() string {
	n := netaddr.IPPort(*i)
	return fmt.Sprintf("%v", n)
}

func (i *IPXETFTP) Set(value string) error {
	ipport, err := netaddr.ParseIPPort(value)
	if err != nil {
		return err
	}
	*i = IPXETFTP(ipport)
	return nil
}

type IPXEHTTP url.URL

func (i *IPXEHTTP) String() string {
	u := url.URL(*i)
	return u.String()
}

func (i *IPXEHTTP) Set(value string) error {
	u, err := url.Parse(value)
	if err != nil {
		return err
	}
	*i = IPXEHTTP(*u)

	return nil
}

type IPXEScript url.URL

func (i *IPXEScript) String() string {
	u := url.URL(*i)
	return fmt.Sprintf("%v", u)
}

func (i *IPXEScript) Set(value string) error {
	u, err := url.Parse(value)
	if err != nil {
		return err
	}
	*i = IPXEScript(*u)
	return nil
}
