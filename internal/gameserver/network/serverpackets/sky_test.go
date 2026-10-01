package serverpackets

import (
	"bytes"
	"testing"

	"github.com/fatal10110/acis_golang/internal/commons/wire"
)

// Golden frames: a 2-byte little-endian length that counts itself, then the
// single opcode byte the reference packets write (SunRise 0x1c, SunSet 0x1d).
func TestSkyFramesGolden(t *testing.T) {
	tests := []struct {
		name  string
		frame func() wire.Frame
		want  []byte
	}{
		{"SunRise", FrameSunRise, []byte{0x03, 0x00, 0x1c}},
		{"SunSet", FrameSunSet, []byte{0x03, 0x00, 0x1d}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := frameBytes(t, tt.frame()); !bytes.Equal(got, tt.want) {
				t.Fatalf("%s frame = % x, want % x", tt.name, got, tt.want)
			}
		})
	}
}
