package skills

import (
	"context"
	"testing"

	"github.com/fatal10110/acis_golang/internal/commons/wire"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/player"
	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
	"github.com/fatal10110/acis_golang/internal/testsupport"
)

// TestLevelUpRewardRefreshesShortcutBeforeSkillList pins the automatic
// skill learning of a level-up (AutoLearnSkills, Player.rewardSkills): a
// skill the new level raises re-points the shortcut bound to it — a
// ShortCutRegister at the new level — before the one SkillList that ends
// the grant.
func TestLevelUpRewardRefreshesShortcutBeforeSkillList(t *testing.T) {
	t.Parallel()
	const raised = 1001 // level 1 at 5, level 2 at 10, bought with SP
	tmpl := gameservertest.ClassTemplate()
	tmpl.Skills = []player.SkillGrant{
		{SkillID: raised, Level: 1, MinLevel: 5, Cost: 50},
		{SkillID: raised, Level: 2, MinLevel: 10, Cost: 100},
	}
	srv, c, objID := bootLearner(t,
		gameservertest.WithCharacter("Newbie", 9, 0),
		gameservertest.WithWantChars(1),
		gameservertest.WithClassTemplate(tmpl),
		gameservertest.WithAutoLearnSkills(),
		gameservertest.WithSkills(skillPersistence(t, []modelskill.Definition{
			{ID: raised, Level: 1, Activation: modelskill.ActivationActive},
			{ID: raised, Level: 2, Activation: modelskill.ActivationActive},
		})),
	)
	if _, err := srv.DB.ExecContext(context.Background(), "INSERT INTO character_skills (char_obj_id, skill_id, skill_level, class_index) VALUES (?, ?, 1, 0)", objID, raised); err != nil {
		t.Fatalf("seed skill: %v", err)
	}
	bindSkillShortcut(t, srv, objID, 3, raised, 1)
	startInWorld(t, c)

	srv.AddPlayerLevel(t, objID, 1)
	frames := readUntilQuiet(t, c)
	register, list := -1, -1
	for i, f := range frames {
		switch f[0] {
		case serverpackets.OpcodeShortCutRegister:
			r := wire.NewReader(f[1:])
			if typ, slot, id, level := r.ReadInt32(), r.ReadInt32(), r.ReadInt32(), r.ReadInt32(); typ != int32(serverpackets.ShortcutSkill) || slot != 3 || id != raised || level != 2 {
				t.Fatalf("ShortCutRegister = type %d slot %d id %d level %d, want skill %d level 2 at slot 3", typ, slot, id, level, raised)
			}
			register = i
		case serverpackets.OpcodeSkillList:
			if list < 0 {
				list = i
			}
		}
	}
	if register < 0 || list < register {
		t.Fatalf("level-up frames %x (ShortCutRegister at %d, SkillList at %d), want the shortcut refreshed then the skill list", testsupport.FrameOpcodes(frames), register, list)
	}
}
