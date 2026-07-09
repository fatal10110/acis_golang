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

func TestFrameAuthLoginFail(t *testing.T) {
	frame := FrameAuthLoginFail(LoginFailSystemErrorTryLater)
	defer frame.Release()

	want := []byte{0x07, 0x00, OpcodeAuthLoginFail, 0x01, 0x00, 0x00, 0x00}
	if !bytes.Equal(frame.Bytes(), want) {
		t.Fatalf("FrameAuthLoginFail(%v) = % X, want % X", LoginFailSystemErrorTryLater, frame.Bytes(), want)
	}

	payload := EncodeAuthLoginFail(LoginFailSystemErrorTryLater)
	if !bytes.Equal(frame.Bytes()[2:], payload) {
		t.Fatalf("framed payload = % X, want EncodeAuthLoginFail output % X", frame.Bytes()[2:], payload)
	}
}
