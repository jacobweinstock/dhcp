package option

import (
	"net"
	"strings"

	"github.com/insomniacslk/dhcp/dhcpv4"
)

// SetSNAME based on option 60.
func SetHeaderSNAME(opt60FromClient string, tftp net.IP, http net.IP) dhcpv4.Modifier {
	sname := tftp
	if strings.HasPrefix(opt60FromClient, string(HTTPClient)) {
		sname = http
	}

	return func(d *dhcpv4.DHCPv4) {
		d.ServerHostName = sname.String()
	}
}
