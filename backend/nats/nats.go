// Package nats implements a backend for communicating via a Nats server.
package nats

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"reflect"
	"time"

	cloudevents "github.com/cloudevents/sdk-go/v2"
	"github.com/equinix-labs/otel-init-go/otelhelpers"
	"github.com/go-logr/logr"
	"github.com/google/uuid"
	"github.com/imdario/mergo"
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
	Log     logr.Logger
}

// EventConf TODO(jacobweinstock): add comment.
type EventConf struct {
	Source string
	Type   string
}

// DHCPRequest is the data passed to used in the request to the nats subject.
type DHCPRequest struct {
	Mac         net.HardwareAddr `json:"MACAddress"`
	Traceparent string           `json:"Traceparent"`
}

// Read implements the interface for getting data via a nats messaging request/reply pattern.
func (c *Config) Read(ctx context.Context, mac net.HardwareAddr) (*data.DHCP, *data.Netboot, error) {
	tracer := otel.Tracer(tracerName)
	ctx, span := tracer.Start(ctx, "DHCP.backend.nats.Read")
	defer span.End()

	defaults := &Config{
		Log:     logr.Discard(),
		Timeout: time.Second * 5,
		Subject: "dhcp",
		EConf: EventConf{
			Source: "/tinkerbell/dhcp",
			Type:   "org.tinkerbell.dhcp.backend.nats.read",
		},
	}

	err := mergo.Merge(c, defaults, mergo.WithTransformers(c))
	if err != nil {
		span.SetStatus(codes.Error, err.Error())

		return nil, nil, err
	}
	c.Log.V(1).Info("nats.Read", "config", c)

	// ctx = cloudevents.WithEncodingStructured(ctx)
	event, err := c.createCloudevent(ctx, uuid.New().String(), mac)
	if err != nil {
		span.SetStatus(codes.Error, err.Error())

		return nil, nil, err
	}
	ctx = cloudevents.WithEncodingStructured(ctx)

	// do request/reply
	reqCTX, cancel := context.WithTimeout(ctx, c.Timeout)
	defer cancel()
	b, err := event.MarshalJSON()
	if err != nil {
		span.SetStatus(codes.Error, err.Error())

		return nil, nil, fmt.Errorf("failed to marshal cloudevent into json: %w", err)
	}

	// response

	ms, err := c.Conn.RequestWithContext(reqCTX, c.Subject, b)
	if err != nil {
		span.SetStatus(codes.Error, err.Error())

		return nil, nil, fmt.Errorf("failed to get reply: %w", err)
	}

	reply := cloudevents.NewEvent()
	// unmarshal the reply's cloudevent
	err = reply.UnmarshalJSON(ms.Data)
	if err != nil {
		span.SetStatus(codes.Error, err.Error())

		return nil, nil, fmt.Errorf("failed to unmarshal cloudevent: %w", err)
	}

	// unmarshal the reply data
	d := &data.Message{}
	err = json.Unmarshal(reply.Data(), d)
	if err != nil {
		span.SetStatus(codes.Error, err.Error())

		return nil, nil, fmt.Errorf("failed to unmarshal received cloudevent.data into msg: %w", err)
	}
	if d.Error.Message != "" {
		span.SetStatus(codes.Error, d.Error.Error())

		return nil, nil, fmt.Errorf("no record from backend: %w", &d.Error)
	}
	n := &data.Netboot{AllowNetboot: true}
	span.SetAttributes(d.DHCP.EncodeToAttributes()...)
	span.SetAttributes(n.EncodeToAttributes()...)
	span.SetStatus(codes.Ok, "")

	return &d.DHCP, &d.Netboot, nil
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

// Transformer for merging the netaddr.IPPort and logr.Logger structs.
func (c *Config) Transformer(typ reflect.Type) func(dst, src reflect.Value) error {
	switch typ {
	case reflect.TypeOf(logr.Logger{}):
		return func(dst, src reflect.Value) error {
			if dst.CanSet() {
				isZero := dst.MethodByName("GetSink")
				result := isZero.Call(nil)
				if result[0].IsNil() {
					dst.Set(src)
				}
			}
			return nil
		}
	}
	return nil
}
