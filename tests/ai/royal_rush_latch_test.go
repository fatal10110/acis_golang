package ai

import (
	"slices"
	"testing"
	"time"

	"github.com/fatal10110/acis_golang/internal/commons/wire"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/npc"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/clientpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/script"
)

// TestRoyalRushNpcStepsSurviveASkippedTick pins #5136: a manager whose
// once-a-second reading of the clock skips second 0 of minute 50, or of
// minute 55, still runs that minute's step once, at its first reading of
// the minute: the entry-closing shout and the escape teleport, then the two
// closing lines. The later readings of the minute repeat nothing.
func TestRoyalRushNpcStepsSurviveASkippedTick(t *testing.T) {
	t.Parallel()
	w, escape := bootSepulcher(t)
	needsDrivenClock(t, w.srv)
	c, _ := w.place(t, ind8ManagerID, 100, 0)
	manager := c.(*npc.Folk)
	start := w.now(t)

	// stall holds back the manager's next reading of the clock by one
	// second, as a late tick does: the reading due in second 0 of the next
	// minute lands in second 1 instead, and the readings go on every second
	// from there.
	stall := func() {
		t.Helper()
		if !w.srv.RunScript(t, w.player.ObjectID(), "RoyalRushNpc", func(s *script.Script, _ *script.Player) {
			self := script.NPCOf(manager)
			s.CancelTimer("3000", self, nil)
			s.StartTimerAtFixedRate("3000", self, nil, 2*time.Second, time.Second)
		}) {
			t.Fatal("the stall panicked")
		}
	}

	w.toTick(t, start, 49, 59)
	stall()
	w.srv.Advance(t, time.Second)
	if got := ind8Shouts(w.srv.ReadQueued(t, w.srv.Client), manager.ObjectID()); len(got) != 0 {
		t.Fatalf("shouts with no reading in second 0 of minute 50 = %q, want none yet", got)
	}
	if now := w.now(t); now.Minute() != 50 || now.Second() != 0 {
		t.Fatalf("the skipped reading's time = %v, want in second 0 of minute 50", now)
	}
	w.srv.Advance(t, time.Second)
	frames := w.srv.ReadQueued(t, w.srv.Client)
	if got := ind8Shouts(frames, manager.ObjectID()); !slices.Equal(got, []string{script.NpcString(1000457)}) {
		t.Fatalf("shouts at the first reading of minute 50, in its second 1 = %q, want the 1000457 line", got)
	}
	w.srv.Settle(t)
	if x, y, _ := w.srv.PlayerPosition(t, w.player.ObjectID()); (location.Location{X: x, Y: y}).Distance2D(location.Location{X: escape.X, Y: escape.Y}) > 150 {
		t.Fatalf("the character stands at %d, %d after minute 50, want within 100 of the escape point %+v", x, y, escape)
	}
	w.srv.Client.Send(wire.NewPacketWriter(clientpackets.OpcodeAppearing).Bytes())
	w.srv.Settle(t)
	w.srv.ReadQueued(t, w.srv.Client)

	w.srv.Advance(t, 58*time.Second)
	if got := ind8Shouts(w.srv.ReadQueued(t, w.srv.Client), manager.ObjectID()); len(got) != 0 {
		t.Fatalf("shouts later in minute 50 = %q, want none: the step ran", got)
	}

	now := w.now(t)
	w.srv.Advance(t, now.Truncate(time.Hour).Add(54*time.Minute+59*time.Second+time.Duration(now.Nanosecond())).Sub(now))
	w.srv.ReadQueued(t, w.srv.Client)
	stall()
	w.srv.Advance(t, 2*time.Second)
	frames = w.srv.ReadQueued(t, w.srv.Client)
	if got := ind8Shouts(frames, manager.ObjectID()); !slices.Equal(got, []string{script.NpcString(1000500), script.NpcString(1000501)}) {
		t.Fatalf("shouts at the first reading of minute 55, in its second 1 = %q, want the 1000500 and 1000501 lines", got)
	}
	w.srv.Advance(t, 58*time.Second)
	if got := ind8Shouts(w.srv.ReadQueued(t, w.srv.Client), manager.ObjectID()); len(got) != 0 {
		t.Fatalf("shouts later in minute 55 = %q, want none: the step ran", got)
	}
}
