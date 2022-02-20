package main

import (
	"context"
	"fmt"
	"log"
	"net"
	"net/url"
	"os"
	"os/signal"
	"syscall"

	natscloudevents "github.com/cloudevents/sdk-go/protocol/nats/v2"
	cloudevents "github.com/cloudevents/sdk-go/v2"
	"github.com/go-logr/stdr"
	natsio "github.com/nats-io/nats.go"
	"github.com/tinkerbell/dhcp"
	"github.com/tinkerbell/dhcp/backend/nats"
	"github.com/tinkerbell/dhcp/data"
	"inet.af/netaddr"
)

func main() {
	ctx, done := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGHUP, syscall.SIGTERM)
	defer done()
	go responder(ctx)
	b := &nats.Conn{}
	s := &dhcp.Server{
		Log:               stdr.New(log.New(os.Stdout, "", 0)),
		Listener:          netaddr.IPPortFrom(netaddr.IPv4(192, 168, 1, 225), 67),
		IPAddr:            netaddr.IPv4(192, 168, 1, 225),
		IPXEBinServerTFTP: netaddr.IPPortFrom(netaddr.IPv4(192, 168, 1, 225), 69),
		//IPXEBinServerHTTP: &url.URL{},
		IPXEScriptURL:  &url.URL{Scheme: "http", Host: "boot.netboot.xyz"},
		NetbootEnabled: true,
		Backend:        b, // &thing{},
	}
	s.ListenAndServe(ctx)
}

type thing struct{}

func (a *thing) Read(context.Context, net.HardwareAddr) (*data.Dhcp, *data.Netboot, error) {
	d := &data.Dhcp{
		IPAddress:      netaddr.IPv4(192, 168, 1, 200),
		SubnetMask:     []byte{255, 255, 255, 0},
		DefaultGateway: netaddr.IPv4(192, 168, 1, 1),
		NameServers: []net.IP{
			{1, 1, 1, 1},
		},
		LeaseTime: 86400,
	}
	n := &data.Netboot{
		AllowNetboot: true,
	}
	return d, n, nil
}

func responder(ctx context.Context) {
	// Connect to a server
	nc, err := natsio.Connect(natsio.DefaultURL)
	if err != nil {
		fmt.Println("1")
		return
	}
	defer nc.Close()
	go cloudevts(ctx, nc)
	// Replies
	nc.Subscribe("dhcp", func(m *natsio.Msg) {
		nc.Publish(m.Reply, []byte(`{"ip_address": "192.168.1.199", "subnet_mask": "255.255.255.0", "lease_time": 86400}`))
	})
	<-ctx.Done()
}

func cloudevts(ctx context.Context, nc *natsio.Conn) {
	p, err := natscloudevents.NewConsumerFromConn(nc, "dhcp")
	//p, err := natscloudevents.NewProtocolFromConn(nc, "dhcp", "dhcp.event")
	if err != nil {
		fmt.Println(err)
		return
	}
	c, err := cloudevents.NewClient(p)
	if err != nil {
		fmt.Printf("cloudevts: failed to create client, %v\n", err)
		return
	}
	fmt.Println("cloudevts: StartReceiver")
	log.Println(c.StartReceiver(ctx, receive))
	fmt.Println("done")
	<-ctx.Done()
}

func receive(event cloudevents.Event) {
	// do something with event.
	fmt.Printf("cloudevent: %s\n", event)
}
