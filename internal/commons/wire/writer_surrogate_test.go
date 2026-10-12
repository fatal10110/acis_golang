package wire

import (
	"bytes"
	"testing"
)

// A UTF-16 code unit written as its three-byte generalized UTF-8 form
// (CESU-8 / WTF-8) must reach the wire as that single unit, the way a client
// string holding an unpaired surrogate is written unit by unit.
func TestWriterStringPreservesEncodedSurrogateUnits(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want []byte
	}{
		{"lone high surrogate", "\xED\xA0\xBD", []byte{0x3D, 0xD8, 0, 0}},
		{"lone low surrogate", "\xED\xB8\x80", []byte{0x00, 0xDE, 0, 0}},
		{"lowest surrogate", "\xED\xA0\x80", []byte{0x00, 0xD8, 0, 0}},
		{"highest surrogate", "\xED\xBF\xBF", []byte{0xFF, 0xDF, 0, 0}},
		{"surrounded by text", "a\xED\xA0\xBDb", []byte{'a', 0, 0x3D, 0xD8, 'b', 0, 0, 0}},
		{"reversed pair stays two units", "\xED\xB8\x80\xED\xA0\xBD", []byte{0x00, 0xDE, 0x3D, 0xD8, 0, 0}},
		{"encoded pair", "\xED\xA0\xBD\xED\xB8\x80", []byte{0x3D, 0xD8, 0x00, 0xDE, 0, 0}},
		{"last unit before surrogates", "\xED\x9F\xBF", []byte{0xFF, 0xD7, 0, 0}},
		{"truncated surrogate", "\xED\xA0", []byte{0xFD, 0xFF, 0xFD, 0xFF, 0, 0}},
		{"bad trailing byte", "\xED\xA0\x41", []byte{0xFD, 0xFF, 0xFD, 0xFF, 'A', 0, 0, 0}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var w Writer
			w.WriteString(tc.in)
			if got := w.Bytes(); !bytes.Equal(got, tc.want) {
				t.Fatalf("WriteString(%q) = % X, want % X", tc.in, got, tc.want)
			}
		})
	}
}

// An encoded surrogate pair reaches the wire with the same bytes as the
// supplementary character it spells.
func TestWriterStringEncodedPairMatchesSupplementaryCharacter(t *testing.T) {
	var cesu, plain Writer
	cesu.WriteString("\xED\xA0\xBD\xED\xB8\x80")
	plain.WriteString("\U0001F600")
	if !bytes.Equal(cesu.Bytes(), plain.Bytes()) {
		t.Fatalf("encoded pair = % X, U+1F600 = % X", cesu.Bytes(), plain.Bytes())
	}
}

// Every non-zero code unit, written as its generalized UTF-8 form, comes out
// as exactly that unit.
func TestWriterStringWritesEveryEncodedUnitVerbatim(t *testing.T) {
	w := NewFrameWriter(8)
	for unit := 1; unit <= 0xFFFF; unit++ {
		w.ResetFrame(8)
		w.WriteString(string(generalizedUTF8(uint16(unit))))
		want := []byte{byte(unit), byte(unit >> 8), 0, 0}
		if got := w.Bytes(); !bytes.Equal(got, want) {
			t.Fatalf("unit %04X wrote % X, want % X", unit, got, want)
		}
	}
}

func TestWriterStringWithSurrogateDoesNotAllocate(t *testing.T) {
	w := NewFrameWriter(64)
	allocs := testing.AllocsPerRun(1000, func() {
		w.ResetFrame(64)
		w.WriteString("Player\xED\xA0\xBDName")
	})
	if allocs != 0 {
		t.Fatalf("WriteString allocations = %v, want 0", allocs)
	}
}

// generalizedUTF8 encodes one UTF-16 code unit in the one-to-three-byte UTF-8
// layout, surrogates included.
func generalizedUTF8(unit uint16) []byte {
	switch {
	case unit < 0x80:
		return []byte{byte(unit)}
	case unit < 0x800:
		return []byte{0xC0 | byte(unit>>6), 0x80 | byte(unit&0x3F)}
	default:
		return []byte{0xE0 | byte(unit>>12), 0x80 | byte(unit>>6&0x3F), 0x80 | byte(unit&0x3F)}
	}
}
