package target

import (
	"slices"
	"testing"
	"time"

	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
)

func TestCorpseMobHandlerCastConditions(t *testing.T) {
	caster := &targetActor{id: 1, category: CategoryPlayable}
	handler := mustHandler(t, NewRegistry(knownList{}), modelskill.TargetCorpseMob)
	now := time.Now()

	tests := []struct {
		name    string
		target  *targetActor
		skill   *modelskill.Definition
		want    bool
		failure CorpseCastFailure
	}{
		{"no corpse", &targetActor{id: 2, category: CategoryAttackable}, &modelskill.Definition{}, false, CorpseCastInvalidTarget},
		{"playable corpse", &targetActor{id: 3, category: CategoryPlayable, corpse: true}, &modelskill.Definition{}, false, CorpseCastInvalidTarget},
		{"mob corpse, default skill", &targetActor{id: 4, category: CategoryAttackable, corpse: true}, &modelskill.Definition{}, true, CorpseCastAllowed},
		{"harvest on monster corpse", &targetActor{id: 5, category: CategoryAttackable, corpse: true, monster: true}, &modelskill.Definition{SkillType: "HARVEST"}, true, CorpseCastAllowed},
		{"harvest on attackable guard corpse", &targetActor{id: 6, category: CategoryAttackable, corpse: true}, &modelskill.Definition{SkillType: "HARVEST"}, false, CorpseCastHarvestNotMonster},
		{"sweep on monster corpse", &targetActor{id: 7, category: CategoryAttackable, corpse: true, monster: true}, &modelskill.Definition{SkillType: "SWEEP"}, true, CorpseCastAllowed},
		{"sweep on attackable guard corpse", &targetActor{id: 8, category: CategoryAttackable, corpse: true}, &modelskill.Definition{SkillType: "SWEEP"}, false, CorpseCastSweepNotMonster},
		{"fresh mob corpse", &targetActor{id: 10, category: CategoryAttackable, corpse: true, corpseDeadline: now.Add(10 * time.Second), corpseTime: 8 * time.Second}, &modelskill.Definition{}, true, CorpseCastAllowed},
		{"too old mob corpse", &targetActor{id: 11, category: CategoryAttackable, corpse: true, corpseDeadline: now.Add(time.Second), corpseTime: 8 * time.Second}, &modelskill.Definition{}, false, CorpseCastTooOld},
		{"spoiled old mob corpse bypasses age cutoff", &targetActor{id: 12, category: CategoryAttackable, corpse: true, corpseDeadline: now.Add(time.Second), corpseTime: 8 * time.Second, spoiled: true}, &modelskill.Definition{}, true, CorpseCastAllowed},
		{"seeded old mob corpse bypasses age cutoff", &targetActor{id: 13, category: CategoryAttackable, corpse: true, corpseDeadline: now.Add(time.Second), corpseTime: 8 * time.Second, seeded: true}, &modelskill.Definition{}, true, CorpseCastAllowed},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := handler.CanCast(caster, tt.target, tt.skill, false); got != tt.want {
				t.Fatalf("CanCast = %v, want %v", got, tt.want)
			}
			if got := CorpseCastFailureFor(tt.target, tt.skill); got != tt.failure {
				t.Fatalf("CorpseCastFailureFor = %v, want %v", got, tt.failure)
			}
		})
	}

	mobCorpse := &targetActor{id: 9, category: CategoryAttackable, corpse: true}
	if got := handler.FinalTarget(caster, mobCorpse, &modelskill.Definition{}); got != mobCorpse {
		t.Fatalf("corpse mob final target = %v, want target", got)
	}
	if got := ids(handler.Targets(caster, mobCorpse, &modelskill.Definition{})); !slices.Equal(got, []int32{9}) {
		t.Fatalf("corpse mob targets = %v, want [9]", got)
	}
}

func TestAreaCorpseMobTargetsSplashAndSpecialSkill(t *testing.T) {
	caster := &targetActor{id: 1, category: CategoryPlayable, x: 0, attackableWithoutForce: true}
	corpse := &targetActor{id: 2, category: CategoryAttackable, corpse: true, x: 100}
	live := &targetActor{id: 3, category: CategoryAttackable, x: 130, attackableWithoutForce: true}
	blocked := &targetActor{id: 4, category: CategoryAttackable, x: 110, attackableWithoutForce: true}
	deadNearby := &targetActor{id: 5, category: CategoryAttackable, x: 120, dead: true, attackableWithoutForce: true}
	corpse.see = map[int32]bool{4: false}

	registry := NewRegistry(knownList{caster, corpse, live, blocked, deadNearby})
	handler := mustHandler(t, registry, modelskill.TargetAreaCorpseMob)

	got := ids(handler.Targets(caster, corpse, &modelskill.Definition{Radius: 100}))
	if want := []int32{2, 3}; !slices.Equal(got, want) {
		t.Fatalf("area corpse mob targets = %v, want %v", got, want)
	}

	got444 := ids(handler.Targets(caster, corpse, &modelskill.Definition{Radius: 100, ID: harvestGrandBoxSkillID}))
	if want := []int32{2, 5}; !slices.Equal(got444, want) {
		t.Fatalf("area corpse mob targets (id 444) = %v, want %v", got444, want)
	}

	if got := handler.FinalTarget(caster, corpse, &modelskill.Definition{}); got != corpse {
		t.Fatalf("area corpse mob final target = %v, want corpse", got)
	}
	if !handler.CanCast(caster, corpse, &modelskill.Definition{}, false) {
		t.Fatal("area corpse mob CanCast on mob corpse = false, want true")
	}
}

func TestCorpsePlayerAndCorpsePetHandlers(t *testing.T) {
	caster := &targetActor{id: 1, category: CategoryPlayable}
	owner := &targetActor{id: 2, category: CategoryPlayable}
	alivePlayer := &targetActor{id: 3, category: CategoryPlayable}
	deadPlayer := &targetActor{id: 4, category: CategoryPlayable, dead: true}
	deadMob := &targetActor{id: 5, category: CategoryAttackable, dead: true}
	deadPet := &targetActor{id: 6, category: CategoryPlayable, dead: true, owner: owner, pet: true}
	deadUnownedPlayable := &targetActor{id: 7, category: CategoryPlayable, dead: true}
	deadServitor := &targetActor{id: 8, category: CategoryPlayable, dead: true, owner: owner}
	deadNoPetSignal := targetCreature{id: 9, category: CategoryPlayable, dead: true}

	registry := NewRegistry(knownList{})

	corpsePlayer := mustHandler(t, registry, modelskill.TargetCorpsePlayer)
	if got := corpsePlayer.FinalTarget(caster, deadPlayer, &modelskill.Definition{}); got != deadPlayer {
		t.Fatalf("corpse player final target = %v, want target", got)
	}
	if got := ids(corpsePlayer.Targets(caster, deadPlayer, &modelskill.Definition{})); !slices.Equal(got, []int32{4}) {
		t.Fatalf("corpse player targets = %v, want [4]", got)
	}
	if !corpsePlayer.CanCast(caster, deadPlayer, &modelskill.Definition{}, false) {
		t.Fatal("corpse player CanCast on dead player = false, want true")
	}
	if corpsePlayer.CanCast(caster, alivePlayer, &modelskill.Definition{}, false) {
		t.Fatal("corpse player CanCast on living player = true, want false")
	}
	if corpsePlayer.CanCast(caster, deadMob, &modelskill.Definition{}, false) {
		t.Fatal("corpse player CanCast on dead mob = true, want false")
	}

	corpsePet := mustHandler(t, registry, modelskill.TargetCorpsePet)
	if got := corpsePet.FinalTarget(caster, deadPet, &modelskill.Definition{}); got != deadPet {
		t.Fatalf("corpse pet final target = %v, want target", got)
	}
	if !corpsePet.CanCast(caster, deadPet, &modelskill.Definition{}, false) {
		t.Fatal("corpse pet CanCast on dead pet = false, want true")
	}
	if corpsePet.CanCast(caster, deadServitor, &modelskill.Definition{}, false) {
		t.Fatal("corpse pet CanCast on dead servitor = true, want false")
	}
	if corpsePet.CanCast(caster, deadNoPetSignal, &modelskill.Definition{}, false) {
		t.Fatal("corpse pet CanCast on dead creature without pet signal = true, want false")
	}
	if corpsePet.CanCast(caster, deadUnownedPlayable, &modelskill.Definition{}, false) {
		t.Fatal("corpse pet CanCast on dead unowned playable = true, want false")
	}
	if corpsePet.CanCast(caster, deadMob, &modelskill.Definition{}, false) {
		t.Fatal("corpse pet CanCast on dead mob = true, want false")
	}
}
