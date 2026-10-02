package serverpackets

import (
	"bytes"
	"testing"
)

// Golden frames: a 2-byte little-endian length that counts itself, then
// CameraMode's body as the reference writes it: 0xf1, the mode (D).
func TestCameraModeGolden(t *testing.T) {
	tests := []struct {
		name string
		mode int32
		want []byte
	}{
		{"ThirdPerson", CameraModeThirdPerson, []byte{0x07, 0x00, 0xf1, 0x00, 0x00, 0x00, 0x00}},
		{"FirstPerson", CameraModeFirstPerson, []byte{0x07, 0x00, 0xf1, 0x01, 0x00, 0x00, 0x00}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := frameBytes(t, FrameCameraMode(tt.mode)); !bytes.Equal(got, tt.want) {
				t.Fatalf("CameraMode(%d) frame = % x, want % x", tt.mode, got, tt.want)
			}
		})
	}
}
