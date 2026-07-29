package target

import (
	"slices"
	"testing"

	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
)

func TestSummonTargetsCasterSummon(t *testing.T) {
	caster := &targetActor{id: 1, category: CategoryPlayable}
	summon := &targetActor{id: 2, category: CategoryPlayable}
	caster.summon = summon

	handler := mustHandler(t, NewRegistry(knownList{caster, summon}), modelskill.TargetSummon)

	if got := handler.FinalTarget(caster, nil, &modelskill.Definition{}); got != summon {
		t.Fatalf("summon final target = %v, want summon", got)
	}
	if got := ids(handler.Targets(caster, nil, &modelskill.Definition{})); !slices.Equal(got, []int32{2}) {
		t.Fatalf("summon targets = %v, want [2]", got)
	}
	if !handler.CanCast(caster, nil, &modelskill.Definition{}, false) {
		t.Fatal("summon CanCast with live summon = false, want true")
	}

	summon.dead = true
	if handler.CanCast(caster, nil, &modelskill.Definition{}, false) {
		t.Fatal("summon CanCast with dead summon = true, want false")
	}
	caster.summon = nil
	if got := handler.FinalTarget(caster, nil, &modelskill.Definition{}); got != nil {
		t.Fatalf("summon final target without summon = %v, want nil", got)
	}
}

func TestOwnerPetTargetsSummonOwner(t *testing.T) {
	owner := &targetActor{id: 1, category: CategoryPlayable}
	summon := &targetActor{id: 2, category: CategoryPlayable, owner: owner}
	other := &targetActor{id: 3, category: CategoryPlayable}

	handler := mustHandler(t, NewRegistry(knownList{owner, summon, other}), modelskill.TargetOwnerPet)

	if got := handler.FinalTarget(summon, nil, &modelskill.Definition{}); got != owner {
		t.Fatalf("owner pet final target = %v, want owner", got)
	}
	if got := ids(handler.Targets(summon, nil, &modelskill.Definition{})); !slices.Equal(got, []int32{1}) {
		t.Fatalf("owner pet targets = %v, want [1]", got)
	}
	if !handler.CanCast(summon, owner, &modelskill.Definition{}, false) {
		t.Fatal("owner pet CanCast on owner = false, want true")
	}
	if handler.CanCast(summon, other, &modelskill.Definition{}, false) {
		t.Fatal("owner pet CanCast on another target = true, want false")
	}

	owner.dead = true
	if handler.CanCast(summon, owner, &modelskill.Definition{}, false) {
		t.Fatal("owner pet CanCast on dead owner = true, want false")
	}
}

func TestAreaSummonUsesSummonAsAnchor(t *testing.T) {
	caster := &targetActor{id: 1, category: CategoryPlayable, x: 0, attackableWithoutForce: true}
	summon := &targetActor{id: 2, category: CategoryPlayable, x: 100}
	near := &targetActor{id: 3, category: CategoryAttackable, x: 130, attackableWithoutForce: true}
	dead := &targetActor{id: 4, category: CategoryAttackable, x: 120, dead: true, attackableWithoutForce: true}
	blocked := &targetActor{id: 5, category: CategoryAttackable, x: 110, attackableWithoutForce: true}
	passive := &targetActor{id: 6, category: CategoryAttackable, x: 115}
	far := &targetActor{id: 7, category: CategoryAttackable, x: 300, attackableWithoutForce: true}
	caster.summon = summon
	summon.see = map[int32]bool{5: false}

	handler := mustHandler(t, NewRegistry(knownList{caster, summon, near, dead, blocked, passive, far}), modelskill.TargetAreaSummon)

	if got := handler.FinalTarget(caster, nil, &modelskill.Definition{}); got != summon {
		t.Fatalf("area summon final target = %v, want summon", got)
	}
	got := ids(handler.Targets(caster, summon, &modelskill.Definition{Radius: 50}))
	if want := []int32{3}; !slices.Equal(got, want) {
		t.Fatalf("area summon targets = %v, want %v", got, want)
	}
}
