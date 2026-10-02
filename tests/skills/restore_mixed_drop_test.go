package skills

import (
	"context"
	"testing"
	"time"

	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
)

// loadingRootPoisonID is a saved skill shaped like Poison of Death (4082):
// a Root, whose tick runs no action, ahead of a DamOverTime, whose ticks
// deal damage.
const (
	loadingRootPoisonID     = 4082
	loadingRootPoisonRoot   = 20
	loadingRootPoisonTicks  = 10
	loadingRootPoisonPeriod = 3
)

func loadingRootPoisonDef() modelskill.Definition {
	return modelskill.Definition{
		ID: loadingRootPoisonID, Level: 1, Activation: modelskill.ActivationActive, Target: modelskill.TargetOne,
		HitTime: 500, ReuseDelay: 60_000, StaticHitTime: true, StaticReuse: true, SkillType: "POISON", Debuff: true,
		Effects: []modelskill.EffectTemplate{
			{Name: "Root", Count: 1, Time: loadingRootPoisonRoot, StackType: "root", StackOrder: 1, Icon: true},
			{Name: "DamOverTime", Count: loadingRootPoisonTicks, Time: loadingRootPoisonPeriod, Value: 5, StackType: "poison", StackOrder: 1, EffectType: "POISON", Icon: true},
		},
	}
}

// TestRestoredMixedSkillDropKeepsItsRunningPoison: once the Root of a
// restored Root-then-poison skill ends on the loading screen, a drop saves
// the poison behind it, as the reference writes the first effect of a skill
// still in the list. The whole row is not dropped, so the poison is not
// cleared by holding the loading screen past the Root and disconnecting.
func TestRestoredMixedSkillDropKeepsItsRunningPoison(t *testing.T) {
	t.Parallel()
	const elapsed = 1
	srv := gameservertest.Boot(t,
		gameservertest.WithCharacter("Newbie", 5, 0),
		gameservertest.WithWantChars(1),
		gameservertest.WithCapturedLog(),
		gameservertest.WithSkills(skillPersistence(t, []modelskill.Definition{loadingRootPoisonDef()})),
	)
	if !srv.DrivesClock() {
		t.Skip("holding the loading screen for a set time needs the driven clock")
	}
	objID := srv.SoleObjectID(t)
	if _, err := srv.DB.ExecContext(context.Background(),
		`INSERT INTO character_skills_save (char_obj_id, skill_id, skill_level, effect_count, effect_cur_time, reuse_delay, systime, restore_type, class_index, buff_index) VALUES (?,?,?,?,?,?,?,?,?,?)`,
		objID, loadingRootPoisonID, 1, loadingRootPoisonTicks, elapsed, 60_000, time.Now().Add(time.Minute).UnixMilli(), 0, 0, 1); err != nil {
		t.Fatalf("seed saved root poison: %v", err)
	}
	c := srv.Client
	selectOnly(t, c)
	srv.Advance(t, (loadingRootPoisonRoot+5)*time.Second)
	dropOnLoadingScreen(t, srv, c, objID)

	var count, curTime int32
	if err := srv.DB.QueryRowContext(context.Background(),
		`SELECT effect_count, effect_cur_time FROM character_skills_save WHERE char_obj_id = ? AND skill_id = ? AND restore_type = 0`,
		objID, loadingRootPoisonID).Scan(&count, &curTime); err != nil {
		t.Fatalf("read the saved root poison back after a drop past its root: %v", err)
	}
	if count != loadingRootPoisonTicks || curTime != elapsed {
		t.Fatalf("saved root poison after a drop past its root = count %d time %d, want the poison's read-back count %d time %d", count, curTime, loadingRootPoisonTicks, elapsed)
	}
}
