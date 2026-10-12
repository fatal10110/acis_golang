package quest

import (
	"context"
	"slices"
	"testing"
	"time"

	gamesql "github.com/fatal10110/acis_golang/internal/gameserver/data/sql"
	"github.com/fatal10110/acis_golang/internal/gameserver/data/sql/sqltest"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/questlog"
	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/clientpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/script"
	skillstate "github.com/fatal10110/acis_golang/internal/gameserver/skill"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
)

// TestTutorialDeathMarkOnTheLoadingScreenShownInTheBurst: a restored lethal
// poison whose killing tick came due on the loading screen raises the
// tutorial's death event in the effect replay, and the tutorial answers
// with its question mark (and keeps the memo that it did). The death sends its client
// nothing, so the EnterWorld burst shows the mark right after its Die
// frame, ahead of the burst's own tutorial event.
func TestTutorialDeathMarkOnTheLoadingScreenShownInTheBurst(t *testing.T) {
	t.Parallel()
	const (
		poisonID   = 4082
		savedHP    = 12
		deathMark  = 8
		killingDue = 10 * time.Second // the third 5 HP tick, 8s after the restore
	)
	poison := modelskill.Definition{
		ID: poisonID, Level: 1, Activation: modelskill.ActivationActive, Target: modelskill.TargetOne,
		HitTime: 500, StaticHitTime: true, SkillType: "POISON", Debuff: true, KillByDOT: true,
		Effects: []modelskill.EffectTemplate{{
			Name: "DamOverTime", Count: 10, Time: 3, Value: 5,
			StackType: "poison", StackOrder: 1, EffectType: "POISON", Icon: true,
		}},
	}
	heard := &tutorialLog{}
	hook := func(_ *script.Script, e script.Event) string {
		heard.add(e.Name)
		if e.Name == "CE30" {
			e.Player.ShowQuestionMark(deathMark)
		}
		return ""
	}
	path := "script.feature." + tutorial
	db := sqltest.SharedDB(t)
	srv := gameservertest.Boot(t,
		gameservertest.WithCharacter("Newbie", 1, 0),
		gameservertest.WithWantChars(1),
		gameservertest.WithSkills(skillstate.NewPersistence(gamesql.NewSkillSaveStore(db),
			modelskill.NewTable([]modelskill.Definition{poison}), gamesql.NewCharacterSkillStore(db))),
		gameservertest.WithScripts([]script.Listing{{Path: path}}, script.Catalog{
			path: func() script.Script { return script.Script{QuestID: -1, Hooks: script.Hooks{OnEvent: hook}} },
		}),
	)
	if !srv.DrivesClock() {
		t.Skip("holding the loading screen for a set time needs the driven clock")
	}
	objID := srv.SoleObjectID(t)
	insertJournal(t, srv.DB, journalRow{objID, tutorial, questlog.KeyState, val("STARTED")})
	ctx := context.Background()
	if _, err := srv.DB.ExecContext(ctx, `UPDATE characters SET curHp = ? WHERE obj_Id = ?`, savedHP, objID); err != nil {
		t.Fatalf("seed saved HP: %v", err)
	}
	if _, err := srv.DB.ExecContext(ctx,
		`INSERT INTO character_skills_save (char_obj_id, skill_id, skill_level, effect_count, effect_cur_time, reuse_delay, systime, restore_type, class_index, buff_index) VALUES (?,?,?,?,?,?,?,?,?,?)`,
		objID, poisonID, 1, 10, 1, 0, 0, 0, 0, 1); err != nil {
		t.Fatalf("seed saved poison: %v", err)
	}

	c := srv.Client
	c.Send(encodeRequestGameStart(0))
	readUntil(t, c, serverpackets.OpcodeCharSelected)
	srv.Advance(t, killingDue)
	if frame := c.ReadWithTimeout(300 * time.Millisecond); frame != nil {
		t.Fatalf("frame %#x sent on the loading screen, want nothing before the EnterWorld burst", frame[0])
	}

	c.Send(encodeSingleOpcode(clientpackets.OpcodeEnterWorld))
	frames := srv.ReadQueued(t, c)
	if got := heard.take(); !slices.Contains(got, "CE30") {
		t.Fatalf("tutorial heard %q, want its death event CE30", got)
	}
	die := slices.IndexFunc(frames, func(f []byte) bool { return f[0] == serverpackets.OpcodeDie })
	mark := slices.IndexFunc(frames, func(f []byte) bool {
		return f[0] == serverpackets.OpcodeTutorialShowQuestionMark && len(f) == 5 && f[1] == deathMark
	})
	coolTime := slices.IndexFunc(frames, func(f []byte) bool { return f[0] == serverpackets.OpcodeSkillCoolTime })
	if die < 0 || mark != die+1 || coolTime < mark {
		t.Fatalf("EnterWorld frames: Die at %d, death question mark at %d, SkillCoolTime at %d; want the mark right after Die", die, mark, coolTime)
	}
	if !srv.PlayerDead(t, objID) {
		t.Fatal("character alive after EnterWorld, want it dead")
	}
}
