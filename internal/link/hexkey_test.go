package link

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

func TestHexKeyTextRoundTrip(t *testing.T) {
	tests := []struct {
		name string
		key  []byte
		want string
	}{
		{name: "positive", key: []byte{0x01, 0x02, 0x03}, want: "10203"},
		{name: "negative", key: []byte{0x80, 0x01}, want: "-7fff"},
		{name: "positive with sign byte", key: []byte{0x00, 0x80, 0x01}, want: "8001"},
		{name: "negative one", key: []byte{0xff}, want: "-1"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := HexKeyText(tt.key)
			if got != tt.want {
				t.Fatalf("HexKeyText(%x) = %q, want %q", tt.key, got, tt.want)
			}

			roundTrip, err := ParseHexKey(got)
			if err != nil {
				t.Fatalf("ParseHexKey(%q) unexpected error: %v", got, err)
			}
			if !bytes.Equal(roundTrip, tt.key) {
				t.Fatalf("ParseHexKey(%q) = %x, want %x", got, roundTrip, tt.key)
			}
		})
	}
}
