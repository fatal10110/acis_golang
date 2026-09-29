package pets

import (
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/summon"
	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/skill/effect"
)

// Reference: EffectProtectionBlessing.onStart returns false, so its onExit
// (Playable.stopProtectionBlessing -> updateAbnormalEffect, Playable.java:
// 277-285) is skipped when the blessing ends (AbstractEffect.java:319) and
// runs only when a recast displaces it from its stack head via
// setInUse(false) (EffectList.java:756-765). A summon's updateAbnormalEffect
// refreshes its appearance for the owner.

// landPkProtect puts skill 5182's effect as the datapack declares it
// (5100-5199.xml: time 3600, stackType pk_protect, stackOrder 1) on pet, or
// the same template as a plain buff when name is "Buff", and returns it.
func landPkProtect(t *testing.T, h *petWorld, pet *summon.Actor, name string) *effect.Effect {
	t.Helper()
	e, err := effect.New(
		effect.Skill{ID: 5182, Level: 1, SkillType: "BUFF", StackType: "pk_protect"},
		modelskill.EffectTemplate{Name: name, Time: 3600, Count: 1, StackType: "pk_protect", StackOrder: 1, Icon: true},
	)
	if err != nil {
		t.Fatalf("effect.New(%s): %v", name, err)
	}
	e.Effector, e.Effected = pet, pet
	runOn(t, pet.Queue(), func() { pet.EffectList().Add(e) })
	return e
}

// TestProtectionBlessingPetAppearance counts the owner's PetInfo frames when
// a pet's Blessing of Protection ends and when it is recast, against the
// same stack slot held by a plain buff: ending it adds no appearance refresh,
// a recast adds exactly one.
func TestProtectionBlessingPetAppearance(t *testing.T) {
	t.Parallel()
	count := func(t *testing.T, name string, recast bool) int {
		t.Helper()
		h := bootOwnerWithCollar(t)
		pet, _ := h.spawnWolf(t)
		e := landPkProtect(t, h, pet, name)
		drainUntilQuiet(t, h.client)
		if recast {
			landPkProtect(t, h, pet, name)
		} else {
			runOn(t, pet.Queue(), func() { pet.EffectList().Remove(e) })
		}
		got, _ := petInfoCount(drainFrames(t, h.client), pet.ObjectID(), false)
		return got
	}
	for _, tt := range []struct {
		name   string
		recast bool
		extra  int
	}{
		{name: "ended", extra: 0},
		{name: "recast", recast: true, extra: 1},
	} {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			var base, got int
			t.Run("plain buff", func(t *testing.T) { base = count(t, "Buff", tt.recast) })
			t.Run("protection blessing", func(t *testing.T) { got = count(t, "ProtectionBlessing", tt.recast) })
			if got != base+tt.extra {
				t.Fatalf("owner PetInfo frames = %d, want %d (plain buff %d + %d appearance refresh)", got, base+tt.extra, base, tt.extra)
			}
		})
	}
}
