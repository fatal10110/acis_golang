package lifecycle

import (
	"testing"
	"time"

	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
)

// Boot arms no Seven Signs period-change timer unless a test supplies one: a
// calendar a millisecond short of Monday 18:00 would otherwise change period
// on a real timer in the middle of the entry and push the neutral sound and
// the sky update to the player.
func TestBootDefaultArmsNoPeriodChangeTimer(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, 1, 5, 17, 59, 59, int(999*time.Millisecond), time.Local)
	if now.Weekday() != time.Monday {
		t.Fatalf("fixture date is a %v, want Monday", now.Weekday())
	}
	srv := gameservertest.Boot(t,
		gameservertest.WithCharacter("Newbie", 1, 0),
		gameservertest.WithWantChars(1),
		gameservertest.WithSevenSignsClock(func() time.Time { return now }),
	)

	c := srv.Client
	c.Send(encodeRequestGameStart(0))
	assertFrameOpcode(t, c.Read(), serverpackets.OpcodeSSQInfo, "game start SSQInfo")
	assertFrameOpcode(t, c.Read(), serverpackets.OpcodeCharSelected, "game start CharSelected")
	c.Send(encodeEnterWorld())
	frames := readEnterWorldBurst(t, c)
	frames = append(frames, c.ReadQueued()...)
	for _, f := range frames {
		switch f[0] {
		case serverpackets.OpcodeSSQInfo, serverpackets.OpcodePlaySound:
			t.Fatalf("entry carries a period-change frame %#x", f[0])
		}
	}
	cycle, period, date := statusRow(t, srv.DB)
	if cycle != 1 || period != "COMPETITION" || date != 0 {
		t.Fatalf("status row = (%d, %s, %d), want (1, COMPETITION, 0)", cycle, period, date)
	}
}
