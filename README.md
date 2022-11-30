[![Test and Build](https://github.com/tinkerbell/dhcp/actions/workflows/ci.yaml/badge.svg)](https://github.com/tinkerbell/dhcp/actions/workflows/ci.yaml)
[![codecov](https://codecov.io/gh/tinkerbell/dhcp/branch/main/graph/badge.svg)](https://codecov.io/gh/tinkerbell/dhcp)
[![Go Report Card](https://goreportcard.com/badge/github.com/tinkerbell/dhcp)](https://goreportcard.com/report/github.com/tinkerbell/dhcp)
[![Go Reference](https://pkg.go.dev/badge/github.com/tinkerbell/dhcp.svg)](https://pkg.go.dev/github.com/tinkerbell/dhcp)

# dhcp

DHCP library and CLI server with multiple handlers and backends. The order of consumption is generally as follows: `listener(handler(backend))`. A listener takes a handler, which takes a backend.

## Listener

The listener is the main entry point for the library. It listens for broadcast/unicast DHCP packets on a specific port. It takes in a slice of handlers and calls each of them for each incoming DHCP packet.

## Handlers

Handlers are functions that are called when a packet is received.

- Reservation
  - This handler provides only DHCP host reservations. There are no leases.
  - Definitions
    - `reservation`: A fixed IP address that is reserved for a specific client.
    - `lease`: An IP address, that can potentially change, that is assigned to a client by a DHCP server. The IP is typically pulled from a pool or subnet of available IP addresses.
- ProxyDHCP
  - This handler provides proxyDHCP functionality.
    > [A] Proxy DHCP server behaves much like a DHCP server by listening for ordinary DHCP client traffic and responding to certain client requests. However, unlike the DHCP server, the PXE Proxy DHCP server does not administer network addresses, and it only responds to clients that identify themselves as PXE clients.
    > The responses given by the PXE Proxy DHCP server contain the mechanism by which the client locates the boot servers or the network addresses and descriptions of the supported, compatible boot servers."
    > -- [IBM](https://www.ibm.com/docs/en/aix/7.1?topic=protocol-preboot-execution-environment-proxy-dhcp-daemon)
- Relay
  - This handler provides DHCP relay functionality. Translating a broadcasted DHCP request into a unicast DHCP request to an upstream DHCP server.

## Backends

Backends are functions that are called to interact with some kind of a persistent data store. They are typically called from a handler to get DHCP data that will be used to populate DHCP responses.

- [Tink Kubernetes](https://github.com/tinkerbell/tink)
  - This backend is the main use case. It interacts with the Hardware CRD of Tink to get DHCP information.
- File based
  - This backend is for mainly for testing and development.
  It reads a file for hardware data. See [example.yaml](./backend/file/testdata/example.yaml) for the data model.
