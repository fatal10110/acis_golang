package character

import (
	"testing"
	"time"

	"github.com/fatal10110/acis_golang/internal/gameserver/network"
	"github.com/fatal10110/acis_golang/internal/gameserver/skill/effect"
	"github.com/fatal10110/acis_golang/internal/gameserver/skill/stat"
	"github.com/fatal10110/acis_golang/internal/gameserver/task"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
)

// TestPlayerEnteringShortRegeneratesThreeSecondsAfterEntry pins the start
// of the regeneration task for vitals restored below max: the restore
// starts it, and the earliest point a client can observe is the entry into
// the world, so the first tick lands exactly one period after the entry,
// not whenever a backstop pass first notices the player.
func TestPlayerEnteringShortRegeneratesThreeSecondsAfterEntry(t *testing.T) {
	t.Parallel()
	srv := gameservertest.Boot(t,
		gameservertest.WithCharacter("Newbie", 5, 0),
		gameservertest.WithWantChars(1),
	)
	if !srv.DrivesClock() {
		t.Skip("pinning the regeneration phase needs the driven clock")
	}
	objID := srv.SoleObjectID(t)
	srv.FlushPersistence(t)
	if _, err := srv.DB.Exec("UPDATE characters SET curHp = 10 WHERE obj_Id = ?", objID); err != nil {
		t.Fatalf("seed saved HP: %v", err)
	}
	srv.Client.Send(encodeRequestGameStart(0))
	srv.ReadQueued(t, srv.Client)
	srv.Client.Send(encodeEnterWorld())
	srv.ReadQueued(t, srv.Client)

	obj, ok := srv.State.Player(objID)
	if !ok {
		t.Fatal("player missing from world state")
	}
	c, ok := network.OnlineCharacter(obj)
	if !ok {
		t.Fatalf("player %T is not an online character", obj)
	}
	if got := c.ResourceValues().CurrentHP; got != 10 {
		t.Fatalf("HP after entering = %v, want the saved 10", got)
	}
	if !c.Regen().Active() {
		t.Fatal("regeneration idle after entering the world short of HP")
	}

	regen := task.NewNPCRegen(srv.State)
	hpAfter := func(d time.Duration) float64 {
		t.Helper()
		srv.Advance(t, d)
		regen.Tick()
		srv.Settle(t)
		return c.ResourceValues().CurrentHP
	}
	if got := hpAfter(task.NPCRegenTick - time.Millisecond); got != 10 {
		t.Fatalf("HP 2.999s after entering = %v, want 10: regenerated early", got)
	}
	if got := hpAfter(time.Millisecond); got <= 10 {
		t.Fatalf("HP 3s after entering = %v, want above 10", got)
	}
}

// TestPlayerMaxHPRaisedUnderFullRegeneratesThreeSecondsAfter pins the start
// of the regeneration task when a stat change raises the maximum HP of a
// player at full HP: the raised maximum leaves the player short, and the
// first tick lands exactly one period after the change.
func TestPlayerMaxHPRaisedUnderFullRegeneratesThreeSecondsAfter(t *testing.T) {
	t.Parallel()
	p := bootRegenPhasePlayer(t)
	full := p.c.ResourceValues()

	p.srv.Advance(t, 1200*time.Millisecond)
	p.c.AddStatFuncs([]effect.Mod{{Stat: stat.MaxHP, Op: effect.OpAdd, Value: 50}})
	p.srv.Settle(t)
	if got := p.c.ResourceValues(); got.MaxHP != full.MaxHP+50 || got.CurrentHP != full.CurrentHP {
		t.Fatalf("after the stat change: HP %v/%v, want %v/%v", got.CurrentHP, got.MaxHP, full.CurrentHP, full.MaxHP+50)
	}
	if !p.c.Regen().Active() {
		t.Fatal("regeneration idle after the maximum HP rose above the current")
	}

	if got := p.after(t, task.NPCRegenTick-time.Millisecond); got.CurrentHP != full.CurrentHP {
		t.Fatalf("HP 2.999s after the change = %v, want %v: regenerated early", got.CurrentHP, full.CurrentHP)
	}
	if got := p.after(t, time.Millisecond); got.CurrentHP <= full.CurrentHP {
		t.Fatalf("HP 3s after the change = %v, want above %v", got.CurrentHP, full.CurrentHP)
	}
}

// TestPlayerStatChangeLeavingFullKeepsRegenIdle covers a stat change that
// raises no maximum: a full player stays full and its regeneration task
// stays idle.
func TestPlayerStatChangeLeavingFullKeepsRegenIdle(t *testing.T) {
	t.Parallel()
	p := bootRegenPhasePlayer(t)
	p.c.AddStatFuncs([]effect.Mod{{Stat: stat.PowerAttack, Op: effect.OpAdd, Value: 50}})
	p.srv.Settle(t)
	if p.c.Regen().Active() {
		t.Fatal("regeneration armed by a stat change that left the player full")
	}
	p.after(t, task.NPCRegenTick)
	if p.c.Regen().Active() {
		t.Fatal("regeneration armed by the backstop pass for a full player")
	}
}
