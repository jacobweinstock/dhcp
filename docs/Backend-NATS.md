# nats.io

This backend implementation is for communicating messages via a [nats](https://nats.io/) message [subject](https://docs.nats.io/nats-concepts/subjects).
This backend uses the [Request-Reply pattern](https://docs.nats.io/nats-concepts/core-nats/reqreply).

This backend allows the development and use of out-of-tree backends.
This will allow legacy backends like [cacher](https://github.com/tinkerbell/boots/blob/ac346cb685046d05ba5296dd0b2083b64fef3287/packet/client.go#L96) to be deprecated from this and other code bases (Boots for example).

## Message Format

Both the [request](#request-message-schema) and [response](#response-message-schema) messages must formatted as [cloudevents](https://cloudevents.io/) ([spec](https://github.com/cloudevents/spec/blob/v1.0.2/cloudevents/spec.md)).
With the `data` field containing the DHCP specific message payload.

Example:

```json
{
    "specversion" : "1.0",
    "type" : "com.github.pull_request.opened",
    "source" : "https://github.com/cloudevents/spec/pull",
    "subject" : "123",
    "id" : "A234-1234-1234",
    "time" : "2018-04-05T17:31:00Z",
    "comexampleextension1" : "value",
    "comexampleothervalue" : 5,
    "datacontenttype" : "application/json",
    "data" : "{\"key\":\"value\"}"
}
```

### Request Message Schema

The schema for the request message is as follows:

```go
// DHCPRequest is the data passed to listening backends.
type DHCPRequest struct {
    Mac         net.HardwareAddr `json:"MACAddress"`
    Traceparent string           `json:"Traceparent"`
}
```

### Response Message Schema

```go
type Message struct {
    DHCP    `json:",inline"`
    Netboot `json:",inline"`
    Error   `json:",inline"`
}

// DHCP holds the headers and options available to be set in a DHCP server response.
// This is the API between the DHCP server and a backend.
type DHCP struct {
    MACAddress       net.HardwareAddr `json:"MACAddress"`       // chaddr DHCP header.
    IPAddress        netaddr.IP       `json:"IPAddress"`        // yiaddr DHCP header.
    SubnetMask       net.IPMask       `json:"SubnetMask"`       // DHCP option 1.
    DefaultGateway   netaddr.IP       `json:"DefaultGateway"`   // DHCP option 3.
    NameServers      []net.IP         `json:"NameServers"`      // DHCP option 6.
    Hostname         string           `json:"Hostname"`         // DHCP option 12.
    DomainName       string           `json:"DomainName"`       // DHCP option 15.
    BroadcastAddress netaddr.IP       `json:"BroadcastAddress"` // DHCP option 28.
    NTPServers       []net.IP         `json:"NTPServers"`       // DHCP option 42.
    LeaseTime        uint32           `json:"LeaseTime"`        // DHCP option 51.
    DomainSearch     []string         `json:"DomainSearch"`     // DHCP option 119.
}

// Netboot holds info used in netbooting a client.
type Netboot struct {
    AllowNetboot  bool     `json:"AllowNetboot"`  // If true, the client will be provided netboot options in the DHCP offer/ack.
    IPXEScriptURL *url.URL `json:"IPXEScriptURL"` // Overrides a default value that is passed into DHCP on startup.
}

type Error struct {
    Code    int    `json:"Code"`
    Message string `json:"Message"`
}
```

```json
{
    "MACAddress":null,
    "IPAddress":"192.168.2.3",
    "SubnetMask":null,
    "DefaultGateway":"",
    "NameServers":null,
    "Hostname":"",
    "DomainName":"",
    "BroadcastAddress":"",
    "NTPServers":null,
    "LeaseTime":0,
    "DomainSearch":null,
    "AllowNetboot":false,
    "IPXEScriptURL":null,
    "Error": {
        "Code":0,
        "Message":""
    }
}

{
    "DHCP": {
        "MACAddress":"",
        "IPAddress":"",
        "SubnetMask":"",
        "DefaultGateway":"",
        "NameServers": [],
        "Hostname":"",
        "DomainName":"",
        "BroadcastAddress":"",
        "NTPServers": [],
        "LeaseTime":0,
        "DomainSearch": []
    },
    "Netboot": {
        "AllowNetboot":false,
        "IPXEScriptURL":""
    },
    "Error": {
        "Code":0,
        "Message":""
    }
}

```

### Example CloudEvent

The following is an example for how to send the request message payload inside of a cloudevent.

```go
// DHCPRequest is the data passed to listening backends.
type DHCPRequest struct {
    Mac         net.HardwareAddr `json:"MACAddress"`
    Traceparent string           `json:"Traceparent"`
}

// create a cloudevent.
event := cloudevents.NewEvent()

// Set the data to the DHCPRequest struct.
event.SetData(cloudevents.ApplicationJSON, &DHCPRequest{Mac: mac, Traceparent: tp})

// Be sure to add encoding of structured data to the context of the request. (why?)
ctx = cloudevents.WithEncodingStructured(ctx)
```

## Architecture

One of the trade-offs of using this backend architecture is that we've added an additional network hop.
The trade-off feels acceptable because of the functionality and flexibility enabled through out-of-tree backends.
Also, there are ways to decrease the impact of the additional network hop.
For example, we can position a sidecar as close to the data as possible.
See the diagram below.

![arch-diagram](dhcp-backend-nats.png)

## Running the Example

```bash
# 1. start the nats server
go run example/main.go -server

# 2. start an example nats client that will respond to messages on the nats dhcp subject.
go run example/main.go -responder

# 3. start the dhcp server
go run example/main.go

```
