package link

import (
	"bytes"
	"encoding/binary"
	"testing"
)

func TestEncodeInitLS(t *testing.T) {
	pubKey := bytes.Repeat([]byte{0xaa}, 128)

	got := EncodeInitLS(pubKey)

	var want []byte
	want = append(want, OpcodeInitLS)
	want = binary.LittleEndian.AppendUint32(want, ProtocolRevision)
	want = binary.LittleEndian.AppendUint32(want, uint32(len(pubKey)))
	want = append(want, pubKey...)

	if !bytes.Equal(got, want) {
		t.Errorf("EncodeInitLS() = %x, want %x", got, want)
	}
}

func TestDecodeInitLS(t *testing.T) {
	pubKey := bytes.Repeat([]byte{0xaa}, 128)
	payload := EncodeInitLS(pubKey)

	revision, key, err := DecodeInitLS(payload)
	if err != nil {
		t.Fatalf("DecodeInitLS: %v", err)
	}
	if revision != ProtocolRevision {
		t.Errorf("revision = %#x, want %#x", revision, ProtocolRevision)
	}
	if !bytes.Equal(key, pubKey) {
		t.Errorf("publicKey = %x, want %x", key, pubKey)
	}
}

func TestDecodeInitLSShort(t *testing.T) {
	payload := []byte{OpcodeInitLS, 0x02, 0x01, 0x00, 0x00, 0xff, 0xff, 0xff, 0x7f} // claims ~2GB of key bytes
	if _, _, err := DecodeInitLS(payload); err == nil {
		t.Error("DecodeInitLS: want error on truncated payload, got nil")
	}
}
