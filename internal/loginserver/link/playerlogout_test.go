package link

import "testing"

func TestDecodePlayerLogout(t *testing.T) {
	payload := appendString([]byte{OpcodePlayerLogout}, "alice")

	got, err := DecodePlayerLogout(payload)
	if err != nil {
		t.Fatalf("DecodePlayerLogout: %v", err)
	}
	if got != "alice" {
		t.Fatalf("DecodePlayerLogout() = %q, want %q", got, "alice")
	}
}

func TestDecodePlayerLogoutShort(t *testing.T) {
	if _, err := DecodePlayerLogout([]byte{OpcodePlayerLogout, 'a'}); err == nil {
		t.Error("DecodePlayerLogout: want error on unterminated string, got nil")
	}
}
