package nats

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net"

	natscloudevents "github.com/cloudevents/sdk-go/protocol/nats/v2"
	cloudevents "github.com/cloudevents/sdk-go/v2"
	"github.com/nats-io/nats.go"
	"github.com/tinkerbell/dhcp/data"
	"inet.af/netaddr"
)

type Conn struct{}

type msg struct {
	IPAddress        string   `json:"ip_address"`
	SubnetMask       string   `json:"subnet_mask"`
	DefaultGateway   string   `json:"default_gateway"`
	NameServers      []string `json:"name_servers"`
	Hostname         string   `json:"hostname"`
	DomainName       string   `json:"domain_name"`
	BroadcastAddress string   `json:"broadcast_address"`
	NTPServers       []string `json:"ntp_servers"`
	LeaseTime        uint32   `json:"lease_time"`
	DomainSearch     []string `json:"domain_search"`
}

func (c *Conn) Read(ctx context.Context, mac net.HardwareAddr) (*data.Dhcp, *data.Netboot, error) {
	// Connect to a server
	nc, err := nats.Connect(nats.DefaultURL)
	if err != nil {
		fmt.Println("1")
		return nil, nil, err
	}
	defer nc.Close()
	err = eventSend(ctx, nc)
	if err != nil {
		fmt.Println(err)
	}
	ms, err := nc.RequestWithContext(ctx, "dhcp", mac)
	if err != nil {
		fmt.Println("2")
		return nil, nil, err
	}
	// unmarshal the message
	m := &msg{}
	err = json.Unmarshal(ms.Data, m)
	if err != nil {
		fmt.Println("3")
		return nil, nil, err
	}
	fmt.Printf("Read: %+v\n", m)

	return translate(m), &data.Netboot{}, nil
}

func eventSend(ctx context.Context, nc *nats.Conn) error {
	p, err := natscloudevents.NewSenderFromConn(nc, "dhcp")
	//p, err := natscloudevents.NewProtocolFromConn(nc, "dhcp", "dhcp.event")
	if err != nil {
		return err
	}
	c, err := cloudevents.NewClient(p)
	if err != nil {
		log.Fatalf("failed to create client, %v", err)
	}

	// Create an Event.
	event := cloudevents.NewEvent()
	event.SetID("12345")
	event.SetSource("example/uri")
	event.SetType("example.type")
	event.SetData(cloudevents.ApplicationJSON, map[string]string{"hello": "world"})

	// Set a target.
	//ctx = cloudevents.ContextWithTarget(ctx, nats.DefaultURL)

	// Send that Event.
	if result := c.Send(ctx, event); cloudevents.IsUndelivered(result) {
		return fmt.Errorf("failed to send, %v", result)
	}
	fmt.Printf("eventSend: %+v\n", event)
	return nil
}

func translate(m *msg) *data.Dhcp {
	ip, err := netaddr.ParseIP(m.IPAddress)
	if err != nil {
		return nil
	}
	return &data.Dhcp{
		IPAddress:  ip,
		SubnetMask: []byte(m.SubnetMask),
		LeaseTime:  m.LeaseTime,
	}
}
