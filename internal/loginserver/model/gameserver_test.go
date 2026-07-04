package model

import (
	"bytes"
	"testing"
)

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
