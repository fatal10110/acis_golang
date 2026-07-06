package serverpackets

import (
	"bytes"
	"encoding/binary"
	"testing"
)

func TestEncodeAuthLoginFail(t *testing.T) {
	got := EncodeAuthLoginFail(LoginFailSystemErrorTryLater)

	var want []byte
	want = append(want, OpcodeAuthLoginFail)
	want = binary.LittleEndian.AppendUint32(want, uint32(LoginFailSystemErrorTryLater))

	if !bytes.Equal(got, want) {
		t.Errorf("EncodeAuthLoginFail(%v) = % X, want % X", LoginFailSystemErrorTryLater, got, want)
	}
}
