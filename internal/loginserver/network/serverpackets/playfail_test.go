package serverpackets

import (
	"bytes"
	"testing"
)

func TestEncodePlayFail(t *testing.T) {
	got := EncodePlayFail(PlayFailTooManyPlayers)
	want := []byte{OpcodePlayFail, byte(PlayFailTooManyPlayers)}

	if !bytes.Equal(got, want) {
		t.Errorf("EncodePlayFail = %x, want %x", got, want)
	}
}
