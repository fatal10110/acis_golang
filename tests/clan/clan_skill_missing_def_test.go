package clan

import (
	"context"
	"database/sql"
	"testing"

	gamesql "github.com/fatal10110/acis_golang/internal/gameserver/data/sql"
	"github.com/fatal10110/acis_golang/internal/gameserver/data/sql/sqltest"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/item"
	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
	skillstate "github.com/fatal10110/acis_golang/internal/gameserver/skill"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
)

// TestClanSkillDroppedByReload boots a clan knowing Clan Vitality and a
// clan skill whose definition a later skill table reload drops. The leader
// logging in after the reload is given Clan Vitality and not the dropped
// skill, whose missing definition holds it from every rank.
func TestClanSkillDroppedByReload(t *testing.T) {
	const droppedID = 30000
	shipped, trees := shippedSkillData(t)
	if _, ok := shipped.Get(droppedID, 1); ok {
		t.Fatalf("skill %d level 1 is in the shipped table; pick an unused id", droppedID)
	}
	defs := modelskill.NewTable(append(shipped.All(), modelskill.Definition{
		ID: droppedID, Level: 1, Name: "Dropped Clan Skill", Activation: modelskill.ActivationPassive,
	}))
	db := sqltest.SharedDB(t)
	w := bootClanWorld(t, 40, 0, 0,
		gameservertest.WithSkills(skillstate.NewPersistence(gamesql.NewSkillSaveStore(db), defs, gamesql.NewCharacterSkillStore(db))),
		gameservertest.WithSkillTrees(trees),
		gameservertest.WithItemTemplates(item.NewTable(gameservertest.ItemTemplates().All())),
		gameservertest.WithClanSeed(func(db *sql.DB) {
			seedClanRows(t, clanSeed{level: 5, reputation: 1000}, db)
			for _, id := range []int{vitalityID, droppedID} {
				if _, err := db.ExecContext(context.Background(), `INSERT INTO clan_skills (clan_id, skill_id, skill_level) VALUES (?,?,1)`, seededClanID, id); err != nil {
					t.Fatalf("seed clan skill %d: %v", id, err)
				}
			}
		}),
	)
	if lvl := w.srv.PlayerSkillLevel(t, w.leaderID, droppedID); lvl != 1 {
		t.Fatalf("leader's clan skill %d before the reload = level %d, want 1", droppedID, lvl)
	}

	defs.Replace(modelskill.NewTable(shipped.All()))
	w.leaveWorld(t, w.leader)
	w.leader = w.srv.DialClient(t, "player1", 1)
	startInWorld(t, w.leader)

	if lvl := w.srv.PlayerSkillLevel(t, w.leaderID, vitalityID); lvl != 1 {
		t.Fatalf("leader's Clan Vitality level = %d, want 1", lvl)
	}
	if lvl := w.srv.PlayerSkillLevel(t, w.leaderID, droppedID); lvl != 0 {
		t.Fatalf("leader's dropped clan skill level = %d, want 0", lvl)
	}
}
