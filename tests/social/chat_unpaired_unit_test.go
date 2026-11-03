package social

import (
	"strings"
	"testing"

	"github.com/fatal10110/acis_golang/internal/commons/wire"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/clientpackets"
)

// encodeRawSay2 is Say2 with its text given as raw UTF-16 code units, so a
// test can send an unpaired surrogate.
func encodeRawSay2(typ int32, text ...uint16) []byte {
	w := wire.NewPacketWriter(clientpackets.OpcodeSay2)
	w.WriteUTF16(text)
	w.WriteInt32(typ)
	return w.Bytes()
}

func rawUnits(s string, extra ...uint16) []uint16 {
	var out []uint16
	for _, r := range s {
		out = append(out, uint16(r))
	}
	return append(out, extra...)
}

// TestChatUnpairedSurrogateReadsAsReplacement pins chat text holding an
// unpaired UTF-16 surrogate: every listener receives the line with U+FFFD in
// the unit's place, and the unit counts one toward the 100-unit limit.
func TestChatUnpairedSurrogateReadsAsReplacement(t *testing.T) {
	t.Parallel()
	p := bootPair(t)
	p.enterAll(t)

	p.alice.Send(encodeRawSay2(sayAll, append(rawUnits("hi", 0xD83D), '!', 0xDE00)...))
	assertSay(t, p.alice, p.aliceID, sayAll, "Alice", "hi�!�")
	assertSay(t, p.bobby, p.aliceID, sayAll, "Alice", "hi�!�")

	// 99 letters and one surrogate are 100 units: delivered.
	full := rawUnits(strings.Repeat("x", 99), 0xD83D)
	p.alice.Send(encodeRawSay2(sayAll, full...))
	assertSay(t, p.bobby, p.aliceID, sayAll, "Alice", strings.Repeat("x", 99)+"�")
	drainUntilQuiet(t, p.alice)

	// One more unit is over the limit: dropped.
	p.alice.Send(encodeRawSay2(sayAll, append([]uint16{'x'}, full...)...))
	assertNoSay(t, p.bobby, "line of 101 units")
	assertNoSay(t, p.alice, "speaker of a 101-unit line")
}
