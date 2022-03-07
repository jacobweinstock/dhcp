package nats

import (
	"context"
	"errors"
	"net"
	"testing"

	"github.com/google/go-cmp/cmp"
)

func TestCreateCloudevent(t *testing.T) {
	tests := map[string]struct {
		mac     net.HardwareAddr
		id      string
		want    string
		wantErr error
	}{
		"fail": {
			mac:  net.HardwareAddr{0x00, 0x00, 0x00, 0x00, 0x00, 0x01},
			id:   "f0f7ed74-510a-4a98-ad08-be29d5c0fa68",
			want: `{"specversion":"1.0","id":"f0f7ed74-510a-4a98-ad08-be29d5c0fa68","source":"/tinkerbell/dhcp","type":"org.tinkerbell.dhcp.backend.read","datacontenttype":"application/json","data":{"MacAddress":"00:00:00:00:01","Traceparent":""}}`,
		},
	}
	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			got, err := createCloudevent(context.Background(), tt.id, tt.mac)
			if !errors.Is(err, tt.wantErr) {
				t.Fatalf("createCloudevent() error (type: %T) = %[1]v, wantErr (type: %T) %[2]v", err, tt.wantErr)
			}
			if diff := cmp.Diff(string(got), tt.want); diff != "" {
				t.Fatalf(diff)
			}
		})
	}
}
