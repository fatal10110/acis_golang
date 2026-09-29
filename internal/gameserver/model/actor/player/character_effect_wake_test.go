package player

import (
	"testing"
	"time"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/creature"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/event"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/sim"
	"github.com/fatal10110/acis_golang/internal/gameserver/skill/effect"
)

// wakeTestCharacter is a live character on an inline queue whose clock the
// test drives, with its events recorded.
func wakeTestCharacter(t *testing.T) (*Character, *sim.Inline, *event.Recorder) {
	t.Helper()
	in := sim.NewInline(time.Unix(0, 0))
	c := &Character{ID: 1}
	live, err := creature.NewLive(location.Location{}, 0, ccGeo{}, c)
	if err != nil {
		t.Fatal(err)
	}
	live.SetQueue(in.NewQueue("wake"))
	c.Live = live
	return c, in, recordEvents(c)
}

func landSelfEffect(t *testing.T, c *Character, name string, seconds int) *effect.Effect {
	t.Helper()
	e, err := effect.New(effect.Skill{ID: 441, Level: 1}, modelskill.EffectTemplate{Name: name, Time: seconds})
	if err != nil {
		t.Fatalf("effect.New(%s) error = %v", name, err)
	}
	e.Effector, e.Effected = c, c
	c.EffectList().Add(e)
	if !e.InUse() {
		t.Fatalf("%s did not start on the character", name)
	}
	return e
}

// TestImmobileUntilAttackedEndWakesPlayerAI pins
// EffectImmobileUntilAttacked.onExit, which notifies THINK on the effected
// whatever its kind: taking damage (which stops the effect by type) wakes the
// player's AI once, and applying the effect does not.
func TestImmobileUntilAttackedEndWakesPlayerAI(t *testing.T) {
	c, _, rec := wakeTestCharacter(t)
	landSelfEffect(t, c, "ImmobileUntilAttacked", 30)
	if got := event.Count[event.ThinkRequested](rec); got != 0 {
		t.Fatalf("ThinkRequested after applying = %d, want 0", got)
	}

	c.EffectList().StopByType(effect.TypeImmobileUntilAttacked)
	if got := event.Count[event.ThinkRequested](rec); got != 1 {
		t.Fatalf("ThinkRequested after the effect was broken = %d, want 1", got)
	}
	if c.ImmobileUntilAttacked() {
		t.Fatal("ImmobileUntilAttacked() = true after the effect was broken")
	}
}

// TestImmobileUntilAttackedExpiryWakesPlayerAI pins the effect's own tick,
// which ends it and wakes the player's AI exactly once. The reference's
// onActionTime and onExit both notify THINK; a second think of the same
// unchanged intention resumes nothing more, so the end wakes it once.
func TestImmobileUntilAttackedExpiryWakesPlayerAI(t *testing.T) {
	c, in, rec := wakeTestCharacter(t)
	landSelfEffect(t, c, "ImmobileUntilAttacked", 5)

	in.Advance(6 * time.Second)
	c.EffectList().Tick()
	if got := event.Count[event.ThinkRequested](rec); got != 1 {
		t.Fatalf("ThinkRequested after the effect expired = %d, want 1", got)
	}
	if c.ImmobileUntilAttacked() {
		t.Fatal("ImmobileUntilAttacked() = true after the effect expired")
	}
}

// TestCrowdControlEndDoesNotWakePlayerAI pins the Player exclusion in
// EffectRoot/EffectSleep/EffectParalyze/EffectPetrification.onExit.
func TestCrowdControlEndDoesNotWakePlayerAI(t *testing.T) {
	for _, name := range []string{"Root", "Sleep", "Paralyze", "Petrification"} {
		t.Run(name, func(t *testing.T) {
			c, _, rec := wakeTestCharacter(t)
			e := landSelfEffect(t, c, name, 30)
			c.EffectList().Remove(e)
			if got := event.Count[event.ThinkRequested](rec); got != 0 {
				t.Fatalf("ThinkRequested after %s ended = %d, want 0", name, got)
			}
		})
	}
}

// TestStunSelfSendsPlayerIdle pins EffectStunSelf.onStart, which sends a
// playable effected idle, carrying whether it could act before the stun.
func TestStunSelfSendsPlayerIdle(t *testing.T) {
	c, _, rec := wakeTestCharacter(t)
	landSelfEffect(t, c, "StunSelf", 9)
	idles := event.Of[event.IdleRequested](rec)
	if len(idles) != 1 || idles[0].AIDenied {
		t.Fatalf("IdleRequested = %+v, want one request from a character able to act", idles)
	}

	c2, _, rec2 := wakeTestCharacter(t)
	landSelfEffect(t, c2, "Stun", 9)
	landSelfEffect(t, c2, "StunSelf", 9)
	idles = event.Of[event.IdleRequested](rec2)
	if len(idles) != 2 || idles[0].AIDenied || !idles[1].AIDenied {
		t.Fatalf("IdleRequested = %+v, want the stun's then the already-stunned StunSelf's", idles)
	}
}
