package effect

import (
	"testing"
	"time"

	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/skill/conditions"
)

// admitActor is a landActor with a condition view, so an effect's attached
// <cond> can be tested against it.
type admitActor struct{ *landActor }

func (admitActor) ConditionActor() conditions.Actor { return fakeConditionActor{level: 80} }

// maxHPGatedBuff is a buff whose <effect> block carries an attached
// <skill stat="maxHp"/> condition.
func maxHPGatedBuff() []modelskill.EffectTemplate {
	return []modelskill.EffectTemplate{{
		Name: "Buff", Count: 1, Time: 1200, StackType: "gated_buff", StackOrder: 1, Icon: true,
		AttachCondition: &modelskill.ConditionClause{Root: modelskill.Condition{Kind: "skill", Attrs: map[string]string{"stat": "maxHp"}}},
	}}
}

// TestApplyCreatesOnlyEffectsTheirAttachedConditionAdmits covers #3669 for
// a direct application: an effect whose attached <cond> rejects the skill
// applying it is never created, one it admits is.
func TestApplyCreatesOnlyEffectsTheirAttachedConditionAdmits(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		stat string
		want int
	}{{"MAX_HP", 1}, {"POWER_ATTACK", 0}, {"", 0}} {
		caster := admitActor{newLandActor(1, 0)}
		target := admitActor{newLandActor(2, 0)}
		Apply(caster, target, Skill{ID: 1045, Level: 1, Stat: tc.stat}, maxHPGatedBuff())
		if got := len(target.list.All()); got != tc.want {
			t.Errorf("Apply with skill stat %q hosted %d effects, want %d", tc.stat, got, tc.want)
		}
	}
}

// TestRestoreReinstatesOnlyEffectsTheirAttachedConditionAdmits covers #3669
// for a relog replay: the self-applied reinstatement is tested with the
// saved skill like a fresh cast, so an effect whose attached <cond> rejects
// it does not come back.
func TestRestoreReinstatesOnlyEffectsTheirAttachedConditionAdmits(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		stat string
		want int
	}{{"MAX_HP", 1}, {"POWER_ATTACK", 0}, {"", 0}} {
		self := admitActor{newLandActor(1, 0)}
		ApplyRestored(self.list, self, self, Skill{ID: 1045, Level: 1, Stat: tc.stat}, maxHPGatedBuff(), 1, 0, time.Time{})
		if got := len(self.list.All()); got != tc.want {
			t.Errorf("restore with skill stat %q reinstated %d effects, want %d", tc.stat, got, tc.want)
		}
	}
}
