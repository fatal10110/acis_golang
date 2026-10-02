package skills

import (
	"context"
	"testing"
	"time"

	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
)

// loadingDebuffID is a saved five-tick, 15s-period debuff shaped like
// Ultimate Debuff (4694): a Debuff effect, whose tick runs no action.
const (
	loadingDebuffID     = 4694
	loadingDebuffTicks  = 5
	loadingDebuffPeriod = 15
)

func loadingDebuffDef() modelskill.Definition {
	return modelskill.Definition{
		ID: loadingDebuffID, Level: 1, Activation: modelskill.ActivationActive, Target: modelskill.TargetOne,
		HitTime: 500, ReuseDelay: 60_000, StaticHitTime: true, StaticReuse: true, SkillType: "DEBUFF", Debuff: true,
		Effects: []modelskill.EffectTemplate{{
			Name: "Debuff", Count: loadingDebuffTicks, Time: loadingDebuffPeriod,
			StackType: "ultimate_debuff", StackOrder: 1, Icon: true,
		}},
	}
}

// bootSavedDebuff boots a character with a saved loadingDebuff row, every
// tick left and none of its period elapsed, and a reuse timer a minute out.
func bootSavedDebuff(t *testing.T) (*gameservertest.Server, int32) {
	t.Helper()
	srv := gameservertest.Boot(t,
		gameservertest.WithCharacter("Newbie", 5, 0),
		gameservertest.WithWantChars(1),
		gameservertest.WithCapturedLog(),
		gameservertest.WithSkills(skillPersistence(t, []modelskill.Definition{loadingDebuffDef()})),
	)
	if !srv.DrivesClock() {
		t.Skip("holding the loading screen for a set time needs the driven clock")
	}
	objID := srv.SoleObjectID(t)
	if _, err := srv.DB.ExecContext(context.Background(),
		`INSERT INTO character_skills_save (char_obj_id, skill_id, skill_level, effect_count, effect_cur_time, reuse_delay, systime, restore_type, class_index, buff_index) VALUES (?,?,?,?,?,?,?,?,?,?)`,
		objID, loadingDebuffID, 1, loadingDebuffTicks, 0, 60_000, time.Now().Add(time.Minute).UnixMilli(), 0, 0, 1); err != nil {
		t.Fatalf("seed saved debuff: %v", err)
	}
	return srv, objID
}

// TestRestoredDebuffEndsOnItsFirstLoadingScreenTick: a restored effect
// whose tick runs no action ends on its first tick whatever count it has
// left, as the reference's scheduleEffect ACTING branch and the live
// effect list both end an in-use effect whose tick action reports false.
// A loading screen held past that tick enters the world without the
// debuff, and a drop after it saves only the reuse timer; one shorter than
// the period still enters with it.
func TestRestoredDebuffEndsOnItsFirstLoadingScreenTick(t *testing.T) {
	t.Parallel()
	const pastFirstTick = (loadingDebuffPeriod + 5) * time.Second
	t.Run("enter world before its first tick", func(t *testing.T) {
		t.Parallel()
		srv, _ := bootSavedDebuff(t)
		c := srv.Client
		selectOnly(t, c)
		srv.Advance(t, loadingScreen)
		c.Send(encodeEnterWorld())
		frames := readEnterWorldBurstWithRestoredBuff(t, c)
		entries := readAbnormalStatusUpdateEntriesFromFrame(t, frames[3])
		if len(entries) != 1 || entries[0].SkillID != loadingDebuffID {
			t.Fatalf("EnterWorld AbnormalStatusUpdate after a %v loading screen = %+v, want the restored debuff", loadingScreen, entries)
		}
	})
	t.Run("enter world past its first tick", func(t *testing.T) {
		t.Parallel()
		srv, _ := bootSavedDebuff(t)
		c := srv.Client
		selectOnly(t, c)
		srv.Advance(t, pastFirstTick)
		c.Send(encodeEnterWorld())
		frames := readEnterWorldBurst(t, c)
		coolTimes := readSkillCoolTimeEntriesFromFrame(t, frames[len(frames)-2])
		if len(coolTimes) != 1 || coolTimes[0].SkillID != loadingDebuffID {
			t.Fatalf("SkillCoolTime = %+v, want the debuff's reuse timer", coolTimes)
		}
	})
	t.Run("drop past its first tick", func(t *testing.T) {
		t.Parallel()
		srv, objID := bootSavedDebuff(t)
		c := srv.Client
		selectOnly(t, c)
		srv.Advance(t, pastFirstTick)
		dropOnLoadingScreen(t, srv, c, objID)
		if count, restoreType := skillSaveRow(t, srv, objID, loadingDebuffID, 1); count != 1 || restoreType != 1 {
			t.Fatalf("saved rows after the debuff's first tick passed on the loading screen = %d (restore_type %d), want its reuse timer alone (restore_type 1)", count, restoreType)
		}
	})
}
