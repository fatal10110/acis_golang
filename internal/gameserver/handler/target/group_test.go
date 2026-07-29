package target

import (
	"slices"
	"testing"

	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
)

func TestPartyHandlerSweepsActingPlayerAndPartyMembers(t *testing.T) {
	caster := &targetActor{
		id: 1, category: CategoryPlayable, x: 0,
		sameParty: map[int32]bool{3: true},
	}
	summon := &targetActor{id: 2, category: CategoryPlayable, x: 20, owner: caster}
	partyMember := &targetActor{id: 3, category: CategoryPlayable, x: 50}
	stranger := &targetActor{id: 4, category: CategoryPlayable, x: 50}
	deadPartyMember := &targetActor{id: 5, category: CategoryPlayable, x: 40, dead: true}
	caster.summon = summon

	registry := NewRegistry(knownList{caster, summon, partyMember, stranger, deadPartyMember})
	party := mustHandler(t, registry, modelskill.TargetParty)

	if got := party.FinalTarget(caster, nil, &modelskill.Definition{}); got != caster {
		t.Fatalf("party final target = %v, want caster", got)
	}
	got := ids(party.Targets(caster, nil, &modelskill.Definition{Radius: 100}))
	if want := []int32{1, 2, 3}; !slices.Equal(got, want) {
		t.Fatalf("party targets = %v, want %v", got, want)
	}
	if !party.CanCast(caster, nil, &modelskill.Definition{}, false) {
		t.Fatal("party CanCast = false, want true")
	}
}

func TestAllyHandlerSweepsClanAndAllyBypassingDuelOpponents(t *testing.T) {
	caster := &targetActor{
		id: 1, category: CategoryPlayable, x: 0, hasClan: true,
		sameClan: map[int32]bool{2: true},
		sameAlly: map[int32]bool{3: true},
	}
	casterSummon := &targetActor{id: 10, category: CategoryPlayable, x: 30, owner: caster}
	caster.summon = casterSummon
	clanMate := &targetActor{id: 2, category: CategoryPlayable, x: 60}
	allyMate := &targetActor{id: 3, category: CategoryPlayable, x: 70}
	stranger := &targetActor{id: 4, category: CategoryPlayable, x: 60}
	deadClanMate := &targetActor{id: 5, category: CategoryPlayable, x: 60, dead: true}

	registry := NewRegistry(knownList{caster, casterSummon, clanMate, allyMate, stranger, deadClanMate})
	ally := mustHandler(t, registry, modelskill.TargetAlly)

	if got := ally.FinalTarget(caster, nil, &modelskill.Definition{}); got != caster {
		t.Fatalf("ally final target = %v, want caster", got)
	}
	got := ids(ally.Targets(caster, nil, &modelskill.Definition{Radius: 100}))
	if want := []int32{1, 10, 2, 3}; !slices.Equal(got, want) {
		t.Fatalf("ally targets = %v, want %v", got, want)
	}

	// Olympiad caster draws only itself.
	caster.olympiad = true
	if got := ids(ally.Targets(caster, nil, &modelskill.Definition{Radius: 100})); !slices.Equal(got, []int32{1}) {
		t.Fatalf("ally olympiad targets = %v, want [1]", got)
	}
	caster.olympiad = false

	// Dueling caster excludes allies from the opposing team.
	duelingClan := &targetActor{
		id: 11, category: CategoryPlayable, x: 60,
		duelID: 7, duelTeam: 1,
	}
	duelingClanEnemy := &targetActor{
		id: 12, category: CategoryPlayable, x: 60,
		duelID: 7, duelTeam: 2,
	}
	caster.duelID = 7
	caster.duelTeam = 1
	caster.sameClan = map[int32]bool{11: true, 12: true}
	duelRegistry := NewRegistry(knownList{caster, duelingClan, duelingClanEnemy})
	duelAlly := mustHandler(t, duelRegistry, modelskill.TargetAlly)
	duelGot := ids(duelAlly.Targets(caster, nil, &modelskill.Definition{Radius: 100}))
	if want := []int32{1, 11}; !slices.Equal(duelGot, want) {
		t.Fatalf("ally dueling targets = %v, want %v", duelGot, want)
	}

	if !ally.CanCast(caster, nil, &modelskill.Definition{}, false) {
		t.Fatal("ally CanCast = false, want true")
	}
}

func TestClanHandlerMatchesMonsterClanGroups(t *testing.T) {
	attacker := &targetActor{id: 1, category: CategoryAttackable, x: 0, clanGroups: []string{"orc"}}
	fellow := &targetActor{id: 2, category: CategoryAttackable, x: 60, clanGroups: []string{"orc"}}
	rival := &targetActor{id: 3, category: CategoryAttackable, x: 60, clanGroups: []string{"undead"}}
	deadFellow := &targetActor{id: 4, category: CategoryAttackable, x: 60, dead: true, clanGroups: []string{"orc"}}
	clanless := &targetActor{id: 5, category: CategoryAttackable, x: 60}
	playableNearby := &targetActor{id: 6, category: CategoryPlayable, x: 60, clanGroups: []string{"orc"}}

	registry := NewRegistry(knownList{attacker, fellow, rival, deadFellow, clanless, playableNearby})
	clan := mustHandler(t, registry, modelskill.TargetClan)

	if got := clan.FinalTarget(attacker, nil, &modelskill.Definition{}); got != attacker {
		t.Fatalf("clan final target = %v, want attacker", got)
	}
	got := ids(clan.Targets(attacker, nil, &modelskill.Definition{Radius: 100}))
	if want := []int32{1, 2}; !slices.Equal(got, want) {
		t.Fatalf("clan targets = %v, want %v", got, want)
	}

	// Non-attackable caster contributes no list.
	playable := &targetActor{id: 7, category: CategoryPlayable}
	if got := clan.Targets(playable, nil, &modelskill.Definition{Radius: 100}); got != nil {
		t.Fatalf("clan targets for playable caster = %v, want nil", got)
	}
}

func TestPartyMemberHandlerSpecialCasesSelfSummonAndSummonFriend(t *testing.T) {
	caster := &targetActor{
		id: 1, category: CategoryPlayable,
		inParty: true, partyMembers: map[int32]bool{3: true},
	}
	casterSummon := &targetActor{id: 2, category: CategoryPlayable, owner: caster}
	caster.summon = casterSummon
	partyPlayer := &targetActor{id: 3, category: CategoryPlayable}
	deadPlayer := &targetActor{id: 4, category: CategoryPlayable, dead: true}
	otherPlayer := &targetActor{id: 5, category: CategoryPlayable}

	registry := NewRegistry(knownList{})
	pm := mustHandler(t, registry, modelskill.TargetPartyMember)

	if got := pm.FinalTarget(caster, partyPlayer, &modelskill.Definition{}); got != partyPlayer {
		t.Fatalf("party member final target = %v, want target", got)
	}
	if got := ids(pm.Targets(caster, partyPlayer, &modelskill.Definition{})); !slices.Equal(got, []int32{3}) {
		t.Fatalf("party member targets = %v, want [3]", got)
	}

	regular := &modelskill.Definition{}
	summonFriend := &modelskill.Definition{ID: summonFriendSkillID}

	tests := []struct {
		name   string
		target *targetActor
		skill  *modelskill.Definition
		want   bool
	}{
		{"self any skill", caster, regular, true},
		{"own summon any regular skill", casterSummon, regular, true},
		{"summon friend on living party player", partyPlayer, summonFriend, true},
		{"summon friend on dead party player", deadPlayer, summonFriend, false},
		{"summon friend on own summon (not player-like)", casterSummon, summonFriend, false},
		{"regular skill on dead playable", deadPlayer, regular, false},
		{"regular skill on party player", partyPlayer, regular, true},
		{"regular skill on non-party player", otherPlayer, regular, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := pm.CanCast(caster, tt.target, tt.skill, false); got != tt.want {
				t.Fatalf("CanCast = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestPartyOtherHandlerGatesSelfMageClassAndPartyMembership(t *testing.T) {
	caster := &targetActor{
		id: 1, category: CategoryPlayable,
		inParty: true, partyMembers: map[int32]bool{2: true, 3: true},
	}
	mage := &targetActor{id: 2, category: CategoryPlayable, mageClass: true}
	fighter := &targetActor{id: 3, category: CategoryPlayable}
	deadPlayer := &targetActor{id: 4, category: CategoryPlayable, dead: true}
	casterSummon := &targetActor{id: 5, category: CategoryPlayable, owner: caster}
	otherPlayer := &targetActor{id: 6, category: CategoryPlayable}

	registry := NewRegistry(knownList{})
	other := mustHandler(t, registry, modelskill.TargetPartyOther)

	if got := other.FinalTarget(caster, mage, &modelskill.Definition{}); got != mage {
		t.Fatalf("party other final target = %v, want target", got)
	}
	if got := ids(other.Targets(caster, mage, &modelskill.Definition{})); !slices.Equal(got, []int32{2}) {
		t.Fatalf("party other targets = %v, want [2]", got)
	}

	regular := &modelskill.Definition{}
	tests := []struct {
		name   string
		target *targetActor
		skill  *modelskill.Definition
		want   bool
	}{
		{"self rejected", caster, regular, false},
		{"non-player (own summon) rejected", casterSummon, regular, false},
		{"dead player rejected", deadPlayer, regular, false},
		{"426 on mage rejected", mage, &modelskill.Definition{ID: dualcastManaSkillID}, false},
		{"426 on fighter accepted", fighter, &modelskill.Definition{ID: dualcastManaSkillID}, true},
		{"427 on mage accepted", mage, &modelskill.Definition{ID: dualcastHealSkillID}, true},
		{"427 on fighter rejected", fighter, &modelskill.Definition{ID: dualcastHealSkillID}, false},
		{"regular on party player accepted", mage, regular, true},
		{"regular on non-party player rejected", otherPlayer, regular, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := other.CanCast(caster, tt.target, tt.skill, false); got != tt.want {
				t.Fatalf("CanCast = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestCorpseAllyHandlerSweepsDeadAllies(t *testing.T) {
	caster := &targetActor{
		id: 1, category: CategoryPlayable, x: 0, hasClan: true,
		sameClan: map[int32]bool{2: true},
		sameAlly: map[int32]bool{3: true},
	}
	deadClanMate := &targetActor{id: 2, category: CategoryPlayable, x: 50, dead: true}
	deadAllyMate := &targetActor{id: 3, category: CategoryPlayable, x: 50, dead: true}
	livingClanMate := &targetActor{id: 4, category: CategoryPlayable, x: 50}
	deadStranger := &targetActor{id: 5, category: CategoryPlayable, x: 50, dead: true}
	deadOwnedSummon := &targetActor{id: 6, category: CategoryPlayable, x: 50, dead: true, owner: caster}

	registry := NewRegistry(knownList{caster, deadClanMate, deadAllyMate, livingClanMate, deadStranger, deadOwnedSummon})
	corpseAlly := mustHandler(t, registry, modelskill.TargetCorpseAlly)

	if got := corpseAlly.FinalTarget(caster, nil, &modelskill.Definition{}); got != caster {
		t.Fatalf("corpse ally final target = %v, want caster", got)
	}
	if got := ids(corpseAlly.Targets(caster, nil, &modelskill.Definition{Radius: 100})); !slices.Equal(got, []int32{2, 3}) {
		t.Fatalf("corpse ally targets = %v, want [2 3]", got)
	}

	// Clanless caster has no scan, falls back to the caster.
	caster.hasClan = false
	if got := ids(corpseAlly.Targets(caster, nil, &modelskill.Definition{Radius: 100})); !slices.Equal(got, []int32{1}) {
		t.Fatalf("corpse ally clanless fallback = %v, want [1]", got)
	}
	caster.hasClan = true

	// Olympiad caster cannot cast at all.
	if !corpseAlly.CanCast(caster, nil, &modelskill.Definition{}, false) {
		t.Fatal("corpse ally CanCast when not in olympiad = false, want true")
	}
	caster.olympiad = true
	if corpseAlly.CanCast(caster, nil, &modelskill.Definition{}, false) {
		t.Fatal("corpse ally CanCast in olympiad = true, want false")
	}
	caster.olympiad = false

	// Dueling caster excludes same-clan corpses on the opposing team.
	caster.duelID = 7
	caster.duelTeam = 1
	deadClanMate.duelID = 7
	deadClanMate.duelTeam = 2
	deadAllyMate.duelID = 7
	deadAllyMate.duelTeam = 1
	duelGot := ids(corpseAlly.Targets(caster, nil, &modelskill.Definition{Radius: 100}))
	if want := []int32{3}; !slices.Equal(duelGot, want) {
		t.Fatalf("corpse ally dueling targets = %v, want %v", duelGot, want)
	}
}
