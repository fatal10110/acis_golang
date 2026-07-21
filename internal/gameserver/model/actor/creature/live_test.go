package creature

import (
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/skill/effect"
)

type liveGeo struct {
	canMove bool
	height  int16
}

func (g liveGeo) CanMove(_, _, _, _, _, _ int) bool { return g.canMove }
func (g liveGeo) Height(_, _, _ int) int16          { return g.height }

// liveGeo does not exercise pathfinding or fall-back resolution: tests that
// build it either walk a clear line or simulate a fully blocked one.
func (g liveGeo) FindPath(_, _ location.Location) ([]location.Location, bool) { return nil, false }
func (g liveGeo) ValidLocation(ox, oy, oz, _, _, _ int) location.Location {
	return location.Location{X: ox, Y: oy, Z: oz}
}

func TestLiveOwnsOneMovementState(t *testing.T) {
	live, err := NewLive(location.Location{X: 10, Y: 20, Z: 30}, 50, liveGeo{canMove: true, height: 30}, nil)
	if err != nil {
		t.Fatal(err)
	}

	first := live.Move()
	if first != &live.movement {
		t.Fatal("Move() does not return the embedded movement state")
	}

	if _, err := first.MoveToLocation(location.Location{X: 60, Y: 20, Z: 999}); err != nil {
		t.Fatal(err)
	}
	second := live.Move()
	if second != first {
		t.Fatal("Move() returned a different movement state")
	}
	if got := second.Destination(); got != (location.Location{X: 60, Y: 20, Z: 30}) {
		t.Fatalf("Destination() = %+v, want the accepted target", got)
	}

	if _, err := second.MoveToLocation(location.Location{X: 70, Y: 20, Z: 999}); err != nil {
		t.Fatal(err)
	}
	if live.Move() != first {
		t.Fatal("repeated movement replaced the embedded movement state")
	}
	if got := first.Destination(); got != (location.Location{X: 70, Y: 20, Z: 30}) {
		t.Fatalf("Destination() = %+v, want the second accepted target", got)
	}
}

func TestLiveMovementStateIsPerCreature(t *testing.T) {
	geo := liveGeo{canMove: true, height: 30}
	first, err := NewLive(location.Location{X: 0, Y: 0, Z: 30}, 100, geo, nil)
	if err != nil {
		t.Fatal(err)
	}
	second, err := NewLive(location.Location{X: 100, Y: 0, Z: 30}, 100, geo, nil)
	if err != nil {
		t.Fatal(err)
	}

	if first.Move() == second.Move() {
		t.Fatal("two live creatures share movement state")
	}
	if _, err := first.Move().MoveToLocation(location.Location{X: 50, Y: 0, Z: 999}); err != nil {
		t.Fatal(err)
	}
	if _, err := second.Move().MoveToLocation(location.Location{X: 150, Y: 0, Z: 999}); err != nil {
		t.Fatal(err)
	}

	if got := first.Move().Destination(); got != (location.Location{X: 50, Y: 0, Z: 30}) {
		t.Fatalf("first Destination() = %+v, want its own target", got)
	}
	if got := second.Move().Destination(); got != (location.Location{X: 150, Y: 0, Z: 30}) {
		t.Fatalf("second Destination() = %+v, want its own target", got)
	}
}

func newTestLive(t *testing.T) *Live {
	t.Helper()
	live, err := NewLive(location.Location{}, 0, liveGeo{canMove: true}, nil)
	if err != nil {
		t.Fatal(err)
	}
	return live
}

// ccTestTarget satisfies every optional target interface a core effect's
// hooks type-assert against, so the effect always activates regardless of
// which one is under test.
type ccTestTarget struct{}

func (ccTestTarget) FleeFrom(effector any, distance int) {}

func addTestEffect(t *testing.T, live *Live, name string) *effect.Effect {
	t.Helper()
	e, err := effect.New(effect.Skill{ID: 1}, modelskill.EffectTemplate{Name: name})
	if err != nil {
		t.Fatalf("effect.New(%q) error: %v", name, err)
	}
	e.Effected = ccTestTarget{}
	live.EffectList().Add(e)
	return e
}

func TestLiveCrowdControlGettersTrackActiveEffectsAndClearOnRemoval(t *testing.T) {
	tests := []struct {
		name       string
		effectName string
		get        func(*Live) bool
	}{
		{"Stunned", "Stun", (*Live).Stunned},
		{"Rooted", "Root", (*Live).Rooted},
		{"Sleeping", "Sleep", (*Live).Sleeping},
		{"Afraid", "Fear", (*Live).Afraid},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			live := newTestLive(t)
			if tt.get(live) {
				t.Fatalf("%s() = true before any effect is active", tt.name)
			}

			e := addTestEffect(t, live, tt.effectName)
			if !tt.get(live) {
				t.Fatalf("%s() = false with the effect active", tt.name)
			}

			live.EffectList().Remove(e)
			if tt.get(live) {
				t.Fatalf("%s() = true after the effect was removed", tt.name)
			}
		})
	}
}

func TestLiveParalyzedUnionsManualLockAndActiveEffect(t *testing.T) {
	live := newTestLive(t)
	if live.Paralyzed() {
		t.Fatal("Paralyzed() = true on a fresh creature")
	}

	if !live.SetParalyzed(true) {
		t.Fatal("SetParalyzed(true) reported no change on first call")
	}
	if !live.Paralyzed() {
		t.Fatal("Paralyzed() = false with only the manual lock set, want true (OR-union)")
	}
	if live.SetParalyzed(true) {
		t.Fatal("SetParalyzed(true) reported a change on a no-op call")
	}

	if !live.SetParalyzed(false) {
		t.Fatal("SetParalyzed(false) reported no change")
	}
	if live.Paralyzed() {
		t.Fatal("Paralyzed() = true after the manual lock was cleared and no effect is active")
	}

	e := addTestEffect(t, live, "Paralyze")
	if !live.Paralyzed() {
		t.Fatal("Paralyzed() = false with an active paralyze effect and no manual lock")
	}

	live.EffectList().Remove(e)
	if live.Paralyzed() {
		t.Fatal("Paralyzed() = true after the paralyze effect was removed")
	}
}

func TestLiveImmobilizedReportsChange(t *testing.T) {
	live := newTestLive(t)
	if live.Immobilized() {
		t.Fatal("Immobilized() = true on a fresh creature")
	}

	if !live.SetImmobilized(true) {
		t.Fatal("SetImmobilized(true) reported no change on first call")
	}
	if !live.Immobilized() {
		t.Fatal("Immobilized() = false after SetImmobilized(true)")
	}
	if live.SetImmobilized(true) {
		t.Fatal("SetImmobilized(true) reported a change on a no-op call")
	}

	if !live.SetImmobilized(false) {
		t.Fatal("SetImmobilized(false) reported no change")
	}
	if live.Immobilized() {
		t.Fatal("Immobilized() = true after SetImmobilized(false)")
	}
}

func TestLiveNilReceiverGettersDoNotPanic(t *testing.T) {
	var live *Live

	if live.EffectList() != nil {
		t.Fatal("EffectList() on a nil receiver = non-nil")
	}
	if live.Stunned() || live.Rooted() || live.Sleeping() || live.Afraid() || live.Paralyzed() || live.Immobilized() {
		t.Fatal("a crowd-control getter on a nil receiver reported true")
	}
	if live.SetParalyzed(true) || live.SetImmobilized(true) {
		t.Fatal("a crowd-control setter on a nil receiver reported a change")
	}
}
