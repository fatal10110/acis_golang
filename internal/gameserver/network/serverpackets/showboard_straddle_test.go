package serverpackets

import (
	"bytes"
	"encoding/binary"
	"strings"
	"testing"
)

// A character whose two UTF-16 code units straddle the 4090-unit cut
// reaches the client as its two halves: the leading unit ends part 101 and
// the trailing one starts part 102.
func TestFramesShowBoardPageSplitsPairAcrossParts(t *testing.T) {
	html := strings.Repeat("a", 4089) + "\U0001F600" + "b"
	frames := FramesShowBoardPage(html)
	if len(frames) != 3 {
		t.Fatalf("FramesShowBoardPage parts = %d, want 3", len(frames))
	}
	header := showBoardHeader()
	for i, want := range [][]uint16{
		append(append(utf16Units("101\b"), utf16Units(strings.Repeat("a", 4089))...), 0xD83D, 0),
		append(utf16Units("102\b"), 0xDE00, 'b', 0),
	} {
		var body []byte
		for _, u := range want {
			body = binary.LittleEndian.AppendUint16(body, u)
		}
		if got := framePayload(t, frames[i]); !bytes.Equal(got, append(bytes.Clone(header), body...)) {
			t.Fatalf("part %d ends % X, want % X", i, got[max(0, len(got)-8):], body[len(body)-8:])
		}
	}
}

func utf16Units(s string) []uint16 {
	var out []uint16
	for _, r := range s {
		out = append(out, uint16(r))
	}
	return out
}
