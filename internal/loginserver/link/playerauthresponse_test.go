package link

import (
	"bytes"
	"testing"
)

func TestEncodePlayerAuthResponse(t *testing.T) {
	tests := []struct {
		ok   bool
		want byte
	}{
		{true, 1},
		{false, 0},
	}
	for _, tt := range tests {
		got := EncodePlayerAuthResponse("alice", tt.ok)
		want := appendString([]byte{OpcodePlayerAuthResponse}, "alice")
		want = append(want, tt.want)
		if !bytes.Equal(got, want) {
			t.Errorf("EncodePlayerAuthResponse(%v) = %x, want %x", tt.ok, got, want)
		}
	}
}
