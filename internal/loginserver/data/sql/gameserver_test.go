package sql

import (
	"bytes"
	"testing"
)

func TestGameServerHexIDStringRoundTrip(t *testing.T) {
	tests := []struct {
		name string
		id   []byte
		want string
	}{
		{name: "positive", id: []byte{0x01, 0x02, 0x03}, want: "10203"},
		{name: "negative", id: []byte{0x80, 0x01}, want: "-7fff"},
		{name: "positive with sign byte", id: []byte{0x00, 0x80, 0x01}, want: "8001"},
		{name: "negative one", id: []byte{0xff}, want: "-1"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := hexIDString(tt.id)
			if got != tt.want {
				t.Fatalf("hexIDString(%x) = %q, want %q", tt.id, got, tt.want)
			}

			roundTrip, err := parseHexID(got)
			if err != nil {
				t.Fatalf("parseHexID(%q) unexpected error: %v", got, err)
			}
			if !bytes.Equal(roundTrip, tt.id) {
				t.Fatalf("parseHexID(%q) = %x, want %x", got, roundTrip, tt.id)
			}
		})
	}
}
