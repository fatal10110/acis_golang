package combat

import (
	"testing"

	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/skill/effect"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
)

func deathEffectSkills() []modelskill.Definition {
	return append(killSkillDefs(),
		modelskill.Definition{ID: 1001, Level: 1, Activation: modelskill.ActivationActive, Target: modelskill.TargetSelf, HitTime: 0, StaticHitTime: true, SkillType: "BUFF", Effects: []modelskill.EffectTemplate{{Name: "NoblesseBless", Time: 60}}},
		modelskill.Definition{ID: 1002, Level: 1, Activation: modelskill.ActivationActive, Target: modelskill.TargetSelf, HitTime: 0, StaticHitTime: true, SkillType: "BUFF", Effects: []modelskill.EffectTemplate{{Name: "PhoenixBless", Time: 60}}},
		modelskill.Definition{ID: 1003, Level: 1, Activation: modelskill.ActivationActive, Target: modelskill.TargetSelf, HitTime: 0, StaticHitTime: true, SkillType: "BUFF", Effects: []modelskill.EffectTemplate{{Name: "CharmOfLuck", Time: 60}}},
		modelskill.Definition{ID: 1004, Level: 1, Activation: modelskill.ActivationActive, Target: modelskill.TargetSelf, HitTime: 0, StaticHitTime: true, SkillType: "BUFF", Effects: []modelskill.EffectTemplate{{Name: "Buff", Time: 60}}},
		modelskill.Definition{ID: 1005, Level: 1, Activation: modelskill.ActivationActive, Target: modelskill.TargetSelf, HitTime: 0, StaticHitTime: true, SkillType: "BUFF", StayAfterDeath: true, Effects: []modelskill.EffectTemplate{{Name: "Buff", Time: 60}}},
	)
}

func TestDeathConsumesBlessingsAndCleansOrdinaryEffects(t *testing.T) {
	tests := []struct {
		name    string
		cast    []int
		wantIDs []int
	}{
		{name: "phoenix", cast: []int{1001, 1002, 1003, 1004}, wantIDs: []int{1002, 1004}},
		{name: "noblesse", cast: []int{1001, 1003, 1004}, wantIDs: []int{1004}},
		{name: "ordinary", cast: []int{1004, 1005}, wantIDs: []int{1005}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			srv := gameservertest.Boot(t, gameservertest.WithCharacter("Newbie", 5, 0), gameservertest.WithWantChars(1), gameservertest.WithSkills(combatPersistence(t, deathEffectSkills())))
			victimClient, victimID := srv.Client, srv.SoleObjectID(t)
			for _, skillID := range tt.cast {
				seedKnownSkill(t, srv, victimID, skillID, 1)
			}
			startInWorld(t, victimClient)
			for _, skillID := range tt.cast {
				victimClient.Send(encodeRequestMagicSkillUse(int32(skillID), false, false))
				drainUntilQuiet(t, victimClient)
			}

			killerChar := srv.SeedCharacterFor(t, "killer", "Killer", 5, 0)
			seedKnownSkill(t, srv, killerChar.ID, 42, 1)
			killer := srv.DialClient(t, "killer", 1)
			startInWorld(t, killer)
			drainUntilQuiet(t, killer)
			drainUntilQuiet(t, victimClient)
			killPrimaryClient(t, srv, killer, killerChar.ID, victimID)

			obj, ok := srv.State.Player(victimID)
			if !ok {
				t.Fatal("victim missing from world state")
			}
			victim, ok := obj.(interface{ EffectList() *effect.List })
			if !ok {
				t.Fatalf("world victim = %T, want effect-list owner", obj)
			}
			remaining := victim.EffectList().All()
			if len(remaining) != len(tt.wantIDs) {
				t.Fatalf("effects after death = %v, want skill IDs %v", effectIDs(remaining), tt.wantIDs)
			}
			for _, wantID := range tt.wantIDs {
				if !hasEffectID(remaining, wantID) {
					t.Fatalf("effects after death = %v, want skill ID %d", effectIDs(remaining), wantID)
				}
			}
		})
	}
}

func hasEffectID(effects []*effect.Effect, id int) bool {
	for _, e := range effects {
		if int(e.Skill.ID) == id {
			return true
		}
	}
	return false
}

func effectIDs(effects []*effect.Effect) []int {
	ids := make([]int, len(effects))
	for i, e := range effects {
		ids[i] = int(e.Skill.ID)
	}
	return ids
}
