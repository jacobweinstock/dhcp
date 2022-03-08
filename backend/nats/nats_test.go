package nats

import (
	"context"
	"errors"
	"net"
	"testing"

	cloudevents "github.com/cloudevents/sdk-go/v2"
	"github.com/google/go-cmp/cmp"
)

func TestCreateCloudevent(t *testing.T) {
	type event struct {
		id     string
		source string
		etype  string
		data   DHCPRequest
	}
	tests := map[string]struct {
		mac     net.HardwareAddr
		id      string
		want    event
		wantErr error
	}{
		"fail": {
			mac: net.HardwareAddr{0x00, 0x00, 0x00, 0x00, 0x00, 0x01},
			id:  "f0f7ed74-510a-4a98-ad08-be29d5c0fa68",
			want: event{
				id:     "f0f7ed74-510a-4a98-ad08-be29d5c0fa68",
				source: "/tinkerbell/dhcp",
				etype:  "org.tinkerbell.dhcp.backend.read",
				data:   DHCPRequest{Mac: net.HardwareAddr{0x00, 0x00, 0x00, 0x00, 0x00, 0x01}},
			},
		},
	}
	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			got, err := createCloudevent(context.Background(), tt.id, tt.mac)
			if !errors.Is(err, tt.wantErr) {
				t.Fatalf("createCloudevent() error (type: %T) = %[1]v, wantErr (type: %T) %[2]v", err, tt.wantErr)
			}
			want := cloudevents.NewEvent()
			want.SetID(tt.want.id)
			want.SetSource(tt.want.source)
			want.SetType(tt.want.etype)
			want.SetData(cloudevents.ApplicationJSON, tt.want.data)
			if diff := cmp.Diff(got, want); diff != "" {
				t.Fatalf(diff)
			}
		})
	}
}
