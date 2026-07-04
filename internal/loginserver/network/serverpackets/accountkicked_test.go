package serverpackets

import (
	"bytes"
	"encoding/binary"
	"testing"
)

func TestEncodeAccountKicked(t *testing.T) {
	got := EncodeAccountKicked(AccountKickedPermanentlyBanned)

	var want []byte
	want = append(want, OpcodeAccountKicked)
	want = binary.LittleEndian.AppendUint32(want, uint32(AccountKickedPermanentlyBanned))

	if !bytes.Equal(got, want) {
		t.Errorf("EncodeAccountKicked = %x, want %x", got, want)
	}
}
