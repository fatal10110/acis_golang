package social

import (
	"bytes"
	"encoding/binary"
	"strings"
	"testing"
)

// utf16Field is units as a null-terminated UTF-16LE string field.
func utf16Field(units ...uint16) []byte {
	out := make([]byte, 0, 2*len(units)+2)
	for _, u := range append(units, 0) {
		out = binary.LittleEndian.AppendUint16(out, u)
	}
	return out
}

// TestChatEchoesUnpairedSurrogates pins chat text holding an unpaired
// UTF-16 surrogate: every listener receives the line with the units the
// speaker sent, and the unit counts one toward the 100-unit limit.
func TestChatEchoesUnpairedSurrogates(t *testing.T) {
	t.Parallel()
	p := bootPair(t)
	p.enterAll(t)

	// "\xED\xA0\xBD" and "\xED\xB8\x80" spell the lone units D83D and DE00,
	// which the test client's writer sends as those single units.
	p.alice.Send(encodeSay2(sayAll, "hi\xED\xA0\xBD!\xED\xB8\x80"))
	wantText := utf16Field('h', 'i', 0xD83D, '!', 0xDE00)
	for _, c := range []struct {
		name  string
		frame []byte
	}{{"speaker", readSay(t, p.alice)}, {"listener", readSay(t, p.bobby)}} {
		if !bytes.HasSuffix(c.frame, wantText) {
			t.Fatalf("%s CreatureSay % X does not end with the sent text % X", c.name, c.frame, wantText)
		}
	}

	// 99 letters and one surrogate are 100 units: delivered.
	full := strings.Repeat("x", 99) + "\xED\xA0\xBD"
	p.alice.Send(encodeSay2(sayAll, full))
	assertSay(t, p.bobby, p.aliceID, sayAll, "Alice", full)
	drainUntilQuiet(t, p.alice)

	// One more unit is over the limit: dropped.
	p.alice.Send(encodeSay2(sayAll, "x"+full))
	assertNoSay(t, p.bobby, "line of 101 units")
	assertNoSay(t, p.alice, "speaker of a 101-unit line")
}
