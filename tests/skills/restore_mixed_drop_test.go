package skills

import (
	"testing"
	"time"

	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
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
// cleared by holding the loading screen past the Root and disconnecting,
// and the poison's ticks due on the loading screen have run their damage.
func TestRestoredMixedSkillDropKeepsItsRunningPoison(t *testing.T) {
	t.Parallel()
	const elapsed = 1
	srv, objID := bootSavedPoison(t, loadingRootPoisonDef(), loadingRootPoisonTicks, elapsed)
	c := srv.Client
	selectOnly(t, c)
	// The Root ends 19s after the restore; the poison's ticks fall due 2s,
	// 5s, ... 23s after it: eight by 25s, the last 2s before the drop.
	srv.Advance(t, (loadingRootPoisonRoot+5)*time.Second)
	dropOnLoadingScreen(t, srv, c, objID)

	count, curTime, ok := savedEffectRow(t, srv, objID, loadingRootPoisonID)
	if !ok || count != loadingRootPoisonTicks-8 || curTime != 2 {
		t.Fatalf("saved root poison after a drop past its root = count %d time %d (row %v), want the poison's count %d time 2",
			count, curTime, ok, loadingRootPoisonTicks-8)
	}
	if hp, want := savedHP(t, srv, objID), float64(loadingPoisonHP-8*loadingPoisonDamage); hp != want {
		t.Fatalf("saved HP after the drop = %g, want %g: eight ticks' damage", hp, want)
	}
}
