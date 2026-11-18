package skills

import (
	"testing"
	"time"

	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
)

// TestDamageOverTimeTicksAtExactDueInstants lays a three-count, one-second
// damage-over-time on the fixture monster. The first tick is due one period
// after the effect starts and every later one a period after the previous:
// nothing happens a millisecond early, each due instant takes exactly one
// tick off, and the effect ends with the tick that spends its last count.
func TestDamageOverTimeTicksAtExactDueInstants(t *testing.T) {
	t.Parallel()
	const skillID = 43
	srv := gameservertest.Boot(t,
		gameservertest.WithCharacter("Newbie", 5, 0),
		gameservertest.WithWantChars(1),
		gameservertest.WithSkills(skillPersistence(t, []modelskill.Definition{{
			ID: skillID, Level: 1, Activation: modelskill.ActivationActive, Target: modelskill.TargetOne,
			CastRange: 900, HitTime: 0, StaticHitTime: true,
			SkillType: "DEBUFF", EffectType: "DEBUFF", Debuff: true,
			EffectPower: 100, IgnoreResists: true,
			Effects: []modelskill.EffectTemplate{{Name: "DamOverTime", Value: 100, Count: 3, Time: 1}},
		}})),
	)
	if !srv.DrivesClock() {
		t.Skip("one-millisecond due instants need the driven clock")
	}
	c, objID := srv.Client, srv.SoleObjectID(t)
	seedKnownSkill(t, srv, objID, skillID, 1)
	startInWorld(t, c)
	hostile := srv.SpawnHostileNPC(t)
	drainUntilQuiet(t, c)
	targetHostile(t, c, hostile.ObjectID())
	drainUntilQuiet(t, c)

	c.Send(encodeRequestMagicSkillUse(skillID, false, false))
	drainUntilQuiet(t, c)
	if got := len(hostile.EffectList().All()); got != 1 {
		t.Fatalf("monster effects after the cast = %d, want the damage over time", got)
	}
	start := hostile.CurrentHP()

	step := func(d time.Duration, wantTicks, wantEffects int) {
		t.Helper()
		srv.Advance(t, d)
		srv.TickEffects()
		if got, want := start-hostile.CurrentHP(), 100*wantTicks; got != want {
			t.Fatalf("HP lost = %d, want %d (%d ticks)", got, want, wantTicks)
		}
		if got := len(hostile.EffectList().All()); got != wantEffects {
			t.Fatalf("effects = %d, want %d", got, wantEffects)
		}
	}
	step(999*time.Millisecond, 0, 1)
	step(time.Millisecond, 1, 1)
	step(999*time.Millisecond, 1, 1)
	step(time.Millisecond, 2, 1)
	step(999*time.Millisecond, 2, 1)
	step(time.Millisecond, 3, 0)
}
