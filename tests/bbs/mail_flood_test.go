package bbs

import (
	"strings"
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
)

// TestMailTooLongCountsAgainstDailyLimit pins a mail too wide to store
// counting against its sender's daily limit even though it files no
// sent-box copy: ten such mails reach their recipient, the eleventh in the
// same day answers NO_MORE_MESSAGES_TODAY and reaches no one. Without it a
// client could repeat an unstored mail without bound, filling the inbox of
// every character in memory.
func TestMailTooLongCountsAgainstDailyLimit(t *testing.T) {
	p := bootPair(t, gameservertest.WithCommunityBoard(boardOn))
	p.enterAll(t)

	long := strings.Repeat("m", 3001)
	for i := range 10 {
		frames := write(t, p.alice, "Mail", "Send", "0", "Bobby", "s", long)
		if len(frames) != 3 {
			t.Fatalf("send %d answer = %x, want the sent box alone", i+1, opcodes(frames))
		}
		assertNewMail(t, drainFrames(t, p.bobby))
	}

	frames := write(t, p.alice, "Mail", "Send", "0", "Bobby", "s", long)
	if len(frames) != 4 {
		t.Fatalf("eleventh send answer = %x, want the refusal then the sent box", opcodes(frames))
	}
	assertSystemMessage(t, frames[0], serverpackets.SystemMessageNoMoreMessagesToday)
	assertSilent(t, p.bobby, "recipient of an unstored mail over the daily limit")
	if got := pageOf(t, command(t, p.bobby, "_bbsmail")); !strings.Contains(got, "in=10 ") {
		t.Fatalf("recipient inbox = %q, want the ten unstored mails listed", got)
	}
	if rows := mailRows(t, p.srv); len(rows) != 0 {
		t.Fatalf("bbs_mail rows = %+v, want none", rows)
	}
}
