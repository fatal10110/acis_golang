package target

import (
	"slices"
	"testing"

	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
)

func TestAreaTargetsAnchorOnAimedTarget(t *testing.T) {
	caster := &targetActor{id: 1, category: CategoryPlayable, x: 0, y: 0, attackableWithoutForce: true}
	aimed := &targetActor{id: 2, category: CategoryAttackable, x: 100, y: 0, attackableWithoutForce: true}
	near := &targetActor{id: 3, category: CategoryAttackable, x: 150, y: 0, attackableWithoutForce: true}
	dead := &targetActor{id: 4, category: CategoryAttackable, x: 120, y: 0, dead: true, attackableWithoutForce: true}
	blocked := &targetActor{id: 5, category: CategoryAttackable, x: 130, y: 0, attackableWithoutForce: true}
	far := &targetActor{id: 6, category: CategoryAttackable, x: 260, y: 0, attackableWithoutForce: true}
	aimed.see = map[int32]bool{5: false}

	registry := NewRegistry(knownList{caster, aimed, near, dead, blocked, far})
	area := mustHandler(t, registry, modelskill.TargetArea)

	got := ids(area.Targets(caster, aimed, &modelskill.Definition{Radius: 100}))
	if want := []int32{2, 3}; !slices.Equal(got, want) {
		t.Fatalf("area targets = %v, want %v", got, want)
	}

	if final := area.FinalTarget(caster, caster, &modelskill.Definition{}); final != nil {
		t.Fatalf("area final target on self = %v, want nil", final)
	}
}

func TestAuraTargetsFilterBySightAndAttackability(t *testing.T) {
	caster := &targetActor{id: 1, category: CategoryPlayable}
	attackable := &targetActor{id: 2, category: CategoryAttackable, x: 80, attackableWithoutForce: true}
	playable := &targetActor{id: 3, category: CategoryPlayable, x: 90, attackableWithoutForce: true}
	passive := &targetActor{id: 4, category: CategoryAttackable, x: 70}
	dead := &targetActor{id: 5, category: CategoryAttackable, x: 60, dead: true, attackableWithoutForce: true}
	blocked := &targetActor{id: 6, category: CategoryAttackable, x: 50, attackableWithoutForce: true}
	caster.see = map[int32]bool{6: false}

	registry := NewRegistry(knownList{caster, attackable, playable, passive, dead, blocked})
	aura := mustHandler(t, registry, modelskill.TargetAura)

	got := ids(aura.Targets(caster, nil, &modelskill.Definition{Radius: 100}))
	if want := []int32{2, 3}; !slices.Equal(got, want) {
		t.Fatalf("aura targets = %v, want %v", got, want)
	}
}

func TestAuraUndeadTargetsFilterByUndeadSightAndAttackability(t *testing.T) {
	caster := &targetActor{id: 1, category: CategoryPlayable}
	undeadMonster := &targetActor{id: 2, category: CategoryAttackable, x: 80, undead: true, attackableWithoutForce: true}
	undeadPlayable := &targetActor{id: 3, category: CategoryPlayable, x: 90, undead: true, attackableWithoutForce: true}
	living := &targetActor{id: 4, category: CategoryAttackable, x: 70, attackableWithoutForce: true}
	dead := &targetActor{id: 5, category: CategoryAttackable, x: 60, dead: true, undead: true, attackableWithoutForce: true}
	blocked := &targetActor{id: 6, category: CategoryAttackable, x: 50, undead: true, attackableWithoutForce: true}
	passive := &targetActor{id: 7, category: CategoryAttackable, x: 40, undead: true}
	caster.see = map[int32]bool{6: false}

	registry := NewRegistry(knownList{caster, undeadMonster, undeadPlayable, living, dead, blocked, passive})
	auraUndead := mustHandler(t, registry, modelskill.TargetAuraUndead)

	if got := auraUndead.FinalTarget(caster, nil, &modelskill.Definition{}); got != caster {
		t.Fatalf("aura undead final target = %v, want caster", got)
	}
	got := ids(auraUndead.Targets(caster, nil, &modelskill.Definition{Radius: 100}))
	if want := []int32{2, 3}; !slices.Equal(got, want) {
		t.Fatalf("aura undead targets = %v, want %v", got, want)
	}

	if !auraUndead.CanCast(caster, nil, &modelskill.Definition{Offensive: true}, false) {
		t.Fatal("aura undead CanCast outside peace zone = false, want true")
	}
	caster.peace = true
	if auraUndead.CanCast(caster, nil, &modelskill.Definition{Offensive: true}, false) {
		t.Fatal("aura undead CanCast in peace zone with offensive skill = true, want false")
	}
	if !auraUndead.CanCast(caster, nil, &modelskill.Definition{}, false) {
		t.Fatal("aura undead CanCast in peace zone with non-offensive skill = false, want true")
	}
}

func TestAuraHandlersRejectPeaceZoneCastsLikeJava(t *testing.T) {
	caster := &targetActor{id: 1, category: CategoryPlayable, peace: true}
	registry := NewRegistry(knownList{caster})

	for _, target := range []modelskill.Target{modelskill.TargetAura, modelskill.TargetFrontAura} {
		handler := mustHandler(t, registry, target)
		if handler.CanCast(caster, nil, &modelskill.Definition{Offensive: true}, false) {
			t.Fatalf("%s CanCast in peace zone with offensive skill = true, want false", target)
		}
		if !handler.CanCast(caster, nil, &modelskill.Definition{}, false) {
			t.Fatalf("%s CanCast in peace zone with non-offensive skill = false, want true", target)
		}
	}

	behind := mustHandler(t, registry, modelskill.TargetBehindAura)
	if behind.CanCast(caster, nil, &modelskill.Definition{}, false) {
		t.Fatal("behind aura CanCast in peace zone = true, want false")
	}
}

func TestFrontAndBehindAuraDoNotAffectPlayableTargetsForFolkCaster(t *testing.T) {
	caster := &targetActor{id: 1, category: CategoryFolk, heading: 0}
	front := &targetActor{id: 2, category: CategoryPlayable, x: 80}
	behind := &targetActor{id: 3, category: CategoryPlayable, x: -80}
	registry := NewRegistry(knownList{caster, front, behind})

	if got := ids(mustHandler(t, registry, modelskill.TargetFrontAura).Targets(caster, nil, &modelskill.Definition{Radius: 100})); len(got) != 0 {
		t.Fatalf("front aura folk targets = %v, want none", got)
	}
	if got := ids(mustHandler(t, registry, modelskill.TargetBehindAura).Targets(caster, nil, &modelskill.Definition{Radius: 100})); len(got) != 0 {
		t.Fatalf("behind aura folk targets = %v, want none", got)
	}
}

func TestFrontAndBehindAurasUseCasterHeading(t *testing.T) {
	caster := &targetActor{id: 1, category: CategoryPlayable, heading: 0}
	front := &targetActor{id: 2, category: CategoryAttackable, x: 80, attackableWithoutForce: true}
	behind := &targetActor{id: 3, category: CategoryAttackable, x: -80, attackableWithoutForce: true}
	side := &targetActor{id: 4, category: CategoryAttackable, y: 80, attackableWithoutForce: true}

	registry := NewRegistry(knownList{caster, front, behind, side})

	gotFront := ids(mustHandler(t, registry, modelskill.TargetFrontAura).Targets(caster, nil, &modelskill.Definition{Radius: 100}))
	if want := []int32{2}; !slices.Equal(gotFront, want) {
		t.Fatalf("front aura targets = %v, want %v", gotFront, want)
	}

	gotBehind := ids(mustHandler(t, registry, modelskill.TargetBehindAura).Targets(caster, nil, &modelskill.Definition{Radius: 100}))
	if want := []int32{3}; !slices.Equal(gotBehind, want) {
		t.Fatalf("behind aura targets = %v, want %v", gotBehind, want)
	}
}

func TestFrontAreaKeepsAimedTargetAndFiltersSplashByCasterHeading(t *testing.T) {
	caster := &targetActor{id: 1, category: CategoryPlayable, heading: 0}
	aimed := &targetActor{id: 2, category: CategoryAttackable, x: 100, attackableWithoutForce: true}
	front := &targetActor{id: 3, category: CategoryAttackable, x: 130, attackableWithoutForce: true}
	behind := &targetActor{id: 4, category: CategoryAttackable, x: -10, attackableWithoutForce: true}

	registry := NewRegistry(knownList{caster, aimed, front, behind})
	frontArea := mustHandler(t, registry, modelskill.TargetFrontArea)

	got := ids(frontArea.Targets(caster, aimed, &modelskill.Definition{Radius: 100}))
	if want := []int32{2, 3}; !slices.Equal(got, want) {
		t.Fatalf("front area targets = %v, want %v", got, want)
	}
}
