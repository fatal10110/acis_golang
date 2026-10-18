package serverpackets

import (
	"bytes"
	"testing"
)

// Golden frames: a 2-byte little-endian length that counts itself, then
// EventTrigger's body: 0xcf, the event id (D), active (C).
func TestEventTriggerGolden(t *testing.T) {
	tests := []struct {
		name   string
		id     int32
		active bool
		want   []byte
	}{
		{"Off", 2418001, false, []byte{0x08, 0x00, 0xcf, 0x51, 0xe5, 0x24, 0x00, 0x00}},
		{"On", 2418001, true, []byte{0x08, 0x00, 0xcf, 0x51, 0xe5, 0x24, 0x00, 0x01}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := frameBytes(t, FrameEventTrigger(tt.id, tt.active)); !bytes.Equal(got, tt.want) {
				t.Fatalf("EventTrigger(%d, %v) frame = % x, want % x", tt.id, tt.active, got, tt.want)
			}
		})
	}
}
