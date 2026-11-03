package skills

import (
	"testing"

	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
)

// TestRaceAndSkillStatConditionsGateCast pins two skill <cond> predicates on
// a player's own cast: <player race> (ConditionPlayerRace) holds only for
// the caster's race, and <skill stat> (ConditionSkillStats) only when the
// skill being cast names that stat in its "stat" set value. The fixture
// caster is a human fighter. A failing clause answers S1_CANNOT_BE_USED
// naming the skill, with no ActionFailed and no cast; a holding one starts
// the cast. Guts (139-1) carries the clause in place of its own.
func TestRaceAndSkillStatConditionsGateCast(t *testing.T) {
	t.Parallel()
	for _, tt := range []struct {
		name  string
		root  modelskill.Condition
		stat  string
		casts bool
	}{
		{"human caster, race HUMAN", modelskill.Condition{Kind: "player", Attrs: map[string]string{"race": "HUMAN"}}, "", true},
		{"human caster, race ELF", modelskill.Condition{Kind: "player", Attrs: map[string]string{"race": "ELF"}}, "", false},
		{"skill names maxHp", modelskill.Condition{Kind: "skill", Attrs: map[string]string{"stat": "maxHp"}}, "MAX_HP", true},
		{"skill names pAtk", modelskill.Condition{Kind: "skill", Attrs: map[string]string{"stat": "maxHp"}}, "POWER_ATTACK", false},
		{"skill names no stat", modelskill.Condition{Kind: "skill", Attrs: map[string]string{"stat": "maxHp"}}, "", false},
	} {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			def := shippedSkill(t, 139, 1)
			def.Stat = tt.stat
			def.Conditions = []modelskill.ConditionClause{{Root: tt.root, MessageID: serverpackets.SystemMessageS1CannotBeUsed, AddName: true}}
			srv := bootTargetConditionCaster(t, def)
			c, objID := srv.Client, srv.SoleObjectID(t)
			startInWorld(t, c)

			c.Send(encodeRequestMagicSkillUse(139, false, false))
			if tt.casts {
				assertFrameOpcode(t, c.Read(), serverpackets.OpcodeMagicSkillUse, tt.name)
				return
			}
			assertSystemMessageSkillFrame(t, c.Read(), serverpackets.SystemMessageS1CannotBeUsed, 139, 1)
			assertNoActionFailedUntilQuiet(t, c, tt.name)
			if srv.PlayerCastingNow(t, objID) {
				t.Fatalf("%s: cast started", tt.name)
			}
		})
	}
}
