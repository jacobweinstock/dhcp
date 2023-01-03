// Package noop is a backend handler that does nothing.
package noop

import (
	"context"
	"errors"
	"net"

	"github.com/tinkerbell/dhcp/data"
)

// Handler is a noop backend.
type Handler struct{}

// Read for the noop handler just returns an error.
func (h Handler) Read(_ context.Context, _ net.HardwareAddr) (*data.DHCP, *data.Netboot, error) {
	return nil, nil, errors.New("no backend specified, please specify a backend")
}

// Name returns the noop handler name.
func (h Handler) Name() string {
	return "noop"
}
