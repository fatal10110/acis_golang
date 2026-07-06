package serverpackets

import (
	"bytes"
	"testing"
)

func TestEncodeCharDeleteOk(t *testing.T) {
	got := EncodeCharDeleteOk()
	want := []byte{OpcodeCharDeleteOk}
	if !bytes.Equal(got, want) {
		t.Errorf("EncodeCharDeleteOk = %x, want %x", got, want)
	}
}
