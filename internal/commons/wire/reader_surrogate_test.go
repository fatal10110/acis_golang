package wire

import (
	"bytes"
	"testing"
)

// Every string field a client can send comes back out of the writer with
// the same code units, unpaired surrogates included.
func TestReadStringWriteStringRoundTripsEveryUnitSequence(t *testing.T) {
	sequences := [][]uint16{
		{0xD83D},
		{0xDE00},
		{'a', 0xD800, 'b'},
		{0xDBFF, 0xDBFF},
		{0xDE00, 0xD83D},
		{0xD83D, 0xDE00},
		{0xD83D, 0xD83D, 0xDE00},
		{0xD83D, 0xDE00, 0xDE00},
		{0xFFFD, 0xD7FF, 0xE000},
	}
	for unit := 1; unit <= 0xFFFF; unit++ {
		sequences = append(sequences, []uint16{uint16(unit)})
	}
	for _, units := range sequences {
		payload := utf16LE(units, true)
		s := NewReader(payload).ReadString()
		var w Writer
		w.WriteString(s)
		if got := w.Bytes(); !bytes.Equal(got, payload) {
			t.Fatalf("units %04X read as %q wrote back % X, want % X", units, s, got, payload)
		}
	}
}
