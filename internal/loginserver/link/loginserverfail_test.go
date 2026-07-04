package link

import (
	"bytes"
	"testing"
)

func TestEncodeLoginServerFail(t *testing.T) {
	got := EncodeLoginServerFail(ReasonAlreadyLoggedIn)
	want := []byte{OpcodeLoginServerFail, byte(ReasonAlreadyLoggedIn)}
	if !bytes.Equal(got, want) {
		t.Errorf("EncodeLoginServerFail() = %x, want %x", got, want)
	}
}
