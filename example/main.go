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
	"github.com/equinix-labs/otel-init-go/otelhelpers"
	"github.com/equinix-labs/otel-init-go/otelinit"
	"github.com/go-logr/stdr"
	"github.com/google/uuid"
	"github.com/nats-io/nats-server/v2/server"
	natsio "github.com/nats-io/nats.go"
	"github.com/tinkerbell/dhcp"
	"github.com/tinkerbell/dhcp/backend/nats"
	"github.com/tinkerbell/dhcp/data"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"
	"inet.af/netaddr"
)

const natsSubject = "tinkerbell.dhcp"
const tracerName = "github.com/packethost/cacher"

func main() {
	ctx, done := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGHUP, syscall.SIGTERM)
	defer done()

	runResponder := flag.Bool("responder", false, "only run the responder")
	runServer := flag.Bool("server", false, "only run the nats server")
	flag.Parse()
	if *runResponder {
		ctx, otelShutdown := otelinit.InitOpenTelemetry(ctx, "github.com/tinkerbell/cacher")
		defer otelShutdown(ctx)
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
	defer b.Conn.Drain() // nolint: errcheck // just a basic example
	ctx, otelShutdown := otelinit.InitOpenTelemetry(ctx, "github.com/tinkerbell/dhcp")
	defer otelShutdown(ctx)
	s := &dhcp.Server{
		Log:               stdr.New(log.New(os.Stdout, "", 0)),
		Listener:          netaddr.IPPortFrom(netaddr.IPv4(192, 168, 2, 225), 67),
		IPAddr:            netaddr.IPv4(192, 168, 2, 225),
		IPXEBinServerTFTP: netaddr.IPPortFrom(netaddr.IPv4(192, 168, 2, 225), 69),
		// IPXEBinServerHTTP: &url.URL{},
		IPXEScriptURL:  &url.URL{Scheme: "http", Host: "boot.netboot.xyz"},
		NetbootEnabled: true,
		Backend:        b,
		OTELEnabled:    true,
	}
	log.Println(s.ListenAndServe(ctx))
}

func setupNats(u string) (*nats.Conn, error) {
	nc, err := natsio.Connect(u)
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
	subsc, err := nc.Subscribe(sub, func(m *natsio.Msg) {

		/*
			c := otelhelpers.ContextWithTraceparentString(ctx, "00-deadbeefcafedeadbeefcafedeadbeef-123456789abcdef0-01")
			sp := span.SpanContext()
			tID := sp.TraceID().String()
			sID := sp.SpanID().String()
			fID := sp.TraceFlags().String()
			tp := fmt.Sprintf("00-%v-%v-%v", tID, sID, fID)
		*/

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

		///

		provisionTP := "00-0af7651916cd43dd8448eb211c80319c-b7ad6b7169203331-01"
		ctxWithProvTP := otelhelpers.ContextWithTraceparentString(context.Background(), provisionTP)
		ctxWithDHCPTP := otelhelpers.ContextWithTraceparentString(context.Background(), rcData.Traceparent)

		links := trace.WithLinks(trace.LinkFromContext(ctxWithProvTP), trace.LinkFromContext(ctxWithDHCPTP))

		// span connected to provision trace
		tracerOne := otel.Tracer(tracerName)
		_, spanOne := tracerOne.Start(ctxWithProvTP, "Span Link to DHCP Trace", links)
		defer spanOne.End()

		// span connected to the DHCP trace
		tracerTwo := otel.Tracer(tracerName)
		_, spanTwo := tracerTwo.Start(ctxWithDHCPTP, "Hardware data response", links)
		defer spanTwo.End()

		fmt.Printf("rcData: %+v\n", rcData)
		fmt.Printf("e: %+v\n", e)
		spanTwo.SetAttributes(attribute.String("mac", rcData.Mac.String()))

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
			// Traceparent:      traceparent, // tsstring, // "00-deadbeefcafedeadbeefcafedeadbeef-123456789abcdef0-01",
		}
		ceResp := cloudevents.NewEvent()
		ceResp.SetID(uuid.New().String())
		ceResp.SetSource("/tinkerbell/dhcp")
		ceResp.SetType("org.tinkerbell.backend.read")
		err = ceResp.SetData(cloudevents.ApplicationJSON, resp)
		if err != nil {
			spanTwo.SetStatus(codes.Error, err.Error())
			spanOne.SetStatus(codes.Error, err.Error())
			log.Fatal(err)
		}
		b, err := ceResp.MarshalJSON()
		if err != nil {
			fmt.Printf("failed to json marshal cloudevent: %v\n", err)
			spanTwo.SetStatus(codes.Error, err.Error())
			spanOne.SetStatus(codes.Error, err.Error())
			return
		}
		err = nc.Publish(m.Reply, b)
		if err != nil {
			log.Println(err)
		}
		spanTwo.SetStatus(codes.Ok, "sent BACKEND response")
		spanOne.SetStatus(codes.Ok, "sent BACKEND response")
	})
	if err != nil {
		log.Println(err)
		return
	}
	defer subsc.Drain() // nolint: errcheck // just a basic example
	<-ctx.Done()
}
