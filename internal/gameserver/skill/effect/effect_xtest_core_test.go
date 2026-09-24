package effect_test

import (
	"testing"
	"time"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/creature"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/player"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/sim"
	"github.com/fatal10110/acis_golang/internal/gameserver/skill/effect"
)

// ---- from persist_restore_player_test.go ----
// noopStatOwner satisfies effect.StatOwner without recording anything:
// neither test below asserts on owner-side calls, only on the restored
// effect's own state and the live list it lands in.
type noopStatOwner struct{}

func (noopStatOwner) AddStatFuncs([]effect.Mod)          {}
func (noopStatOwner) RemoveStatsByOwner(effect.ModOwner) {}
func (noopStatOwner) MaxBuffCount() int                  { return 20 }

// TestApplyRestoredDeliversOnStartToLiveEffectList is the regression case
// for the reported gap: a restored effect used to sit inert in
// Persistence's save registry and never reached the live effect list, so
// its OnStart hook (icons, stat application, ExRegenMax, ...) never fired
// on relog. ApplyRestored is what Persistence.ReplayEffects now calls to
// replay it through List.Add like a live cast would. Uses a real
// *player.Character target instead of a hand-rolled fake: the old
// fakeChargesTarget.IncreaseCharges reimplemented the same cap/overflow
// logic already on the real (*player.Character).IncreaseCharges.
func TestApplyRestoredDeliversOnStartToLiveEffectList(t *testing.T) {
	target := inWorldCharacter(t)
	list := effect.NewList(noopStatOwner{})
	meta := effect.Skill{ID: 7, Level: 3}
	templates := []modelskill.EffectTemplate{{Name: "IncreaseCharges", Value: 2, Count: 5}}

	effect.ApplyRestored(list, target, target, meta, templates, 5, 0)

	if got := target.Charges(); got != 2 {
		t.Fatalf("target.Charges() after ApplyRestored = %d, want 2 (OnStart delivered on restore, like a live cast)", got)
	}
	active := list.All()
	if len(active) != 1 || !active[0].InUse() {
		t.Fatal("ApplyRestored effect never became active in the live list")
	}
	if got := active[0].Remaining(); got != 5 {
		t.Fatalf("Remaining() = %d, want 5 (persisted count == template count, no clamp)", got)
	}
}

func TestApplyRestoredSkipsUnsupportedTemplatesWithoutFailingTheRest(t *testing.T) {
	target := inWorldCharacter(t)
	list := effect.NewList(noopStatOwner{})
	meta := effect.Skill{ID: 8}
	templates := []modelskill.EffectTemplate{
		{Name: "not-a-real-effect"},
		{Name: "IncreaseCharges", Value: 1, Count: 3},
	}

	effect.ApplyRestored(list, target, target, meta, templates, 3, 0)

	if len(list.All()) != 1 {
		t.Fatalf("ApplyRestored added %d effects, want 1 (unsupported template skipped)", len(list.All()))
	}
}

func (noopStatOwner) NotifyEffectAborted(modelskill.ID, int) {}

func (noopStatOwner) NotifyEffectDisappeared(modelskill.ID, int) {}

func (noopStatOwner) NotifyEffectWornOff(modelskill.ID, int) {}

func (noopStatOwner) UpdateEffectIcons() {}

// inWorldCharacter is a character attached to a live body on a queue nothing
// advances, so the charge auto-clear timer it arms never fires.
func inWorldCharacter(t *testing.T) *player.Character {
	t.Helper()
	c := &player.Character{ID: 1}
	live, err := creature.NewLive(location.Location{}, 0, openGeo{}, c)
	if err != nil {
		t.Fatal(err)
	}
	live.SetQueue(sim.NewInline(time.Unix(0, 0)).NewQueue("test"))
	c.Live = live
	return c
}

type openGeo struct{}

func (openGeo) CanMove(int, int, int, int, int, int) bool { return true }
func (openGeo) Height(_, _, z int) int16                  { return int16(z) }
func (openGeo) FindPath(_, target location.Location) ([]location.Location, bool) {
	return []location.Location{target}, true
}
func (openGeo) ValidLocation(_, _, _, tx, ty, tz int) location.Location {
	return location.Location{X: tx, Y: ty, Z: tz}
}
func (openGeo) Walkable(int, int, int) bool { return true }
