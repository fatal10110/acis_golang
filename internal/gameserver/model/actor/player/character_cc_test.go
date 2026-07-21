package player

import (
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/creature"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/skill/effect"
)

type ccGeo struct{}

func (ccGeo) CanMove(_, _, _, _, _, _ int) bool { return true }
func (ccGeo) Height(_, _, _ int) int16          { return 0 }

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

func TestCharacterEffectListAndCrowdControlGettersAreSafeBeforeLiveIsAttached(t *testing.T) {
	c := &Character{ID: 1}
	if c.EffectList() != nil {
		t.Fatal("EffectList() = non-nil before Live is attached")
	}
	if c.Stunned() || c.Rooted() || c.Sleeping() || c.Afraid() || c.Paralyzed() {
		t.Fatal("a crowd-control getter reported true before Live is attached")
	}
}
