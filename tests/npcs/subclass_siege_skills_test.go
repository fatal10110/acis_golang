package npcs

import (
	"context"
	"database/sql"
	"testing"

	gamesql "github.com/fatal10110/acis_golang/internal/gameserver/data/sql"
	"github.com/fatal10110/acis_golang/internal/gameserver/data/sql/sqltest"
	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
	skillstate "github.com/fatal10110/acis_golang/internal/gameserver/skill"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
)

// Seal of Ruler and Build Headquarters, the siege skills a leader of a
// clan of siege level (SiegeClanMinLevel, 4) holds outside any class
// (Player.regiveTemporarySkills).
const (
	sealOfRuler = 246
	buildHQ     = 247
)

// subclassSiegeOptions stores a level 5 clan the Gladiator leads and
// loads the siege skills beside the subclass scenarios' class skills.
func subclassSiegeOptions(t *testing.T) []gameservertest.Option {
	t.Helper()
	db := sqltest.SharedDB(t)
	defs := modelskill.NewTable([]modelskill.Definition{
		{ID: laterSkill, Level: 3},
		{ID: learnedSkill, Level: 1},
		{ID: sealOfRuler, Level: 1},
		{ID: buildHQ, Level: 1},
	})
	return []gameservertest.Option{
		gameservertest.WithSkills(skillstate.NewPersistence(gamesql.NewSkillSaveStore(db), defs, gamesql.NewCharacterSkillStore(db))),
		gameservertest.WithClanSeed(func(db *sql.DB) {
			for _, q := range []string{
				`UPDATE characters SET clanid = 268435456, power_grade = 0 WHERE char_name = 'Talker'`,
				`INSERT INTO clan_data (clan_id, clan_name, clan_level, leader_id)
					SELECT 268435456, 'Masters', 5, obj_Id FROM characters WHERE char_name = 'Talker'`,
			} {
				if _, err := db.ExecContext(context.Background(), q); err != nil {
					t.Fatalf("%s: %v", q, err)
				}
			}
		}),
	}
}

// TestSubclassSwitchKeepsTheSiegeSkills switches the leader of a level 5
// clan to a new subclass and back: it holds the siege skills after every
// switch, as at login.
func TestSubclassSwitchKeepsTheSiegeSkills(t *testing.T) {
	w := bootSubclassWorld(t, subclassSiegeOptions(t)...)
	held := func(when string) {
		t.Helper()
		for _, id := range []int{sealOfRuler, buildHQ} {
			if got := w.srv.PlayerSkillLevel(t, w.player, id); got != 1 {
				t.Fatalf("%s: siege skill %d = %d, want 1", when, id, got)
			}
		}
	}
	held("login")
	w.addSpellhowler(t)
	held("on the new subclass")
	w.changeTo(t, 0)
	held("back on the base class")
}
