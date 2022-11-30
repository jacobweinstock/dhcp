package proxy

import (
	"context"
	"testing"
	"time"

	"github.com/libp2p/go-reuseport"
	"github.com/tinkerbell/dhcp"
)

func TestXxx(t *testing.T) {
	t.Skip()
	ctx, done := context.WithTimeout(context.Background(), time.Millisecond*100)
	defer done()
	ls, err := reuseport.ListenPacket("udp", ":67")
	if err != nil {
		t.Fatal(err)
	}

	l := &dhcp.Listener{}
	go l.Serve(ctx, ls)

	ls2, err := reuseport.ListenPacket("udp", ":67")
	if err != nil {
		t.Fatal(err)
	}
	l2 := &dhcp.Listener{}
	go l2.Serve(ctx, ls2)

	time.Sleep(time.Second * 3)

	t.Fatal()
}
