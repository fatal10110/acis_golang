package serverpackets

import (
	"bytes"
	"testing"
)

func TestFrameRestartResponse(t *testing.T) {
	tests := []struct {
		name string
		ok   bool
		want []byte
	}{
		{"success", true, []byte{OpcodeRestartResponse, 1, 0, 0, 0}},
		{"failure", false, []byte{OpcodeRestartResponse, 0, 0, 0, 0}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := framePayload(t, FrameRestartResponse(tt.ok))
			if !bytes.Equal(got, tt.want) {
				t.Fatalf("FrameRestartResponse(%v) = %x, want %x", tt.ok, got, tt.want)
			}
		})
	}
}

func TestFrameLeaveWorld(t *testing.T) {
	got := framePayload(t, FrameLeaveWorld())
	want := []byte{OpcodeLeaveWorld}
	if !bytes.Equal(got, want) {
		t.Fatalf("FrameLeaveWorld() = %x, want %x", got, want)
	}
}
