package combat

import (
	"testing"
	"time"

	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/skill/effect"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
)

// TestHostileHealOverTimeTickRefreshesTargeterBar lands a heal-over-time on
// a damaged NPC the player targets. Each tick raises its HP past the next
// health-bar segment, so the targeting player receives the NPC's CUR_HP on
// every tick while a non-targeting observer receives nothing, as the
// reference's HP setter broadcasts through the NPC's status listeners.
func TestHostileHealOverTimeTickRefreshesTargeterBar(t *testing.T) {
	t.Parallel()
	srv := gameservertest.Boot(t, gameservertest.WithCharacter("Newbie", 5, 0), gameservertest.WithWantChars(1))
	c := srv.Client
	startInWorld(t, c)
	w := joinWatcher(t, srv)
	hostile := srv.SpawnHostileNPC(t)
	id := hostile.ObjectID()
	hostile.ConsumeHP(40)
	drainUntilQuiet(t, c)
	drainUntilQuiet(t, w)

	c.Send(encodeAction(id, hostileX, hostileY, hostileZ, false))
	statuses, _ := statusFramesFor(t, c, id)
	assertStatusFrames(t, "target select", statuses, statusFixture(id, wantMaxHP, 440, wantCurHP, 400))
	drainUntilQuiet(t, w)

	e, err := effect.New(effect.Skill{ID: 1220, Level: 1}, modelskill.EffectTemplate{Name: "HealOverTime", Value: 10, Count: 3, Time: 1})
	if err != nil {
		t.Fatalf("effect.New: %v", err)
	}
	e.Effector, e.Effected = hostile, hostile
	done := make(chan struct{})
	if !hostile.Queue().Post(func() { hostile.EffectList().Add(e); close(done) }) {
		t.Fatal("post heal: queue closed")
	}
	<-done
	drainUntilQuiet(t, c)

	for _, want := range []int32{410, 420} {
		srv.Advance(t, 1100*time.Millisecond)
		srv.TickEffects()
		statuses, _ = statusFramesFor(t, c, id)
		assertStatusFrames(t, "heal tick", statuses, statusFixture(id, wantCurHP, want))
		statuses, _ = statusFramesFor(t, w, id)
		assertStatusFrames(t, "heal tick, non-targeting observer", statuses)
	}
}

// TestHostileHealEffectRefreshesTargeterBar lands an instant heal effect on a
// damaged NPC the player targets. The effect restores its amount and then
// adds the applied amount a second time; each restore crosses a health-bar
// segment, so the targeting player receives the NPC's CUR_HP once per
// restore, in order, while a non-targeting observer receives nothing.
func TestHostileHealEffectRefreshesTargeterBar(t *testing.T) {
	t.Parallel()
	srv := gameservertest.Boot(t, gameservertest.WithCharacter("Newbie", 5, 0), gameservertest.WithWantChars(1))
	c := srv.Client
	startInWorld(t, c)
	w := joinWatcher(t, srv)
	hostile := srv.SpawnHostileNPC(t)
	id := hostile.ObjectID()
	hostile.ConsumeHP(40)
	drainUntilQuiet(t, c)
	drainUntilQuiet(t, w)

	c.Send(encodeAction(id, hostileX, hostileY, hostileZ, false))
	statuses, _ := statusFramesFor(t, c, id)
	assertStatusFrames(t, "target select", statuses, statusFixture(id, wantMaxHP, 440, wantCurHP, 400))
	drainUntilQuiet(t, w)

	e, err := effect.New(effect.Skill{ID: 1011, Level: 1}, modelskill.EffectTemplate{Name: "Heal", Value: 10})
	if err != nil {
		t.Fatalf("effect.New: %v", err)
	}
	e.Effector, e.Effected = hostile, hostile
	done := make(chan struct{})
	if !hostile.Queue().Post(func() { hostile.EffectList().Add(e); close(done) }) {
		t.Fatal("post heal: queue closed")
	}
	<-done

	statuses, _ = statusFramesFor(t, c, id)
	assertStatusFrames(t, "heal", statuses, statusFixture(id, wantCurHP, 410), statusFixture(id, wantCurHP, 420))
	statuses, _ = statusFramesFor(t, w, id)
	assertStatusFrames(t, "heal, non-targeting observer", statuses)
}
