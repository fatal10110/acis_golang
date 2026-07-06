package link

import (
	"bytes"
	"testing"
)

func TestEncodeAuthResponse(t *testing.T) {
	got := EncodeAuthResponse(3, "MyServer")
	want := appendString([]byte{OpcodeAuthResponse, 3}, "MyServer")
	if !bytes.Equal(got, want) {
		t.Errorf("EncodeAuthResponse() = %x, want %x", got, want)
	}
}

func TestDecodeAuthResponse(t *testing.T) {
	id, name, err := DecodeAuthResponse(EncodeAuthResponse(3, "MyServer"))
	if err != nil {
		t.Fatalf("DecodeAuthResponse: %v", err)
	}
	if id != 3 || name != "MyServer" {
		t.Fatalf("DecodeAuthResponse() = %d, %q, want 3, MyServer", id, name)
	}
}

func TestDecodeAuthResponseShort(t *testing.T) {
	if _, _, err := DecodeAuthResponse([]byte{OpcodeAuthResponse}); err == nil {
		t.Error("DecodeAuthResponse: want error on short payload, got nil")
	}
}
