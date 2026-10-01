package target

import (
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor"
	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
)

// TestMonsterOnlySkillRejection pins the SPOIL and DRAIN_SOUL target rule:
// only a player's cast on a target that is not Monster-family is refused
// with INVALID_TARGET. A summon's cast, a Monster target and any other
// skill type pass it.
func TestMonsterOnlySkillRejection(t *testing.T) {
	t.Parallel()
	player := &targetActor{id: 1, kind: actor.KindPlayer}
	summon := &targetActor{id: 2, kind: actor.KindSummon, owner: player}
	npc := &targetActor{id: 3, kind: actor.KindNPC, attackableBy: true}
	monster := &targetActor{id: 4, kind: actor.KindNPC, attackableBy: true, monster: true}
	skill := func(skillType string) *modelskill.Definition {
		return &modelskill.Definition{SkillType: skillType, Offensive: true, Target: modelskill.TargetOne}
	}
	for _, tc := range []struct {
		name           string
		caster, target Actor
		skillType      string
		want           CastRejection
	}{
		{"player spoil on non-monster", player, npc, "SPOIL", CastRejectInvalidTarget},
		{"player drain soul on non-monster", player, npc, "DRAIN_SOUL", CastRejectInvalidTarget},
		{"player spoil on monster", player, monster, "SPOIL", CastRejectNone},
		{"player drain soul on monster", player, monster, "DRAIN_SOUL", CastRejectNone},
		{"summon spoil on non-monster", summon, npc, "SPOIL", CastRejectNone},
		{"summon drain soul on non-monster", summon, npc, "DRAIN_SOUL", CastRejectNone},
		{"player other skill on non-monster", player, npc, "PDAM", CastRejectNone},
	} {
		def := skill(tc.skillType)
		if got := CastRejectionFor(modelskill.TargetOne, tc.caster, tc.target, def, false); got != tc.want {
			t.Errorf("%s: CastRejectionFor = %d, want %d", tc.name, got, tc.want)
		}
	}
	if got := monsterOnlySkillRejection(player, nil, skill("SPOIL")); got != CastRejectNone {
		t.Errorf("nil target: monsterOnlySkillRejection = %d, want none", got)
	}
}
