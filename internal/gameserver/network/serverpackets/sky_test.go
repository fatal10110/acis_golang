package serverpackets

import (
	"bytes"
	"testing"

	"github.com/fatal10110/acis_golang/internal/commons/wire"
)

// Golden frames: a 2-byte little-endian length that counts itself, then the
// packet body the reference writes (SunRise 0x1c, SunSet 0x1d, ExRedSky).
func TestSkyFramesGolden(t *testing.T) {
	tests := []struct {
		name  string
		frame func() wire.Frame
		want  []byte
	}{
		{"SunRise", FrameSunRise, []byte{0x03, 0x00, 0x1c}},
		{"SunSet", FrameSunSet, []byte{0x03, 0x00, 0x1d}},
		// ExRedSky: 0xfe, sub-opcode 0x0040 (H), duration (D).
		{"ExRedSky", func() wire.Frame { return FrameExRedSky(10) }, []byte{0x09, 0x00, 0xfe, 0x40, 0x00, 0x0a, 0x00, 0x00, 0x00}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := frameBytes(t, tt.frame()); !bytes.Equal(got, tt.want) {
				t.Fatalf("%s frame = % x, want % x", tt.name, got, tt.want)
			}
		})
	}
}
