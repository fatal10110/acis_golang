package skills

import (
	"testing"
	"time"

	gamesql "github.com/fatal10110/acis_golang/internal/gameserver/data/sql"
	"github.com/fatal10110/acis_golang/internal/gameserver/data/sql/sqltest"
	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	skillstate "github.com/fatal10110/acis_golang/internal/gameserver/skill"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
)

// slowStoreDelay is longer than the sim pool's 50 ms slow-task budget, so a
// character_skills write still made on an actor queue would be logged by the
// watchdog.
const slowStoreDelay = 120 * time.Millisecond

// TestSlowKnownSkillStoreKeepsQueuesFree learns a skill against a
// character_skills table whose writes take longer than the sim pool's
// slow-task budget. The learn applies in memory and queues its row write on
// the persistence worker, the way the reference stores a skill only after
// putting it in the character's map (Player.addSkill), so the player's queue
// never waits on the database: the whole learn reply arrives at memory
// speed, the watchdog logs nothing, and the row lands once the worker
// drains.
func TestSlowKnownSkillStoreKeepsQueuesFree(t *testing.T) {
	// Not parallel: the slow-task budget is wall-clock, so other tests' CPU
	// load fails it spuriously.
	srv, c, objID := bootLearner(t,
		gameservertest.WithCharacter("Newbie", 5, 50),
		gameservertest.WithWantChars(1),
		gameservertest.WithCapturedLog(),
		gameservertest.WithRealPool(), // the slow-task watchdog runs only on the pool
		gameservertest.WithSkills(slowLearnerTable(t)),
	)
	bindSkillShortcut(t, srv, objID, 3, 3, -1)
	startInWorld(t, c)

	c.Send(encodeRequestAcquireSkill(3, 1, 0))
	assertSPStatus(t, c.Read(), objID, 0)
	assertFrameOpcode(t, c.Read(), serverpackets.OpcodeSystemMessage, "SP-decreased SystemMessage")
	assertSystemMessageSkillFrame(t, c.Read(), serverpackets.SystemMessageLearnedSkill, 3, 1)
	assertSkillList(t, c.Read(), skillListEntry{passive: 0, level: 1, id: 3})
	assertShortCutRegister(t, c, 3, 3, 1)
	assertAcquireSkillListEmpty(t, c.Read(), serverpackets.AcquireSkillTypeUsual)
	drainUntilQuiet(t, c)

	srv.Settle(t)
	if lines := srv.SlowTaskLogs(); len(lines) != 0 {
		t.Fatalf("queue task blocked on character_skills: %v", lines)
	}
	assertKnownSkills(t, srv, objID, map[int]int{3: 1})
}

// slowLearnerTable is learnerTable's skill set over a character_skills store
// whose writes are slow.
func slowLearnerTable(t *testing.T) *skillstate.Persistence {
	t.Helper()
	db := sqltest.SharedDB(t)
	defs := []modelskill.Definition{
		{ID: 248, Level: 3},
		{ID: 294, Level: 1},
		{ID: 3, Level: 1, Activation: modelskill.ActivationActive},
	}
	return skillstate.NewPersistence(
		gamesql.NewSkillSaveStore(db),
		modelskill.NewTable(defs),
		gameservertest.SlowKnownSkills(gamesql.NewCharacterSkillStore(db), slowStoreDelay),
	)
}
