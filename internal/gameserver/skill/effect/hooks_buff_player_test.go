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
