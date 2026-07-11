package serverpackets

import (
	"bytes"
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
)

func TestFrameTeleportToLocation(t *testing.T) {
	to := location.Location{X: -71440, Y: 258000, Z: -3104}

	tests := []struct {
		name         string
		fastTeleport bool
		want         []byte
	}{
		{
			name:         "black screen",
			fastTeleport: false,
			want: []byte{
				0x28,
				0x64, 0xa0, 0x00, 0x10,
				0xf0, 0xe8, 0xfe, 0xff,
				0xd0, 0xef, 0x03, 0x00,
				0xe0, 0xf3, 0xff, 0xff,
				0x00, 0x00, 0x00, 0x00,
			},
		},
		{
			name:         "fast teleport",
			fastTeleport: true,
			want: []byte{
				0x28,
				0x64, 0xa0, 0x00, 0x10,
				0xf0, 0xe8, 0xfe, 0xff,
				0xd0, 0xef, 0x03, 0x00,
				0xe0, 0xf3, 0xff, 0xff,
				0x01, 0x00, 0x00, 0x00,
			},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := framePayload(t, FrameTeleportToLocation(268476516, to, tc.fastTeleport))
			if !bytes.Equal(got, tc.want) {
				t.Errorf("FrameTeleportToLocation(_, _, %v) = %x, want %x", tc.fastTeleport, got, tc.want)
			}
		})
	}
}
