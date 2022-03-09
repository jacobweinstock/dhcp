package nats

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"testing"
	"time"

	cloudevents "github.com/cloudevents/sdk-go/v2"
	"github.com/google/go-cmp/cmp"
	"github.com/google/uuid"
	"github.com/nats-io/nats-server/v2/server"
	"github.com/nats-io/nats.go"
	"github.com/phayes/freeport"
	"github.com/tinkerbell/dhcp/data"
	"inet.af/netaddr"
)

func TestCreateCloudevent(t *testing.T) {
	type event struct {
		id     string
		source string
		etype  string
		data   DHCPRequest
	}
	tests := map[string]struct {
		mac     net.HardwareAddr
		want    event
		wantErr error
	}{
		"success": {
			mac: net.HardwareAddr{0x00, 0x00, 0x00, 0x00, 0x00, 0x01},
			want: event{
				id:     "f0f7ed74-510a-4a98-ad08-be29d5c0fa68",
				source: "/tinkerbell/dhcp",
				etype:  "org.tinkerbell.dhcp.backend.read",
				data:   DHCPRequest{Mac: net.HardwareAddr{0x00, 0x00, 0x00, 0x00, 0x00, 0x01}},
			},
		},
	}
	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			c := &Config{EConf: EventConf{Source: "/tinkerbell/dhcp", Type: "org.tinkerbell.dhcp.backend.read"}}
			got, err := c.createCloudevent(context.Background(), tt.want.id, tt.mac)
			if !errors.Is(err, tt.wantErr) {
				t.Fatalf("createCloudevent() error (type: %T) = %[1]v, wantErr (type: %T) %[2]v", err, tt.wantErr)
			}
			want := cloudevents.NewEvent()
			want.SetID(tt.want.id)
			want.SetSource(tt.want.source)
			want.SetType(tt.want.etype)
			want.SetData(cloudevents.ApplicationJSON, tt.want.data)
			if diff := cmp.Diff(got, want); diff != "" {
				t.Fatalf(diff)
			}
		})
	}
}

func TestRead(t *testing.T) {
	tests := map[string]struct {
		mac         net.HardwareAddr
		wantErr     error
		wantDHCP    *data.DHCP
		wantNetboot *data.Netboot
	}{
		"failure invalid nats connection": {wantErr: nats.ErrInvalidConnection},
	}
	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			c := &Config{}
			d, n, err := c.Read(context.Background(), tt.mac)
			if !errors.Is(err, tt.wantErr) {
				t.Fatalf("Read() error (type: %T) = %[1]v, wantErr (type: %T) %[2]v", err, tt.wantErr)
			}
			if diff := cmp.Diff(d, tt.wantDHCP); diff != "" {
				t.Fatalf(diff)
			}
			if diff := cmp.Diff(n, tt.wantNetboot); diff != "" {
				t.Fatalf(diff)
			}
		})
	}
}

func startServerResponder(ctx context.Context, sub string, host string, port int) error {
	// server
	s, err := server.NewServer(&server.Options{Debug: true, Port: port, Host: host})
	if err != nil {
		return err
	}
	s.Start()

	// responder
	// Connect to a server
	url := fmt.Sprintf("nats://%v:%d", host, port)
	nc, err := nats.Connect(url)
	if err != nil {
		return err
	}
	defer nc.Close()
	// Replies
	r := &responder{conn: nc}
	subsc, err := nc.Subscribe(sub, r.handle)
	if err != nil {
		return err
	}
	defer subsc.Drain()
	<-ctx.Done()

	return nil
}

type responder struct {
	conn *nats.Conn
}

func (r *responder) handle(m *nats.Msg) {
	cloudevents.WithEncodingStructured(context.Background())
	e := cloudevents.NewEvent()
	err := e.UnmarshalJSON(m.Data)
	if err != nil {
		fmt.Printf("failed to unmarshal received data into cloudevent: %v\n", err)
		return
	}

	rcData := &DHCPRequest{}
	err = json.Unmarshal(e.Data(), rcData)
	if err != nil {
		fmt.Printf("failed to unmarshal received cloudevent.data into sendMsg: %v\n", err)
		return
	}

	resp := &data.DHCP{
		MACAddress:     net.HardwareAddr{0x00, 0x00, 0x00, 0x00, 0x00, 0x01},
		IPAddress:      netaddr.IPv4(192, 168, 2, 199),
		SubnetMask:     net.IPMask(net.ParseIP("255.255.255.0").To4()),
		DefaultGateway: netaddr.IPv4(192, 168, 2, 1),
		NameServers: []net.IP{
			net.ParseIP("1.1.1.1"),
			net.ParseIP("8.8.8.8"),
		},
		Hostname:         "pxe-virtualbox",
		BroadcastAddress: netaddr.IPv4(192, 168, 2, 255),
		LeaseTime:        86400,
		// Traceparent:      traceparent, // tsstring, // "00-deadbeefcafedeadbeefcafedeadbeef-123456789abcdef0-01",
	}
	ceResp := cloudevents.NewEvent()
	ceResp.SetID(uuid.New().String())
	ceResp.SetSource("/tinkerbell/dhcp")
	ceResp.SetType("org.tinkerbell.backend.read")
	err = ceResp.SetData(cloudevents.ApplicationJSON, resp)
	if err != nil {
		fmt.Println(err)
		return
	}
	b, err := ceResp.MarshalJSON()
	if err != nil {
		fmt.Printf("failed to json marshal cloudevent: %v\n", err)

		return
	}
	err = r.conn.Publish(m.Reply, b)
	if err != nil {
		fmt.Println(err)
		return
	}
}

func getPort() int {
	port, err := freeport.GetFreePort()
	if err != nil {
		return 0
	}
	return port
}

func TestServer(t *testing.T) {
	tests := map[string]struct {
		want   *data.DHCP
		wantNB *data.Netboot
	}{
		"success": {
			want: &data.DHCP{
				MACAddress:       []byte{0x00, 0x00, 0x00, 0x00, 0x00, 0x01},
				IPAddress:        netaddr.IPv4(192, 168, 2, 199),
				SubnetMask:       []byte{0xff, 0xff, 0xff, 0x00},
				DefaultGateway:   netaddr.IPv4(192, 168, 2, 1),
				NameServers:      []net.IP{{0x01, 0x01, 0x01, 0x01}, {0x08, 0x08, 0x08, 0x08}},
				Hostname:         "pxe-virtualbox",
				BroadcastAddress: netaddr.IPv4(192, 168, 2, 255),
				LeaseTime:        86400,
			},
			wantNB: &data.Netboot{AllowNetboot: true},
		},
	}
	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			port := getPort()
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()

			go startServerResponder(ctx, "dhcp", "127.0.0.1", port)
			time.Sleep(time.Millisecond * 100)

			url := fmt.Sprintf("nats://127.0.0.1:%d", port)
			nc, err := nats.Connect(url)
			if err != nil {
				t.Fatal(err)
			}
			defer nc.Close()

			c := &Config{Conn: nc, Subject: "dhcp", Timeout: time.Second * 5, EConf: EventConf{
				Source: "/tinkerbell/dhcp",
				Type:   "org.tinkerbell.backend.read",
			}}
			d, n, err := c.Read(ctx, net.HardwareAddr{0x00, 0x00, 0x00, 0x00, 0x00, 0x01})
			if !errors.Is(err, nil) {
				t.Fatalf("Read() error (type: %T) = %[1]v, wantErr (type: %T) %[2]v", err, nil)
			}

			if diff := cmp.Diff(d, tt.want, netaddrComparer); diff != "" {
				t.Fatalf(diff)
			}
			if diff := cmp.Diff(n, tt.wantNB); diff != "" {
				t.Fatalf(diff)
			}
		},
		)
	}
}

var netaddrComparer = cmp.Comparer(func(x, y netaddr.IP) bool {
	return x.Compare(y) == 0
})
