package link

import (
	"bytes"
	"testing"
)

func TestEncodePlayerAuthResponse(t *testing.T) {
	tests := []struct {
		ok   bool
		want byte
	}{
		{true, 1},
		{false, 0},
	}
	for _, tt := range tests {
		got := EncodePlayerAuthResponse("alice", tt.ok)
		want := appendString([]byte{OpcodePlayerAuthResponse}, "alice")
		want = append(want, tt.want)
		if !bytes.Equal(got, want) {
			t.Errorf("EncodePlayerAuthResponse(%v) = %x, want %x", tt.ok, got, want)
		}
	}
}

func TestDecodePlayerAuthResponse(t *testing.T) {
	for _, ok := range []bool{true, false} {
		account, got, err := DecodePlayerAuthResponse(EncodePlayerAuthResponse("alice", ok))
		if err != nil {
			t.Fatalf("DecodePlayerAuthResponse: %v", err)
		}
		if account != "alice" || got != ok {
			t.Fatalf("DecodePlayerAuthResponse() = %q, %v, want alice, %v", account, got, ok)
		}
	}
}

func TestDecodePlayerAuthResponseShort(t *testing.T) {
	if _, _, err := DecodePlayerAuthResponse([]byte{OpcodePlayerAuthResponse, 'a'}); err == nil {
		t.Error("DecodePlayerAuthResponse: want error on unterminated string, got nil")
	}
}
