// Package nats implements a backend for communicating via a Nats server.
package nats

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"time"

	cloudevents "github.com/cloudevents/sdk-go/v2"
	"github.com/equinix-labs/otel-init-go/otelhelpers"
	"github.com/google/uuid"
	"github.com/nats-io/nats.go"
	"github.com/tinkerbell/dhcp/data"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/codes"
)

const tracerName = "github.com/tinkerbell/dhcp"

// Conn holds details about communicating with a Nats server.
type Conn struct {
	Subject string
	Timeout time.Duration
	Conn    *nats.Conn
}

// DHCPRequest is the data passed to listening backends.
type DHCPRequest struct {
	Mac         net.HardwareAddr `json:"MacAddress"`
	Traceparent string           `json:"Traceparent"`
}

// Read implements the interface for getting data via a nats messaging request/reply pattern.
func (c *Conn) Read(ctx context.Context, mac net.HardwareAddr) (*data.DHCP, *data.Netboot, error) {
	tracer := otel.Tracer(tracerName)
	_, span := tracer.Start(ctx, "backend.nats.Read")
	defer span.End()

	event := cloudevents.NewEvent()
	event.SetID(uuid.New().String())
	event.SetSource("/tinkerbell/dhcp")
	event.SetType("org.tinkerbell.backend.read")
	err := event.SetData(cloudevents.ApplicationJSON, &DHCPRequest{Mac: mac, Traceparent: otelhelpers.TraceparentStringFromContext(ctx)})
	if err != nil {
		span.SetStatus(codes.Error, err.Error())

		return nil, nil, fmt.Errorf("failed to set cloudevents data")
	}
	b, err := event.MarshalJSON()
	if err != nil {
		span.SetStatus(codes.Error, err.Error())

		return nil, nil, fmt.Errorf("failed to marshal cloudevent into json: %w", err)
	}
	ctx, cancel := context.WithTimeout(ctx, c.Timeout)
	defer cancel()
	ctx = cloudevents.WithEncodingStructured(ctx)
	// do request/reply
	ms, err := c.Conn.RequestWithContext(ctx, c.Subject, b)
	if err != nil {
		span.SetStatus(codes.Error, err.Error())

		return nil, nil, fmt.Errorf("failed to get reply: %w", err)
	}

	// unmarshal the cloudevent
	reply := cloudevents.NewEvent()
	err = reply.UnmarshalJSON(ms.Data)
	if err != nil {
		span.SetStatus(codes.Error, err.Error())

		return nil, nil, fmt.Errorf("failed to unmarshal cloudevent: %w", err)
	}

	// unmarshal the response data
	d := &data.DHCP{}
	err = json.Unmarshal(reply.Data(), d)
	if err != nil {
		span.SetStatus(codes.Error, err.Error())

		return nil, nil, fmt.Errorf("failed to unmarshal received cloudevent.data into msg: %w", err)
	}
	n := &data.Netboot{AllowNetboot: true}
	span.SetAttributes(d.EncodeToAttributes()...)
	span.SetAttributes(n.EncodeToAttributes()...)
	span.SetStatus(codes.Ok, "")

	return d, n, nil
}
