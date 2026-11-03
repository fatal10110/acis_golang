package quest

import (
	"slices"
	"testing"
	"time"

	"github.com/fatal10110/acis_golang/internal/commons/wire"
	gamesql "github.com/fatal10110/acis_golang/internal/gameserver/data/sql"
	"github.com/fatal10110/acis_golang/internal/gameserver/data/sql/sqltest"
	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/clientpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	skillstate "github.com/fatal10110/acis_golang/internal/gameserver/skill"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
)

// TestTutorialLowHPAtRestoreAheadOfLoadingScreenTicks: restoring the saved
// HP raises the tutorial's low-HP step before any tick of a restored
// damage-over-time effect. A newbie saved under a third of its maximum HP
// with a saved poison, held on the loading screen while the poison ticks,
// enters the world without the step's question mark or client event: the restore ran the step silently, so the ticks' HP writes find it
// done.
func TestTutorialLowHPAtRestoreAheadOfLoadingScreenTicks(t *testing.T) {
	t.Parallel()
	const (
		poisonID = 9321
		savedHP  = 8
	)
	poison := modelskill.Definition{
		ID: poisonID, Level: 1, Activation: modelskill.ActivationActive, Target: modelskill.TargetOne,
		SkillType: "POISON",
		Effects: []modelskill.EffectTemplate{{
			Name: "DamOverTime", Count: 10, Time: 1, Value: 1,
			StackType: "poison", StackOrder: 1, EffectType: "POISON", Icon: true,
		}},
	}
	db := sqltest.SharedDB(t)
	srv := bootNewbie(t, true, false, gameservertest.WithSkills(skillstate.NewPersistence(
		gamesql.NewSkillSaveStore(db), modelskill.NewTable([]modelskill.Definition{poison}), gamesql.NewCharacterSkillStore(db))))
	if !srv.DrivesClock() {
		t.Skip("holding the loading screen for a set time needs the driven clock")
	}
	objID := createNewbie(t, srv)
	srv.FlushPersistence(t)
	if _, err := srv.DB.Exec("UPDATE characters SET curHp=? WHERE obj_Id=?", savedHP, objID); err != nil {
		t.Fatal(err)
	}
	if _, err := srv.DB.Exec(
		`INSERT INTO character_skills_save (char_obj_id, skill_id, skill_level, effect_count, effect_cur_time, reuse_delay, systime, restore_type, class_index, buff_index) VALUES (?,?,?,?,?,?,?,?,?,?)`,
		objID, poisonID, 1, 10, 0, 0, 0, 0, 0, 1); err != nil {
		t.Fatal(err)
	}

	c := srv.Client
	c.Send(encodeRequestGameStart(0))
	if got := tutorialFrames(srv.ReadQueued(t, c)); len(got) != 0 {
		t.Fatalf("selection sent tutorial frames %x, want none", got)
	}
	srv.Advance(t, 3*time.Second)
	c.Send(encodeSingleOpcode(clientpackets.OpcodeEnterWorld))
	burst := srv.ReadQueued(t, c)
	if hp, maxHP := srv.PlayerCurrentHP(t, objID), srv.PlayerMaxHP(t, objID); hp >= savedHP || 10*savedHP >= 3*maxHP {
		t.Fatalf("HP %d of %d after the loading screen, want the poison's ticks under a saved %d below 30%%", hp, maxHP, savedHP)
	}
	for _, f := range tutorialFrames(burst) {
		arg := wire.NewReader(f[1:]).ReadInt32()
		if f[0] == serverpackets.OpcodeTutorialShowQuestionMark && arg == 10 ||
			f[0] == serverpackets.OpcodeTutorialEnableClientEvent && arg == 8388608 {
			t.Fatalf("burst carries the low-HP step's frame %x", f)
		}
	}
	if got := srv.QuestVars(t, objID, tutorial); got["HP"] != "1" || got["sit"] != "8388608" {
		t.Fatalf("tutorial after entering = %v, want HP 1 and sit 8388608", got)
	}
	srv.FlushPersistence(t)
	if got := readJournal(t, srv.DB); !slices.Contains(got, journalRow{objID, tutorial, "HP", val("1")}) {
		t.Fatalf("journal rows %v, want the HP step's row", got)
	}
}
