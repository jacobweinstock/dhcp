package proxy

import (
	"testing"
	"time"

	"github.com/libp2p/go-reuseport"
	"github.com/tinkerbell/dhcp"
)

func TestXxx(t *testing.T) {
	ls, err := reuseport.ListenPacket("udp", ":67")
	if err != nil {
		t.Fatal(err)
	}

	l := &dhcp.Listener{}
	go l.Serve(ls)

	ls2, err := reuseport.ListenPacket("udp", ":67")
	if err != nil {
		t.Fatal(err)
	}
	l2 := &dhcp.Listener{}
	go l2.Serve(ls2)

	time.Sleep(time.Second * 3)
	l.Shutdown()
	l2.Shutdown()
	t.Fatal()
}
