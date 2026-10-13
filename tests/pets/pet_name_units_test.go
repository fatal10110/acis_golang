package pets

import (
	"strings"
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
)

// TestRenamePetCountsNameInUTF16Units pins the length gate's unit: a name
// is measured in UTF-16 code units, so a short name of multi-byte or
// unpaired-surrogate characters passes the length gate and is refused by
// the character check instead, while 17 units are too long.
func TestRenamePetCountsNameInUTF16Units(t *testing.T) {
	t.Parallel()
	h := bootOwnerWithCollar(t)
	h.spawnWolf(t)

	for _, tc := range []struct {
		name, in string
		want     int
	}{
		// Nine Cyrillic letters: 9 units, 18 UTF-8 bytes.
		{"cyrillic", "Пушистики", serverpackets.SystemMessageNamingPetnameContainsInvalidChars},
		// Six unpaired D83D units, sent as single units by the test client.
		{"unpaired surrogates", strings.Repeat("\xED\xA0\xBD", 6), serverpackets.SystemMessageNamingPetnameContainsInvalidChars},
		{"seventeen units", strings.Repeat("\xED\xA0\xBD", 17), serverpackets.SystemMessageNamingCharnameUpTo16Chars},
	} {
		frames := sysMessages(renameTo(t, h, tc.in))
		if len(frames) != 1 {
			t.Fatalf("%s: rejections = %d, want exactly one", tc.name, len(frames))
		}
		assertStaticSystemMessage(t, frames[0], tc.want)
	}
	if _, ok, err := h.srv.Pets.Get(petCtx(), h.collarID); ok || err != nil {
		t.Fatalf("pets row after rejections: ok=%v err=%v, want none", ok, err)
	}
}
