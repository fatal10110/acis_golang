package npcs

import (
	"slices"
	"testing"

	gamesql "github.com/fatal10110/acis_golang/internal/gameserver/data/sql"
	"github.com/fatal10110/acis_golang/internal/gameserver/data/sql/sqltest"
	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	skillstate "github.com/fatal10110/acis_golang/internal/gameserver/skill"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
)

// nobleHeroSkillOptions loads the fixture skills plus the noble and hero
// skills, so a skill list shows them.
func nobleHeroSkillOptions(t *testing.T) gameservertest.Option {
	t.Helper()
	defs := []modelskill.Definition{{ID: laterSkill, Level: 3}, {ID: learnedSkill, Level: 1}}
	for _, ref := range append(modelskill.NobleSkills(), modelskill.HeroSkills()...) {
		defs = append(defs, modelskill.Definition{ID: ref.ID, Level: ref.Level})
	}
	db := sqltest.SharedDB(t)
	return gameservertest.WithSkills(skillstate.NewPersistence(gamesql.NewSkillSaveStore(db), modelskill.NewTable(defs), gamesql.NewCharacterSkillStore(db)))
}

// skillListsHold reports, for each SkillList among frames, whether it
// holds every one of refs.
func skillListsHold(t *testing.T, frames [][]byte, refs []modelskill.Ref) []bool {
	t.Helper()
	var out []bool
	for _, f := range frames {
		if f[0] != serverpackets.OpcodeSkillList {
			continue
		}
		ids := skillListIDs(t, f)
		all := true
		for _, ref := range refs {
			all = all && slices.Contains(ids, int(ref.ID))
		}
		out = append(out, all)
	}
	return out
}

// TestSubclassSwitchKeepsNobleAndHeroSkills pins the noble and hero skills
// across a class switch: right after the class's skill list, a noble is
// given the noble skills back with a skill list and UserInfo, then a hero
// the hero skills, on the base class only, with a skill list.
func TestSubclassSwitchKeepsNobleAndHeroSkills(t *testing.T) {
	w := bootSubclassWorld(t, nobleHeroSkillOptions(t), gameservertest.WithOlympiadSeed(seedStatements(t,
		`UPDATE characters SET nobless = 1 WHERE char_name = 'Talker'`,
		`INSERT INTO heroes (char_id, class_id, count, played, active) SELECT obj_Id, 88, 1, 1, 1 FROM characters WHERE char_name = 'Talker'`)))

	frames := w.addSpellhowler(t)
	want := []byte{
		serverpackets.OpcodeActionFailed, serverpackets.OpcodeUserInfo, serverpackets.OpcodeSkillList,
		serverpackets.OpcodeSkillList, serverpackets.OpcodeUserInfo, serverpackets.OpcodeSkillList,
		serverpackets.OpcodeEtcStatusUpdate, serverpackets.OpcodeHennaInfo, serverpackets.OpcodeUserInfo, serverpackets.OpcodeShortCutInit,
		serverpackets.OpcodeSocialAction, serverpackets.OpcodeSkillCoolTime,
		serverpackets.OpcodeSystemMessage, serverpackets.OpcodeNpcHtmlMessage, serverpackets.OpcodeActionFailed,
	}
	if got := keyOpcodes(frames); string(got) != string(want) {
		t.Fatalf("add answer = %x, want %x", got, want)
	}
	if got := skillListsHold(t, frames, modelskill.NobleSkills()); !slices.Equal(got, []bool{false, true, true}) {
		t.Fatalf("skill lists holding the noble skills = %v, want the noble's and the hero's", got)
	}
	if got := skillListsHold(t, frames, modelskill.HeroSkills()[:1]); !slices.Equal(got, []bool{false, false, false}) {
		t.Fatalf("skill lists holding a hero skill on the subclass = %v, want none", got)
	}

	frames = w.changeTo(t, 0)
	if got := skillListsHold(t, frames, modelskill.HeroSkills()); !slices.Equal(got, []bool{false, false, true}) {
		t.Fatalf("skill lists holding the hero skills back on the base class = %v, want the last", got)
	}
	if got := skillListsHold(t, frames, modelskill.NobleSkills()); !slices.Equal(got, []bool{false, true, true}) {
		t.Fatalf("skill lists holding the noble skills back on the base class = %v", got)
	}
}
