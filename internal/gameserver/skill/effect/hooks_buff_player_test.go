package effect_test

import (
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/player"
	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/skill/effect"
	"github.com/fatal10110/acis_golang/internal/gameserver/world"
)

func TestTargetMeEffectAttacksLivePlayerAlreadyTargetingEffector(t *testing.T) {
	effector := &player.Character{ID: 1, Name: "effector", CharLevel: 1}
	target := &player.Character{ID: 2, Name: "target", CharLevel: 1}
	target.SetTargetTracked(effector)
	var attackedWith world.Tracked
	target.SetAttackTargetHook(func(t world.Tracked) { attackedWith = t })
	e, err := effect.New(effect.Skill{ID: 1}, modelskill.EffectTemplate{Name: "TargetMe"})
	if err != nil {
		t.Fatalf("New() error: %v", err)
	}
	e.Effected, e.Effector = target, effector

	if !e.OnStart(e) {
		t.Fatal("OnStart() = false, want true")
	}
	if attackedWith != effector {
		t.Fatalf("attack target = %v, want effector", attackedWith)
	}
}

// TestIncreaseChargesEffectAddsUpToTemplateCountCap and
// TestIncreaseChargesEffectReportsSuccessEvenAtCap use a real
// *player.Character instead of a hand-rolled fake: the old
// fakeChargesTarget.IncreaseCharges reimplemented the same cap/overflow
// logic already on the real (*player.Character).IncreaseCharges, risking
// silent drift between the two. See docs/agents/test-strategy.md.

func TestIncreaseChargesEffectAddsUpToTemplateCountCap(t *testing.T) {
	target := &player.Character{ID: 1}
	e, err := effect.New(effect.Skill{ID: 1}, modelskill.EffectTemplate{Name: "IncreaseCharges", Value: 1, Count: 7})
	if err != nil {
		t.Fatalf("New() error: %v", err)
	}
	e.Effected = target

	if !e.OnStart(e) {
		t.Fatal("increase charges effect start rejected a valid target")
	}
	if got := target.Charges(); got != 1 {
		t.Fatalf("target.Charges() = %d, want 1", got)
	}
}

func TestIncreaseChargesEffectReportsSuccessEvenAtCap(t *testing.T) {
	target := &player.Character{ID: 1}
	if !target.IncreaseCharges(7, 7) {
		t.Fatal("seed IncreaseCharges(7, 7) rejected, want success reaching the cap")
	}
	e, err := effect.New(effect.Skill{ID: 1}, modelskill.EffectTemplate{Name: "IncreaseCharges", Value: 1, Count: 7})
	if err != nil {
		t.Fatalf("New() error: %v", err)
	}
	e.Effected = target

	if !e.OnStart(e) {
		t.Fatal("increase charges effect start should still report success when the target is already at its cap")
	}
	if got := target.Charges(); got != 7 {
		t.Fatalf("target.Charges() = %d, want unchanged 7", got)
	}
}
