package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"net"
	"net/url"
	"os"
	"os/signal"
	"syscall"
	"time"

	cloudevents "github.com/cloudevents/sdk-go/v2"
	"github.com/go-logr/stdr"
	"github.com/google/uuid"
	"github.com/nats-io/nats-server/v2/server"
	natsio "github.com/nats-io/nats.go"
	"github.com/tinkerbell/dhcp"
	"github.com/tinkerbell/dhcp/backend/nats"
	"github.com/tinkerbell/dhcp/data"
	"inet.af/netaddr"
)

const natsSubject = "tinkerbell.dhcp"

func main() {
	ctx, done := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGHUP, syscall.SIGTERM)
	defer done()
	runResponder := flag.Bool("responder", false, "only run the responder")
	runServer := flag.Bool("server", false, "only run the nats server")
	flag.Parse()
	if *runResponder {
		fmt.Println("starting responder")
		responder(ctx, natsSubject)
		fmt.Println("responder shutdown")
		return
	}
	if *runServer {
		s, err := server.NewServer(&server.Options{Debug: true, JetStream: true})
		if err != nil {
			panic(err)
		}
		go func() {
			<-ctx.Done()
			s.Shutdown()
			fmt.Println("server shutdown")
		}()
		s.ConfigureLogger()
		fmt.Println("starting server")
		s.Start()
		<-ctx.Done()
		return
	}

	b, err := setupNats(natsio.DefaultURL)
	if err != nil {
		panic(err)
	}
	defer b.Conn.Drain()
	s := &dhcp.Server{
		Log:               stdr.New(log.New(os.Stdout, "", 0)),
		Listener:          netaddr.IPPortFrom(netaddr.IPv4(192, 168, 2, 225), 67),
		IPAddr:            netaddr.IPv4(192, 168, 2, 225),
		IPXEBinServerTFTP: netaddr.IPPortFrom(netaddr.IPv4(192, 168, 2, 225), 69),
		//IPXEBinServerHTTP: &url.URL{},
		IPXEScriptURL:  &url.URL{Scheme: "http", Host: "boot.netboot.xyz"},
		NetbootEnabled: true,
		Backend:        b, // &thing{},
	}
	s.ListenAndServe(ctx)
}

type thing struct{}

func (a *thing) Read(context.Context, net.HardwareAddr) (*data.DHCP, *data.Netboot, error) {
	d := &data.DHCP{
		IPAddress:      netaddr.IPv4(192, 168, 2, 200),
		SubnetMask:     []byte{255, 255, 255, 0},
		DefaultGateway: netaddr.IPv4(192, 168, 2, 1),
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

func setupNats(url string) (*nats.Conn, error) {
	nc, err := natsio.Connect(url)
	if err != nil {
		return nil, err
	}
	return &nats.Conn{
		Conn:    nc,
		Timeout: time.Second * 5,
		Subject: natsSubject,
	}, nil
}

func responder(ctx context.Context, sub string) {
	// Connect to a server
	nc, err := natsio.Connect(natsio.DefaultURL)
	if err != nil {
		fmt.Println("1")
		return
	}
	defer nc.Close()
	// Replies
	nc.Subscribe(sub, func(m *natsio.Msg) {
		e := cloudevents.NewEvent()
		err := e.UnmarshalJSON(m.Data)
		if err != nil {
			fmt.Printf("failed to unmarshal received data into cloudevent: %v\n", err)
			return
		}

		rcData := &nats.DHCPRequest{}
		err = json.Unmarshal(e.Data(), rcData)
		if err != nil {
			fmt.Printf("failed to unmarshal received cloudevent.data into sendMsg: %v\n", err)
			return
		}
		fmt.Printf("%+v\n", rcData)
		fmt.Printf("%+v\n", e)

		// make call to tink server then populate a response.

		resp := &data.DHCP{
			IPAddress:      netaddr.IPv4(192, 168, 2, 199),
			SubnetMask:     net.IPMask(net.ParseIP("192.168.2.1").To4()),
			DefaultGateway: netaddr.IPv4(192, 168, 2, 1),
			NameServers: []net.IP{
				net.ParseIP("1.1.1.1"),
				net.ParseIP("8.8.8.8"),
			},
			Hostname:         "pxe-virtualbox",
			BroadcastAddress: netaddr.IPv4(192, 168, 2, 255),
			LeaseTime:        86400,
		}
		/*b, err := json.Marshal(resp)
		if err != nil {
			fmt.Println("error with json.Marshall()", err)
			return
		}*/
		ceResp := cloudevents.NewEvent()
		ceResp.SetID(uuid.New().String())
		ceResp.SetSource("/tinkerbell/dhcp")
		ceResp.SetType("org.tinkerbell.backend.read")
		ceResp.SetData(cloudevents.ApplicationJSON, resp)
		b, err := ceResp.MarshalJSON()
		if err != nil {
			fmt.Printf("failed to json marshal cloudevent: %v\n", err)
			return
		}
		nc.Publish(m.Reply, b)
	})
	<-ctx.Done()
}
