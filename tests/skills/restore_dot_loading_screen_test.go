package skills

import (
	"context"
	"testing"
	"time"

	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
)

// loadingPoisonID is a saved ten-tick, 3s-period poison: a restored effect
// whose ticks deal damage.
const (
	loadingPoisonID     = 84
	loadingPoisonTicks  = 10
	loadingPoisonPeriod = 3
	// loadingPoisonHold outlasts the poison's whole 30s run.
	loadingPoisonHold = 40 * time.Second
)

func loadingPoisonDef() modelskill.Definition {
	return modelskill.Definition{
		ID: loadingPoisonID, Level: 1, Activation: modelskill.ActivationActive, Target: modelskill.TargetOne,
		HitTime: 500, ReuseDelay: 60_000, StaticHitTime: true, StaticReuse: true, SkillType: "POISON",
		Effects: []modelskill.EffectTemplate{{
			Name: "DamOverTime", Count: loadingPoisonTicks, Time: loadingPoisonPeriod, Value: 5,
			StackType: "poison", StackOrder: 1, EffectType: "POISON", Icon: true,
		}},
	}
}

// bootSavedPoison boots a character with a saved loadingPoison row, all
// ticks left and elapsed seconds into its period.
func bootSavedPoison(t *testing.T, elapsed int32) (*gameservertest.Server, int32) {
	t.Helper()
	srv := gameservertest.Boot(t,
		gameservertest.WithCharacter("Newbie", 5, 0),
		gameservertest.WithWantChars(1),
		gameservertest.WithCapturedLog(),
		gameservertest.WithSkills(skillPersistence(t, []modelskill.Definition{loadingPoisonDef()})),
	)
	if !srv.DrivesClock() {
		t.Skip("holding the loading screen for a set time needs the driven clock")
	}
	objID := srv.SoleObjectID(t)
	if _, err := srv.DB.ExecContext(context.Background(),
		`INSERT INTO character_skills_save (char_obj_id, skill_id, skill_level, effect_count, effect_cur_time, reuse_delay, systime, restore_type, class_index, buff_index) VALUES (?,?,?,?,?,?,?,?,?,?)`,
		objID, loadingPoisonID, 1, loadingPoisonTicks, elapsed, 60_000, time.Now().Add(time.Minute).UnixMilli(), 0, 0, 1); err != nil {
		t.Fatalf("seed saved poison: %v", err)
	}
	return srv, objID
}

// TestRestoredPoisonOutlastsAHeldLoadingScreen: ticks of a restored
// damage-over-time effect that fall due on the loading screen cannot run
// their damage there, so they must not be spent either. A loading screen
// held past the poison's whole run still enters the world poisoned, with
// every tick left, rather than cleansed for free.
func TestRestoredPoisonOutlastsAHeldLoadingScreen(t *testing.T) {
	t.Parallel()
	const elapsed = 1
	srv, _ := bootSavedPoison(t, elapsed)
	c := srv.Client
	selectOnly(t, c)
	srv.Advance(t, loadingPoisonHold)
	c.Send(encodeEnterWorld())
	frames := readEnterWorldBurstWithRestoredBuff(t, c)

	entries := readAbnormalStatusUpdateEntriesFromFrame(t, frames[3])
	if len(entries) != 1 || entries[0].SkillID != loadingPoisonID {
		t.Fatalf("EnterWorld AbnormalStatusUpdate after a %v loading screen = %+v, want the restored poison", loadingPoisonHold, entries)
	}
	if min := int32(loadingPoisonTicks*loadingPoisonPeriod - 2*loadingPoisonPeriod); entries[0].Duration < min {
		t.Fatalf("restored poison duration at EnterWorld = %ds, want at least %ds: the loading screen spent its ticks", entries[0].Duration, min)
	}
}

// TestRestoredPoisonDropOnLoadingScreenSavesEveryTick: a drop after the
// same hold saves the poison as it was read back, so the next login
// restores it whole.
func TestRestoredPoisonDropOnLoadingScreenSavesEveryTick(t *testing.T) {
	t.Parallel()
	const elapsed = 1
	srv, objID := bootSavedPoison(t, elapsed)
	c := srv.Client
	selectOnly(t, c)
	srv.Advance(t, loadingPoisonHold)
	dropOnLoadingScreen(t, srv, c, objID)

	var count, curTime int32
	if err := srv.DB.QueryRowContext(context.Background(),
		`SELECT effect_count, effect_cur_time FROM character_skills_save WHERE char_obj_id = ? AND skill_id = ? AND restore_type = 0`,
		objID, loadingPoisonID).Scan(&count, &curTime); err != nil {
		t.Fatalf("read the saved poison back after a drop on the loading screen: %v", err)
	}
	if count != loadingPoisonTicks || curTime != elapsed {
		t.Fatalf("saved poison after a drop on the loading screen = count %d time %d, want the read-back count %d time %d", count, curTime, loadingPoisonTicks, elapsed)
	}
}
