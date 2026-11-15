package pets

import (
	"testing"

	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/skill/effect"
)

// TestHerbBuffHalvedForPetOwnerNotPet lands a Herb of Power buff (skill
// 2280, 120 s) on a player whose pet is out and on that pet: a pet counts as
// the owner's summon, so the owner's buff runs 60 s, but the pet itself is
// no servitor and keeps the full 120 s.
func TestHerbBuffHalvedForPetOwnerNotPet(t *testing.T) {
	t.Parallel()
	h, wolf, _ := bootWolfStriker(t)
	herb := func(target effect.Actor) *effect.Effect {
		e, err := effect.New(effect.Skill{ID: 2280, Level: 1}, modelskill.EffectTemplate{
			Name: "Buff", Count: 1, Time: 120, Icon: true, StackType: "pa_up", StackOrder: 2.9,
		})
		if err != nil {
			t.Fatalf("effect.New(Buff): %v", err)
		}
		e.Effector, e.Effected = target, target
		return e
	}

	owner := h.character(t)
	var ownerHerb *effect.Effect
	done := make(chan struct{})
	if !owner.Queue().Post(func() {
		ownerHerb = herb(owner)
		owner.EffectList().Add(ownerHerb)
		close(done)
	}) {
		t.Fatal("post owner herb: queue closed")
	}
	<-done
	if got := ownerHerb.Period(); got != 60 {
		t.Errorf("pet owner's herb period = %d s, want the halved 60", got)
	}

	petHerb := herb(wolf)
	done = make(chan struct{})
	if !wolf.Queue().Post(func() { wolf.EffectList().Add(petHerb); close(done) }) {
		t.Fatal("post pet herb: queue closed")
	}
	<-done
	if got := petHerb.Period(); got != 120 {
		t.Errorf("pet's herb period = %d s, want the full 120", got)
	}
}
