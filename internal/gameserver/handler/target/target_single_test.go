package target

import (
	"slices"
	"testing"

	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
)

func TestRegistryRegistersRepresentativeHandlers(t *testing.T) {
	registry := NewRegistry(knownList{})

	for _, typ := range []modelskill.Target{
		modelskill.TargetSelf,
		modelskill.TargetOne,
		modelskill.TargetArea,
		modelskill.TargetFrontArea,
		modelskill.TargetAura,
		modelskill.TargetFrontAura,
		modelskill.TargetBehindAura,
		modelskill.TargetUndead,
		modelskill.TargetAuraUndead,
		modelskill.TargetUnlockable,
		modelskill.TargetHoly,
		modelskill.TargetSummon,
		modelskill.TargetAreaSummon,
		modelskill.TargetOwnerPet,
		modelskill.TargetCorpseMob,
		modelskill.TargetAreaCorpseMob,
		modelskill.TargetCorpsePlayer,
		modelskill.TargetCorpsePet,
		modelskill.TargetGround,
		modelskill.TargetParty,
		modelskill.TargetAlly,
		modelskill.TargetClan,
		modelskill.TargetPartyMember,
		modelskill.TargetPartyOther,
		modelskill.TargetCorpseAlly,
	} {
		if _, ok := registry.Handler(typ); !ok {
			t.Fatalf("Handler(%s) missing", typ)
		}
	}
}

func TestSelfAndOneHandlers(t *testing.T) {
	caster := &targetActor{id: 1, category: CategoryPlayable}
	target := &targetActor{id: 2, category: CategoryAttackable}
	registry := NewRegistry(knownList{})
	skill := &modelskill.Definition{Radius: 100}

	self := mustHandler(t, registry, modelskill.TargetSelf)
	if got := self.FinalTarget(caster, target, skill); got != caster {
		t.Fatalf("self final target = %v, want caster", got)
	}
	if got := ids(self.Targets(caster, target, skill)); !slices.Equal(got, []int32{1}) {
		t.Fatalf("self targets = %v, want [1]", got)
	}

	one := mustHandler(t, registry, modelskill.TargetOne)
	if got := one.FinalTarget(caster, target, skill); got != target {
		t.Fatalf("one final target = %v, want aimed target", got)
	}
	if got := ids(one.Targets(caster, target, skill)); !slices.Equal(got, []int32{2}) {
		t.Fatalf("one targets = %v, want [2]", got)
	}
	if one.CanCast(caster, nil, skill, false) {
		t.Fatal("one CanCast with nil target = true, want false")
	}
}

func TestSingleTargetKindHandlersValidateCastTargets(t *testing.T) {
	caster := &targetActor{id: 1, category: CategoryPlayable}
	holyThing := &targetActor{id: 2, category: CategoryFolk, holy: true}
	lockedDoor := &targetActor{id: 3, category: CategoryFolk}
	unlockableDoor := &targetActor{id: 4, category: CategoryFolk, unlockable: true}
	undeadMonster := &targetActor{id: 5, category: CategoryAttackable, undead: true}
	livingMonster := &targetActor{id: 6, category: CategoryAttackable}
	deadUndead := &targetActor{id: 7, category: CategoryAttackable, dead: true, undead: true}
	undeadServitor := &targetActor{id: 8, category: CategoryPlayable, owner: caster, undead: true}
	registry := NewRegistry(knownList{caster, holyThing, lockedDoor, unlockableDoor, undeadMonster, livingMonster, deadUndead, undeadServitor})
	skill := &modelskill.Definition{}

	holy := mustHandler(t, registry, modelskill.TargetHoly)
	if got := holy.FinalTarget(caster, holyThing, skill); got != holyThing {
		t.Fatalf("holy final target = %v, want holy thing", got)
	}
	if got := ids(holy.Targets(caster, holyThing, skill)); !slices.Equal(got, []int32{2}) {
		t.Fatalf("holy targets = %v, want [2]", got)
	}
	if !holy.CanCast(caster, holyThing, skill, false) {
		t.Fatal("holy CanCast on holy target = false, want true")
	}
	if holy.CanCast(caster, lockedDoor, skill, false) {
		t.Fatal("holy CanCast on non-holy target = true, want false")
	}

	unlockable := mustHandler(t, registry, modelskill.TargetUnlockable)
	if got := unlockable.FinalTarget(caster, unlockableDoor, skill); got != unlockableDoor {
		t.Fatalf("unlockable final target = %v, want unlockable door", got)
	}
	if got := ids(unlockable.Targets(caster, unlockableDoor, skill)); !slices.Equal(got, []int32{4}) {
		t.Fatalf("unlockable targets = %v, want [4]", got)
	}
	if !unlockable.CanCast(caster, unlockableDoor, skill, false) {
		t.Fatal("unlockable CanCast on unlockable target = false, want true")
	}
	if unlockable.CanCast(caster, lockedDoor, skill, false) {
		t.Fatal("unlockable CanCast on locked target = true, want false")
	}

	undead := mustHandler(t, registry, modelskill.TargetUndead)
	if got := undead.FinalTarget(caster, undeadMonster, skill); got != undeadMonster {
		t.Fatalf("undead final target = %v, want undead monster", got)
	}
	if got := ids(undead.Targets(caster, undeadMonster, skill)); !slices.Equal(got, []int32{5}) {
		t.Fatalf("undead targets = %v, want [5]", got)
	}
	if !undead.CanCast(caster, undeadMonster, skill, false) {
		t.Fatal("undead CanCast on undead monster = false, want true")
	}
	if !undead.CanCast(caster, undeadServitor, skill, false) {
		t.Fatal("undead CanCast on undead servitor = false, want true")
	}
	if undead.CanCast(caster, livingMonster, skill, false) {
		t.Fatal("undead CanCast on living monster = true, want false")
	}
	if undead.CanCast(caster, deadUndead, skill, false) {
		t.Fatal("undead CanCast on dead undead = true, want false")
	}
}
