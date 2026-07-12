package clientpackets

import "testing"

func TestDecodeProtocolVersion(t *testing.T) {
	payload := []byte{OpcodeProtocolVersion, 0x21, 0xc6, 0x00, 0x00} // 0xc621, Interlude revision
	got, err := DecodeProtocolVersion(payload)
	if err != nil {
		t.Fatalf("DecodeProtocolVersion: %v", err)
	}
	if got.Revision != 0xc621 {
		t.Errorf("Revision = %#x, want %#x", got.Revision, 0xc621)
	}
}

func TestDecodeProtocolVersionShort(t *testing.T) {
	if _, err := DecodeProtocolVersion([]byte{OpcodeProtocolVersion, 0x01}); err == nil {
		t.Fatal("DecodeProtocolVersion() error = nil, want short-payload error")
	}
}
