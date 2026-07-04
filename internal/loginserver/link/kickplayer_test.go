package link

import (
	"bytes"
	"testing"
)

func TestEncodeKickPlayer(t *testing.T) {
	got := EncodeKickPlayer("alice")
	want := appendString([]byte{OpcodeKickPlayer}, "alice")
	if !bytes.Equal(got, want) {
		t.Errorf("EncodeKickPlayer() = %x, want %x", got, want)
	}
}
