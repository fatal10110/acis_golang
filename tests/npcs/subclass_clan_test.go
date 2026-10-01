package npcs

import (
	"context"
	"database/sql"
	"slices"
	"testing"

	"github.com/fatal10110/acis_golang/internal/commons/wire"
	gamesql "github.com/fatal10110/acis_golang/internal/gameserver/data/sql"
	"github.com/fatal10110/acis_golang/internal/gameserver/data/sql/sqltest"
	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/clientpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	skillstate "github.com/fatal10110/acis_golang/internal/gameserver/skill"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
)

// Clan Vitality, a clan skill held from clan rank 2 (clanSkills.xml); the
// Gladiator leads a level 5 clan, rank 4.
const (
	clanVitality = 370
	subclassClan = 268435456
)

// subclassClanOptions stores a level 5 clan with reputation the Gladiator
// leads, holding Clan Vitality 1, and loads that skill beside the
// subclass scenarios' class skills.
func subclassClanOptions(t *testing.T) []gameservertest.Option {
	t.Helper()
	db := sqltest.SharedDB(t)
	defs := modelskill.NewTable([]modelskill.Definition{
		{ID: laterSkill, Level: 3},
		{ID: learnedSkill, Level: 1},
		{ID: clanVitality, Level: 1, MinPledgeClass: 2},
	})
	return []gameservertest.Option{
		gameservertest.WithSkills(skillstate.NewPersistence(gamesql.NewSkillSaveStore(db), defs, gamesql.NewCharacterSkillStore(db))),
		gameservertest.WithClanSeed(func(db *sql.DB) {
			for _, q := range []struct {
				query string
				args  []any
			}{
				{`UPDATE characters SET clanid = ?, power_grade = 0 WHERE char_name = 'Talker'`, []any{subclassClan}},
				{`INSERT INTO clan_data (clan_id, clan_name, clan_level, reputation_score, leader_id)
					SELECT ?, 'Masters', 5, 1000, obj_Id FROM characters WHERE char_name = 'Talker'`, []any{subclassClan}},
				{`INSERT INTO clan_skills (clan_id, skill_id, skill_level) VALUES (?, ?, 1)`, []any{subclassClan, clanVitality}},
			} {
				if _, err := db.ExecContext(context.Background(), q.query, q.args...); err != nil {
					t.Fatalf("%s: %v", q.query, err)
				}
			}
		}),
	}
}

// requestSkillList asks for the skill list and returns the skills it shows.
func (w *subclassWorld) requestSkillList(t *testing.T) []int {
	t.Helper()
	w.c.Send(wire.NewPacketWriter(clientpackets.OpcodeRequestSkillList).Bytes())
	frame, ok := firstOpcode(drainFrames(t, w.c), serverpackets.OpcodeSkillList)
	if !ok {
		t.Fatal("RequestSkillList answered no SkillList")
	}
	return skillListIDs(t, frame)
}

// TestSubclassSwitchKeepsTheClanSkills switches a clan leader to a new
// subclass and back: the clan's skills, held outside any class, come back
// after each switch's own skill list, which does not show them.
func TestSubclassSwitchKeepsTheClanSkills(t *testing.T) {
	w := bootSubclassWorld(t, subclassClanOptions(t)...)
	if skills := w.requestSkillList(t); !slices.Contains(skills, clanVitality) {
		t.Fatalf("skills at login = %v, want the clan's %d", skills, clanVitality)
	}

	for _, step := range []struct {
		name string
		run  func() [][]byte
	}{
		{"add Spellhowler", func() [][]byte { return w.addSpellhowler(t) }},
		{"back to the base class", func() [][]byte { return w.changeTo(t, 0) }},
		{"back to the subclass", func() [][]byte { return w.changeTo(t, 1) }},
	} {
		frames := step.run()
		if skills := skillListIDs(t, frames[firstIndex(frames, serverpackets.OpcodeSkillList)]); slices.Contains(skills, clanVitality) {
			t.Fatalf("%s: the switch's skill list = %v, want it sent before the clan's skills come back", step.name, skills)
		}
		if skills := w.requestSkillList(t); !slices.Contains(skills, clanVitality) {
			t.Fatalf("%s: skills after the switch = %v, want the clan's %d", step.name, skills, clanVitality)
		}
	}
}
