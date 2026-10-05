package skills

import (
	"testing"

	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
)

// TestNullSkillConditionRefusesCastSilently pins a skill <cond> whose
// predicate holds no condition: the reference attaches it as null
// (L2Skill.attach) and skill.checkCondition throws on it before any
// feedback, so the cast is refused with nothing sent: no system message (the
// clause's msgId is never read), no ActionFailed, no cast. A <not> around
// such a predicate throws the same way when tested. <player bogus="1"
// level="1"/> is a level check the level-5 caster passes, so that cast
// starts. Guts (139-1) carries the clause in place of its own.
func TestNullSkillConditionRefusesCastSilently(t *testing.T) {
	t.Parallel()
	gameChance := modelskill.Condition{Kind: "game", Attrs: map[string]string{"chance": "50"}}
	for _, tt := range []struct {
		name  string
		root  modelskill.Condition
		casts bool
	}{
		{"game chance", gameChance, false},
		{"not of game chance", modelskill.Condition{Kind: "not", Children: []modelskill.Condition{gameChance}}, false},
		{"player bogus level 1", modelskill.Condition{Kind: "player", Attrs: map[string]string{"bogus": "1", "level": "1"}}, true},
	} {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			def := shippedSkill(t, 139, 1)
			def.Conditions = []modelskill.ConditionClause{{Root: tt.root, MessageID: 113, AddName: true}}
			srv := bootTargetConditionCaster(t, def)
			c, objID := srv.Client, srv.SoleObjectID(t)
			startInWorld(t, c)

			c.Send(encodeRequestMagicSkillUse(139, false, false))
			frames := readUntilQuiet(t, c)
			if tt.casts {
				if len(frames) == 0 || frames[0][0] != serverpackets.OpcodeMagicSkillUse {
					t.Fatalf("%s: frames %x, want the cast to start", tt.name, frameOpcodes(frames))
				}
				return
			}
			if len(frames) != 0 {
				t.Fatalf("%s: frames %x, want none", tt.name, frameOpcodes(frames))
			}
			if srv.PlayerCastingNow(t, objID) {
				t.Fatalf("%s: cast started", tt.name)
			}
		})
	}
}
