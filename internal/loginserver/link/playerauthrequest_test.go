package link

import (
	"encoding/binary"
	"testing"
)

func TestDecodePlayerAuthRequest(t *testing.T) {
	payload := appendString([]byte{OpcodePlayerAuthRequest}, "alice")
	payload = binary.LittleEndian.AppendUint32(payload, uint32(int32(11)))
	payload = binary.LittleEndian.AppendUint32(payload, uint32(int32(22)))
	payload = binary.LittleEndian.AppendUint32(payload, uint32(int32(33)))
	payload = binary.LittleEndian.AppendUint32(payload, uint32(int32(44)))

	got, err := DecodePlayerAuthRequest(payload)
	if err != nil {
		t.Fatalf("DecodePlayerAuthRequest: %v", err)
	}
	want := PlayerAuthRequest{Account: "alice", PlayKey1: 11, PlayKey2: 22, LoginKey1: 33, LoginKey2: 44}
	if got != want {
		t.Fatalf("DecodePlayerAuthRequest() = %+v, want %+v", got, want)
	}
}

func TestDecodePlayerAuthRequestShort(t *testing.T) {
	payload := appendString([]byte{OpcodePlayerAuthRequest}, "alice")
	if _, err := DecodePlayerAuthRequest(payload); err == nil {
		t.Error("DecodePlayerAuthRequest: want error on short payload, got nil")
	}
}
