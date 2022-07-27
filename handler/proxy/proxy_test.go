package proxy

import (
	"testing"

	"github.com/insomniacslk/dhcp/dhcpv4/server4"
)

func TestXxx(t *testing.T) {
	udp, err := server4.NewIPv4UDPConn("en0", nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("%+v", udp)

	udp, err = server4.NewIPv4UDPConn("en1", nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("%+v", udp)

	t.Fatal()
}
