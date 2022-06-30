// Package otel handles translating DHCP headers and options to otel key/value attributes.
package otel

import (
	"context"
	"fmt"
	"net"
	"strings"

	"github.com/go-logr/logr"
	"github.com/insomniacslk/dhcp/dhcpv4"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"
)

const keyNamespace = "DHCP"

type notFoundError struct {
	optName string
}

func (e *notFoundError) Error() string {
	return fmt.Sprintf("%q not found in DHCP packet", e.optName)
}

func (e *notFoundError) found() bool {
	return true
}

// OptNotFound returns true if err is an option not found error.
func OptNotFound(err error) bool {
	type found interface {
		found() bool
	}
	te, ok := err.(found)
	return ok && te.found()
}

// Encoder translates a DHCP header/option to an OTEL key/value pair.
// namespace is added to the OTEL key name.
type Encoder func(d *dhcpv4.DHCPv4, namespace string) (attribute.KeyValue, error)

// Encode runs a slice of encoders against a DHCPv4 packet turning the values into opentelemetry attribute key/value pairs.
func Encode(l logr.Logger, pkt *dhcpv4.DHCPv4, namespace string, encoders ...Encoder) []attribute.KeyValue {
	var attrs []attribute.KeyValue
	for _, elem := range encoders {
		kv, err := elem(pkt, namespace)
		if err != nil {
			l.V(1).Info("opentelemetry attribute not added", "error", fmt.Sprintf("%v", err))
			continue
		}
		attrs = append(attrs, kv)
	}

	return attrs
}

// AllEncoders returns a slice of all available DHCP otel encoders.
func AllEncoders() []Encoder {
	return []Encoder{
		EncodeYIADDR, EncodeSIADDR,
		EncodeCHADDR, EncodeFILE,
		EncodeOpt1, EncodeOpt3, EncodeOpt6,
		EncodeOpt12, EncodeOpt15, EncodeOpt28,
		EncodeOpt42, EncodeOpt51, EncodeOpt53,
		EncodeOpt54, EncodeOpt119,
	}
}

// EncodeOpt1 takes DHCP Opt 1 from a DHCP packet and returns an OTEL key/value pair.
//
// See https://www.iana.org/assignments/bootp-dhcp-parameters/bootp-dhcp-parameters.xhtml
func EncodeOpt1(d *dhcpv4.DHCPv4, namespace string) (attribute.KeyValue, error) {
	opt := "Opt1.SubnetMask"
	key := fmt.Sprintf("%v.%v.%v", keyNamespace, namespace, opt)
	if d != nil && d.SubnetMask() != nil {
		sm := net.IP(d.SubnetMask()).String()
		return attribute.String(key, sm), nil
	}

	return attribute.KeyValue{}, &notFoundError{optName: opt}
}

// EncodeOpt3 takes DHCP Opt 3 from a DHCP packet and returns an OTEL key/value pair.
//
// See https://www.iana.org/assignments/bootp-dhcp-parameters/bootp-dhcp-parameters.xhtml
func EncodeOpt3(d *dhcpv4.DHCPv4, namespace string) (attribute.KeyValue, error) {
	key := fmt.Sprintf("%v.%v.Opt3.DefaultGateway", keyNamespace, namespace)
	if d != nil {
		var routers []string
		for _, e := range d.Router() {
			routers = append(routers, e.String())
		}
		if len(routers) > 0 {
			return attribute.String(key, strings.Join(routers, ",")), nil
		}
	}

	return attribute.KeyValue{}, &notFoundError{optName: key}
}

// EncodeOpt6 takes DHCP Opt 6 from a DHCP packet and returns an OTEL key/value pair.
//
// See https://www.iana.org/assignments/bootp-dhcp-parameters/bootp-dhcp-parameters.xhtml
func EncodeOpt6(d *dhcpv4.DHCPv4, namespace string) (attribute.KeyValue, error) {
	key := fmt.Sprintf("%v.%v.Opt6.NameServers", keyNamespace, namespace)
	if d != nil {
		var ns []string
		for _, e := range d.DNS() {
			ns = append(ns, e.String())
		}
		if len(ns) > 0 {
			return attribute.String(key, strings.Join(ns, ",")), nil
		}
	}

	return attribute.KeyValue{}, &notFoundError{optName: key}
}

// EncodeOpt12 takes DHCP Opt 12 from a DHCP packet and returns an OTEL key/value pair.
//
// See https://www.iana.org/assignments/bootp-dhcp-parameters/bootp-dhcp-parameters.xhtml
func EncodeOpt12(d *dhcpv4.DHCPv4, namespace string) (attribute.KeyValue, error) {
	key := fmt.Sprintf("%v.%v.Opt12.Hostname", keyNamespace, namespace)
	if d != nil && d.HostName() != "" {
		return attribute.String(key, d.HostName()), nil
	}

	return attribute.KeyValue{}, &notFoundError{optName: key}
}

// EncodeOpt15 takes DHCP Opt 15 from a DHCP packet and returns an OTEL key/value pair.
//
// See https://www.iana.org/assignments/bootp-dhcp-parameters/bootp-dhcp-parameters.xhtml
func EncodeOpt15(d *dhcpv4.DHCPv4, namespace string) (attribute.KeyValue, error) {
	key := fmt.Sprintf("%v.%v.Opt15.DomainName", keyNamespace, namespace)
	if d != nil && d.DomainName() != "" {
		return attribute.String(key, d.DomainName()), nil
	}

	return attribute.KeyValue{}, &notFoundError{optName: key}
}

// EncodeOpt28 takes DHCP Opt 28 from a DHCP packet and returns an OTEL key/value pair.
//
// See https://www.iana.org/assignments/bootp-dhcp-parameters/bootp-dhcp-parameters.xhtml
func EncodeOpt28(d *dhcpv4.DHCPv4, namespace string) (attribute.KeyValue, error) {
	key := fmt.Sprintf("%v.%v.Opt28.BroadcastAddress", keyNamespace, namespace)
	if d != nil && d.BroadcastAddress() != nil {
		return attribute.String(key, d.BroadcastAddress().String()), nil
	}

	return attribute.KeyValue{}, &notFoundError{optName: key}
}

// EncodeOpt42 takes DHCP Opt 42 from a DHCP packet and returns an OTEL key/value pair.
//
// See https://www.iana.org/assignments/bootp-dhcp-parameters/bootp-dhcp-parameters.xhtml
func EncodeOpt42(d *dhcpv4.DHCPv4, namespace string) (attribute.KeyValue, error) {
	key := fmt.Sprintf("%v.%v.Opt42.NTPServers", keyNamespace, namespace)
	if d != nil {
		var ntp []string
		for _, e := range d.NTPServers() {
			ntp = append(ntp, e.String())
		}
		if len(ntp) > 0 {
			return attribute.String(key, strings.Join(ntp, ",")), nil
		}
	}

	return attribute.KeyValue{}, &notFoundError{optName: key}
}

// EncodeOpt51 takes DHCP Opt 51 from a DHCP packet and returns an OTEL key/value pair.
//
// See https://www.iana.org/assignments/bootp-dhcp-parameters/bootp-dhcp-parameters.xhtml
func EncodeOpt51(d *dhcpv4.DHCPv4, namespace string) (attribute.KeyValue, error) {
	key := fmt.Sprintf("%v.%v.Opt51.LeaseTime", keyNamespace, namespace)
	if d != nil && d.IPAddressLeaseTime(0) != 0 {
		return attribute.Float64(key, d.IPAddressLeaseTime(0).Seconds()), nil
	}

	return attribute.KeyValue{}, &notFoundError{optName: key}
}

// EncodeOpt53 takes DHCP Opt 53 from a DHCP packet and returns an OTEL key/value pair.
//
// See https://www.iana.org/assignments/bootp-dhcp-parameters/bootp-dhcp-parameters.xhtml
func EncodeOpt53(d *dhcpv4.DHCPv4, namespace string) (attribute.KeyValue, error) {
	key := fmt.Sprintf("%v.%v.Opt53.MessageType", keyNamespace, namespace)
	if d != nil && d.MessageType() != dhcpv4.MessageTypeNone {
		return attribute.String(key, d.MessageType().String()), nil
	}

	return attribute.KeyValue{}, &notFoundError{optName: key}
}

// EncodeOpt54 takes DHCP Opt 54 from a DHCP packet and returns an OTEL key/value pair.
//
// See https://www.iana.org/assignments/bootp-dhcp-parameters/bootp-dhcp-parameters.xhtml
func EncodeOpt54(d *dhcpv4.DHCPv4, namespace string) (attribute.KeyValue, error) {
	key := fmt.Sprintf("%v.%v.Opt54.ServerIdentifier", keyNamespace, namespace)
	if d != nil && d.ServerIdentifier() != nil {
		return attribute.String(key, d.ServerIdentifier().String()), nil
	}

	return attribute.KeyValue{}, &notFoundError{optName: key}
}

// EncodeOpt119 takes DHCP Opt 119 from a DHCP packet and returns an OTEL key/value pair.
//
// See https://www.iana.org/assignments/bootp-dhcp-parameters/bootp-dhcp-parameters.xhtml
func EncodeOpt119(d *dhcpv4.DHCPv4, namespace string) (attribute.KeyValue, error) {
	key := fmt.Sprintf("%v.%v.Opt119.DomainSearch", keyNamespace, namespace)
	if d != nil {
		if l := d.DomainSearch(); l != nil {
			return attribute.String(key, strings.Join(l.Labels, ",")), nil
		}
	}

	return attribute.KeyValue{}, &notFoundError{optName: key}
}

// EncodeYIADDR takes the yiaddr header from a DHCP packet and returns an OTEL key/value pair.
//
// See https://datatracker.ietf.org/doc/html/rfc2131#page-9
func EncodeYIADDR(d *dhcpv4.DHCPv4, namespace string) (attribute.KeyValue, error) {
	key := fmt.Sprintf("%v.%v.Header.yiaddr", keyNamespace, namespace)
	if d != nil && d.YourIPAddr != nil {
		return attribute.String(key, d.YourIPAddr.String()), nil
	}

	return attribute.KeyValue{}, &notFoundError{optName: key}
}

// EncodeSIADDR takes the siaddr header from a DHCP packet and returns an OTEL key/value pair.
//
// See https://datatracker.ietf.org/doc/html/rfc2131#page-9
func EncodeSIADDR(d *dhcpv4.DHCPv4, namespace string) (attribute.KeyValue, error) {
	key := fmt.Sprintf("%v.%v.Header.siaddr", keyNamespace, namespace)
	if d != nil && d.ServerIPAddr != nil {
		return attribute.String(key, d.ServerIPAddr.String()), nil
	}

	return attribute.KeyValue{}, &notFoundError{optName: key}
}

// EncodeCHADDR takes the CHADDR header from a DHCP packet and returns an OTEL key/value pair.
//
// See https://datatracker.ietf.org/doc/html/rfc2131#page-9
func EncodeCHADDR(d *dhcpv4.DHCPv4, namespace string) (attribute.KeyValue, error) {
	key := fmt.Sprintf("%v.%v.Header.chaddr", keyNamespace, namespace)
	if d != nil && d.ClientHWAddr != nil {
		return attribute.String(key, d.ClientHWAddr.String()), nil
	}

	return attribute.KeyValue{}, &notFoundError{optName: key}
}

// EncodeFILE takes the file header from a DHCP packet and returns an OTEL key/value pair.
//
// See https://datatracker.ietf.org/doc/html/rfc2131#page-9
func EncodeFILE(d *dhcpv4.DHCPv4, namespace string) (attribute.KeyValue, error) {
	key := fmt.Sprintf("%v.%v.Header.file", keyNamespace, namespace)
	if d != nil && d.BootFileName != "" {
		return attribute.String(key, d.BootFileName), nil
	}

	return attribute.KeyValue{}, &notFoundError{optName: key}
}

// TraceparentFromContext extracts the binary trace id, span id, and trace flags
// from the running span in ctx and returns a 26 byte []byte with the traceparent
// encoded and ready to pass into a suboption (most likely 69) of opt43.
func TraceparentFromContext(ctx context.Context) []byte {
	sc := trace.SpanContextFromContext(ctx)
	tpBytes := make([]byte, 0, 26)

	// the otel spec says 16 bytes for trace id and 8 for spans are good enough
	// for everyone copy them into a []byte that we can deliver over option43
	tid := [16]byte(sc.TraceID()) // type TraceID [16]byte
	sid := [8]byte(sc.SpanID())   // type SpanID [8]byte

	tpBytes = append(tpBytes, 0x00)      // traceparent version
	tpBytes = append(tpBytes, tid[:]...) // trace id
	tpBytes = append(tpBytes, sid[:]...) // span id
	if sc.IsSampled() {
		tpBytes = append(tpBytes, 0x01) // trace flags
	} else {
		tpBytes = append(tpBytes, 0x00)
	}

	return tpBytes
}
