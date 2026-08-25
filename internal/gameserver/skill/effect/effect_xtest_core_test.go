package effect_test

import (
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/player"
	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
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
	target := &player.Character{ID: 1}
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
	target := &player.Character{ID: 1}
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
