package model

import (
	"bytes"
	"testing"
)

// A 16-byte key whose first byte is zero and second byte has the top bit
// clear reads back from its hex text without the leading zero: the text form
// is a signed big integer, so the round trip yields the minimal form.
func TestParseHexKeyReturnsMinimalFormOfLeadingZeroKey(t *testing.T) {
	key := []byte{0x00, 0x7f, 1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14}
	got, err := ParseHexKey(HexKeyText(key))
	if err != nil {
		t.Fatalf("ParseHexKey: %v", err)
	}
	if want := key[1:]; !bytes.Equal(got, want) {
		t.Fatalf("round trip = %x, want the minimal form %x", got, want)
	}
	if again := HexKeyText(got); again != HexKeyText(key) {
		t.Fatalf("minimal form renders %q, want %q", again, HexKeyText(key))
	}
}
