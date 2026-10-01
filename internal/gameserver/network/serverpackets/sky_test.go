package serverpackets

import (
	"bytes"
	"testing"

	"github.com/fatal10110/acis_golang/internal/commons/wire"
)

// Golden frames: a 2-byte little-endian length that counts itself, then the
// packet body the reference writes (SunRise 0x1c, SunSet 0x1d, ExRedSky,
// SSQInfo with each seven-signs sky state).
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
		// SSQInfo: 0xf8, sky state (H).
		{"SSQInfoRegular", func() wire.Frame { return FrameSSQInfoSky(SSQSkyRegular) }, []byte{0x05, 0x00, 0xf8, 0x00, 0x01}},
		{"SSQInfoDusk", func() wire.Frame { return FrameSSQInfoSky(SSQSkyDusk) }, []byte{0x05, 0x00, 0xf8, 0x01, 0x01}},
		{"SSQInfoDawn", func() wire.Frame { return FrameSSQInfoSky(SSQSkyDawn) }, []byte{0x05, 0x00, 0xf8, 0x02, 0x01}},
		{"SSQInfoRed", func() wire.Frame { return FrameSSQInfoSky(SSQSkyRed) }, []byte{0x05, 0x00, 0xf8, 0x03, 0x01}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := frameBytes(t, tt.frame()); !bytes.Equal(got, tt.want) {
				t.Fatalf("%s frame = % x, want % x", tt.name, got, tt.want)
			}
		})
	}
}
