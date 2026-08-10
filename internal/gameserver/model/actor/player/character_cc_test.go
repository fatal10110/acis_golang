package player

import (
	"testing"
	"time"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/creature"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/skill/effect"
)

type ccGeo struct{}

func (ccGeo) CanMove(_, _, _, _, _, _ int) bool { return true }
func (ccGeo) Height(_, _, _ int) int16          { return 0 }

// ccGeo never blocks in these tests, so pathfinding and fall-back queries
// never need a useful answer: return no path and reflect the origin.
func (ccGeo) FindPath(_, _ location.Location) ([]location.Location, bool) { return nil, false }
func (ccGeo) ValidLocation(ox, oy, oz, _, _, _ int) location.Location {
	return location.Location{X: ox, Y: oy, Z: oz}
}

// ccFleeTarget satisfies the flee hook a Fear effect's runtime needs, so it
// activates regardless of what its actual effected actor is.
type ccFleeTarget struct{}

func (ccFleeTarget) FleeFrom(effector any, distance int) {}

func attachTestLive(t *testing.T, c *Character) {
	t.Helper()
	live, err := creature.NewLive(location.Location{}, 0, ccGeo{}, c)
	if err != nil {
		t.Fatal(err)
	}
	c.Live = live
}

func addCharacterEffect(t *testing.T, c *Character, name string) *effect.Effect {
	t.Helper()
	e, err := effect.New(effect.Skill{ID: 1}, modelskill.EffectTemplate{Name: name})
	if err != nil {
		t.Fatalf("effect.New(%q) error: %v", name, err)
	}
	e.Effected = ccFleeTarget{}
	c.EffectList().Add(e)
	return e
}

func TestCharacterCrowdControlGettersTrackActiveEffectsAndClearOnRemoval(t *testing.T) {
	tests := []struct {
		name       string
		effectName string
		get        func(*Character) bool
	}{
		{"Stunned", "Stun", (*Character).Stunned},
		{"Rooted", "Root", (*Character).Rooted},
		{"Sleeping", "Sleep", (*Character).Sleeping},
		{"Afraid", "Fear", (*Character).Afraid},
		{"ImmobileUntilAttacked", "ImmobileUntilAttacked", (*Character).ImmobileUntilAttacked},
		{"FakeDead", "FakeDeath", (*Character).FakeDead},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := &Character{ID: 1}
			attachTestLive(t, c)

			if tt.get(c) {
				t.Fatalf("%s() = true before any effect is active", tt.name)
			}

			e := addCharacterEffect(t, c, tt.effectName)
			if !tt.get(c) {
				t.Fatalf("%s() = false with the effect active", tt.name)
			}

			c.EffectList().Remove(e)
			if tt.get(c) {
				t.Fatalf("%s() = true after the effect was removed", tt.name)
			}
		})
	}
}

func TestCharacterThrowUpEffectActivatesAndMovesToLanding(t *testing.T) {
	effector := &Character{ID: 1}
	effector.SetLastKnownPosition(location.Location{X: 100, Y: 0, Z: 0}, 0)
	attachThrowUpTestLive(t, effector)

	effected := &Character{ID: 2}
	effected.SetLastKnownPosition(location.Location{}, 0)
	attachThrowUpTestLive(t, effected)

	e, err := effect.New(effect.Skill{ID: 1, FlyRadius: 600}, modelskill.EffectTemplate{Name: "ThrowUp"})
	if err != nil {
		t.Fatal(err)
	}
	e.Effector, e.Effected = effector, effected
	effected.EffectList().Add(e)
	if !effected.Stunned() {
		t.Fatal("ThrowUp was rejected instead of applying its stunned state")
	}

	effected.EffectList().Remove(e)
	if got := effected.CurrentLocation(); got != (location.Location{X: -600, Y: 0, Z: 0}) {
		t.Fatalf("landing = %+v, want {-600 0 0}", got)
	}
}

func attachThrowUpTestLive(t *testing.T, c *Character) {
	t.Helper()
	live, err := creature.NewLive(location.Location{}, 100, permissiveGeo{}, c)
	if err != nil {
		t.Fatal(err)
	}
	c.Live = live
}

func TestCharacterParalyzedUnionsManualLockAndActiveEffect(t *testing.T) {
	c := &Character{ID: 1}
	attachTestLive(t, c)

	if c.Paralyzed() {
		t.Fatal("Paralyzed() = true on a fresh character")
	}
	if !c.SetParalyzed(true) {
		t.Fatal("SetParalyzed(true) reported no change")
	}
	if !c.Paralyzed() {
		t.Fatal("Paralyzed() = false with only the manual lock set, want true (OR-union)")
	}

	c.SetParalyzed(false)
	if c.Paralyzed() {
		t.Fatal("Paralyzed() = true after the manual lock was cleared and no effect is active")
	}

	e := addCharacterEffect(t, c, "Paralyze")
	if !c.Paralyzed() {
		t.Fatal("Paralyzed() = false with an active paralyze effect and no manual lock")
	}
	c.EffectList().Remove(e)
	if c.Paralyzed() {
		t.Fatal("Paralyzed() = true after the paralyze effect was removed")
	}
}

func TestCharacterAlikeDeadUnionsRealDeathAndFakeDeath(t *testing.T) {
	c := &Character{ID: 1}
	attachTestLive(t, c)
	c.maxHP = 100
	c.curHP = 100

	if c.AlikeDead() {
		t.Fatal("AlikeDead() = true on a fresh character")
	}

	e := addCharacterEffect(t, c, "FakeDeath")
	if !c.AlikeDead() {
		t.Fatal("AlikeDead() = false with an active fake-death effect, want true")
	}

	c.EffectList().Remove(e)
	if c.AlikeDead() {
		t.Fatal("AlikeDead() = true after the fake-death effect was removed")
	}

	c.MarkDead()
	if !c.AlikeDead() {
		t.Fatal("AlikeDead() = false on a really-dead character, want true")
	}
}

func TestCharacterRecentFakeDeathTracksGracePeriodAfterMarking(t *testing.T) {
	c := &Character{ID: 1}
	attachTestLive(t, c)

	if c.RecentFakeDeath() {
		t.Fatal("RecentFakeDeath() = true before MarkRecentFakeDeath was ever called")
	}

	c.MarkRecentFakeDeath()
	if !c.RecentFakeDeath() {
		t.Fatal("RecentFakeDeath() = false right after MarkRecentFakeDeath, want true")
	}

	c.recentFakeDeathUntil = time.Now().Add(-time.Second)
	if c.RecentFakeDeath() {
		t.Fatal("RecentFakeDeath() = true after the grace period elapsed")
	}
}

// TestCharacterClearRecentFakeDeathCancelsGrace matches
// Player.clearRecentFakeDeath() zeroing `_recentFakeDeathEndTime`
// (Player.java:2130-2133), called unconditionally from
// PlayerAttack.doAttack (PlayerAttack.java:23) and PlayerCast.doCast
// (PlayerCast.java:184): an attack or completed cast cancels the grace
// immediately instead of letting it run out on its own.
func TestCharacterClearRecentFakeDeathCancelsGrace(t *testing.T) {
	c := &Character{ID: 1}
	attachTestLive(t, c)

	c.MarkRecentFakeDeath()
	if !c.RecentFakeDeath() {
		t.Fatal("RecentFakeDeath() = false right after MarkRecentFakeDeath, want true")
	}

	c.ClearRecentFakeDeath()
	if c.RecentFakeDeath() {
		t.Fatal("RecentFakeDeath() = true after ClearRecentFakeDeath, want false")
	}
}

func TestCharacterEffectListAndCrowdControlGettersAreSafeBeforeLiveIsAttached(t *testing.T) {
	c := &Character{ID: 1}
	if c.EffectList() != nil {
		t.Fatal("EffectList() = non-nil before Live is attached")
	}
	if c.Stunned() || c.Rooted() || c.Sleeping() || c.Afraid() || c.ImmobileUntilAttacked() || c.Paralyzed() || c.Teleporting() {
		t.Fatal("a crowd-control getter reported true before Live is attached")
	}
}

func TestCharacterTeleportingReportsLiveState(t *testing.T) {
	c := &Character{ID: 1}
	attachTestLive(t, c)

	if c.Teleporting() {
		t.Fatal("Teleporting() = true on a fresh character")
	}
	if !c.SetTeleporting(true) {
		t.Fatal("SetTeleporting(true) reported no change")
	}
	if !c.Teleporting() {
		t.Fatal("Teleporting() = false after SetTeleporting(true)")
	}
	if !c.SetTeleporting(false) {
		t.Fatal("SetTeleporting(false) reported no change")
	}
	if c.Teleporting() {
		t.Fatal("Teleporting() = true after SetTeleporting(false)")
	}
}
