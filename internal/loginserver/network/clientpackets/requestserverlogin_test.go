package clientpackets

import (
	"encoding/binary"
	"testing"
)

func TestDecodeRequestServerLogin(t *testing.T) {
	payload := make([]byte, 1+requestServerLoginSize)
	payload[0] = OpcodeRequestServerLogin
	binary.LittleEndian.PutUint32(payload[1:], 111)
	binary.LittleEndian.PutUint32(payload[5:], 222)
	payload[9] = 3

	got, err := DecodeRequestServerLogin(payload)
	if err != nil {
		t.Fatalf("DecodeRequestServerLogin: %v", err)
	}
	want := RequestServerLogin{SessionKey1: 111, SessionKey2: 222, ServerID: 3}
	if got != want {
		t.Errorf("DecodeRequestServerLogin = %+v, want %+v", got, want)
	}
}

func TestDecodeRequestServerLoginShort(t *testing.T) {
	if _, err := DecodeRequestServerLogin(make([]byte, 4)); err == nil {
		t.Error("DecodeRequestServerLogin: want error on short payload, got nil")
	}
}
