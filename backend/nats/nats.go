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

// Config holds details about communicating with a Nats server.
type Config struct {
	Subject string
	Timeout time.Duration
	Conn    *nats.Conn
	EConf   EventConf
}

// EventConf TODO(jacobweinstock): add comment.
type EventConf struct {
	Source string
	Type   string
}

// DHCPRequest is the data passed to used in the request to the nats subject.
type DHCPRequest struct {
	Mac         net.HardwareAddr `json:"MacAddress"`
	Traceparent string           `json:"Traceparent"`
}

// Read implements the interface for getting data via a nats messaging request/reply pattern.
func (c *Config) Read(ctx context.Context, mac net.HardwareAddr) (*data.DHCP, *data.Netboot, error) {
	// TODO(jacobweinstock): validate EventConf
	// TODO(jacobweinstock): validate Conn.Subject, Conn.Timeout, Conn.Conn
	tracer := otel.Tracer(tracerName)
	ctx, span := tracer.Start(ctx, "backend.nats.Read")
	defer span.End()

	ctx = cloudevents.WithEncodingStructured(ctx)
	event, err := c.createCloudevent(ctx, uuid.New().String(), mac)
	if err != nil {
		return nil, nil, err
	}

	// do request/reply
	reqCTX, cancel := context.WithTimeout(ctx, c.Timeout)
	defer cancel()
	b, err := event.MarshalJSON()
	if err != nil {
		return nil, nil, fmt.Errorf("failed to marshal cloudevent into json: %w", err)
	}
	ms, err := c.Conn.RequestWithContext(reqCTX, c.Subject, b)
	if err != nil {
		span.SetStatus(codes.Error, err.Error())

		return nil, nil, fmt.Errorf("failed to get reply: %w", err)
	}

	// unmarshal the reply's cloudevent
	reply := cloudevents.NewEvent()
	err = reply.UnmarshalJSON(ms.Data)
	if err != nil {
		span.SetStatus(codes.Error, err.Error())

		return nil, nil, fmt.Errorf("failed to unmarshal cloudevent: %w", err)
	}

	// unmarshal the reply data
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

func (c *Config) createCloudevent(ctx context.Context, id string, mac net.HardwareAddr) (cloudevents.Event, error) {
	event := cloudevents.NewEvent()
	event.SetID(id)
	event.SetSource(c.EConf.Source)
	event.SetType(c.EConf.Type)

	err := event.SetData(cloudevents.ApplicationJSON, &DHCPRequest{Mac: mac, Traceparent: otelhelpers.TraceparentStringFromContext(ctx)})
	if err != nil {
		return cloudevents.Event{}, fmt.Errorf("failed to set cloudevents data")
	}

	return event, nil
}
