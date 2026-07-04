package clientpackets

import (
	"encoding/binary"
	"testing"
)

func TestDecodeRequestServerList(t *testing.T) {
	payload := make([]byte, 1+requestServerListSize)
	payload[0] = OpcodeRequestServerList
	binary.LittleEndian.PutUint32(payload[1:], 111)
	binary.LittleEndian.PutUint32(payload[5:], 222)

	got, err := DecodeRequestServerList(payload)
	if err != nil {
		t.Fatalf("DecodeRequestServerList: %v", err)
	}
	want := RequestServerList{SessionKey1: 111, SessionKey2: 222}
	if got != want {
		t.Errorf("DecodeRequestServerList = %+v, want %+v", got, want)
	}
}

func TestDecodeRequestServerListShort(t *testing.T) {
	if _, err := DecodeRequestServerList(make([]byte, 4)); err == nil {
		t.Error("DecodeRequestServerList: want error on short payload, got nil")
	}
}
