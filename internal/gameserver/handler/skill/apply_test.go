package skill

import (
	"testing"

	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/skill/effect"
)

func TestActiveEffectFindsAMatchingLiveInstance(t *testing.T) {
	target := newCancelFakeActor(10)
	addBuff(t, target, modelskill.EffectTemplate{Name: "Buff", Time: 60}, effect.Skill{ID: 288})

	if !ActiveEffect(target, 288) {
		t.Fatal("ActiveEffect() = false, want true for a live instance of skill 288")
	}
	if ActiveEffect(target, 99) {
		t.Fatal("ActiveEffect() = true, want false for a skill id with no live instance")
	}
}

func TestActiveEffectOnATargetWithNoEffectListIsFalse(t *testing.T) {
	if ActiveEffect(struct{}{}, 288) {
		t.Fatal("ActiveEffect() = true, want false for a target with no effect list")
	}
}

func TestStopEffectRemovesTheMatchingLiveInstance(t *testing.T) {
	target := newCancelFakeActor(10)
	e := addBuff(t, target, modelskill.EffectTemplate{Name: "Buff", Time: 60}, effect.Skill{ID: 288})
	addBuff(t, target, modelskill.EffectTemplate{Name: "Buff", Time: 60}, effect.Skill{ID: 4})

	StopEffect(target, 288)

	if hasEffect(target.list, e) {
		t.Fatal("skill 288's effect is still active after StopEffect")
	}
	if !ActiveEffect(target, 4) {
		t.Fatal("StopEffect removed an unrelated skill's active effect")
	}
}

func TestStopEffectOnATargetWithNoEffectListIsANoop(t *testing.T) {
	StopEffect(struct{}{}, 288)
}
