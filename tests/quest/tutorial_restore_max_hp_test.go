package quest

import (
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

// TestTutorialLowHPAtRestoreCountsRestoredBuff: the restored HP is measured
// against the maximum HP the restored effects make. A level-1 newbie saved
// at half the maximum HP its first entry computed, but under a third of the
// maximum its saved max-HP buff raises, runs the tutorial's low-HP step on
// the way into the world. The step's journal writes stay, and none of its
// frames reach the client.
func TestTutorialLowHPAtRestoreCountsRestoredBuff(t *testing.T) {
	t.Parallel()
	const (
		buffID = 9320
		bonus  = 5000
	)
	buff := modelskill.Definition{
		ID: buffID, Level: 1, Activation: modelskill.ActivationActive, Target: modelskill.TargetSelf,
		SkillType: "BUFF",
		Effects: []modelskill.EffectTemplate{{
			Name: "Buff", Count: 1, Time: 3600, Icon: true, StackType: "max_hp_up", StackOrder: 1,
			Funcs: []modelskill.FuncTemplate{{Op: modelskill.FuncAdd, Stat: "maxHp", Value: bonus}},
		}},
	}
	db := sqltest.SharedDB(t)
	srv := bootNewbie(t, true, false, gameservertest.WithSkills(skillstate.NewPersistence(
		gamesql.NewSkillSaveStore(db), modelskill.NewTable([]modelskill.Definition{buff}), gamesql.NewCharacterSkillStore(db))))
	objID := createNewbie(t, srv)
	c := srv.Client
	// A first entry, at full HP and with no buff, computes the plain
	// maximum HP.
	enterBurst(t, srv)
	maxHP := srv.PlayerMaxHP(t, objID)
	if got := srv.QuestVars(t, objID, tutorial); got["HP"] != "" {
		t.Fatalf("tutorial after the first entry = %v, want no HP step yet", got)
	}
	c.Send(encodeSingleOpcode(clientpackets.OpcodeRequestRestart))
	readUntil(t, c, serverpackets.OpcodeCharSelectInfo)
	srv.FlushPersistence(t)
	saved := (maxHP + 1) / 2
	if 10*saved < 3*maxHP || 10*saved >= 3*(maxHP+bonus) {
		t.Fatalf("saved HP %d is not between 30%% of %d and 30%% of %d", saved, maxHP, maxHP+bonus)
	}
	if _, err := srv.DB.Exec("UPDATE characters SET curHp=? WHERE obj_Id=?", saved, objID); err != nil {
		t.Fatal(err)
	}
	if _, err := srv.DB.Exec(
		`INSERT INTO character_skills_save (char_obj_id, skill_id, skill_level, effect_count, effect_cur_time, reuse_delay, systime, restore_type, class_index, buff_index) VALUES (?,?,?,?,?,?,?,?,?,?)`,
		objID, buffID, 1, 1, 1, 0, 0, 0, 0, 1); err != nil {
		t.Fatal(err)
	}

	c.Send(encodeRequestGameStart(0))
	if got := tutorialFrames(srv.ReadQueued(t, c)); len(got) != 0 {
		t.Fatalf("selection sent tutorial frames %x, want none", got)
	}
	c.Send(encodeSingleOpcode(clientpackets.OpcodeEnterWorld))
	burst := srv.ReadQueued(t, c)
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
