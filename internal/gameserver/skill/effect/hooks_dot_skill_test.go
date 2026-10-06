package effect

import (
	"fmt"
	"slices"
	"testing"
	"time"

	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
)

// skillDOTEffectTarget is a target whose hits name their skill.
type skillDOTEffectTarget struct {
	*liveEffectTarget
}

func (t skillDOTEffectTarget) ReduceHPBySkillDOT(damage float64, effector Actor, sk modelskill.Ref) {
	t.hp -= damage
	t.events = append(t.events, fmt.Sprintf("skill-dot:%g:%d-%d", damage, sk.ID, sk.Level))
}

// A damage-over-time tick on a target whose hits name their skill names
// the effect's skill and level.
func TestDamageOverTimeTickNamesItsSkill(t *testing.T) {
	now := time.Unix(5000, 0)
	var events []string
	list := catchUpList(now, &events)
	target := skillDOTEffectTarget{&liveEffectTarget{hp: 100, list: list}}

	ApplyRestored(list, target, target, Skill{ID: 84, Level: 3}, restoredPoison(10, 1), 10, 0, now.Add(-2*time.Second))

	if want := []string{"skill-dot:5:84-3", "skill-dot:5:84-3"}; !slices.Equal(target.events, want) {
		t.Fatalf("ticks = %q, want %q", target.events, want)
	}
}
