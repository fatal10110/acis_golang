package effect

import (
	"slices"
	"strings"
	"testing"
	"time"

	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
)

// An effect restored at the replay instant itself (a duel's end puts the
// pre-duel effects back that way) with its tick saved overdue runs nothing
// in the replay: the tick is left to the effects sweep, so the replay of a
// character in the world takes no removal into its quiet catch-up.
func TestApplyRestoredAtTheReplayRunsNoOverdueTick(t *testing.T) {
	now := time.Unix(5000, 0)
	var events []string
	list := catchUpList(now, &events)
	target := &liveEffectTarget{hp: 100, list: list}

	ApplyRestored(list, target, target, Skill{ID: 84, Level: 1}, restoredPoison(10, 3), 10, 3, now)

	if target.hp != 100 || damageTicks(target.events) != 0 {
		t.Fatalf("HP after the replay = %g (%v), want 100: nothing is due at the restore instant", target.hp, target.events)
	}
	if list.catchingUp.Load() != 0 {
		t.Fatalf("catchingUp = %d after the replay, want 0", list.catchingUp.Load())
	}
}

// The ticks due on the loading screen run in time order across every effect
// the replay restores, not one effect's ticks after another's: the
// reference runs each restored effect on its own fixed-rate task from the
// restore instant. A 3s poison saved first and a 2s poison saved second,
// restored 4.5s before the replay, tick at 2s (second), 3s (first) and 4s
// (second); one icon refresh covers the whole replay.
func TestRestoreRunsTheDueTicksOfAllEffectsInTimeOrder(t *testing.T) {
	now := time.Unix(5000, 0)
	var events []string
	list := catchUpList(now, &events)
	target := &liveEffectTarget{hp: 100, list: list}
	restoredAt := now.Add(-4500 * time.Millisecond)

	slow := []modelskill.EffectTemplate{{Name: "DamOverTime", Count: 10, Time: 3, Value: 5, StackType: "slow_poison", StackOrder: 1, Icon: true}}
	fast := []modelskill.EffectTemplate{{Name: "DamOverTime", Count: 10, Time: 2, Value: 7, StackType: "fast_poison", StackOrder: 1, Icon: true}}
	list.Restore(func() {
		ApplyRestored(list, target, target, Skill{ID: 84, Level: 1}, slow, 10, 0, restoredAt)
		ApplyRestored(list, target, target, Skill{ID: 85, Level: 1}, fast, 10, 0, restoredAt)
	})

	var dots []string
	for _, ev := range target.events {
		if parts := strings.SplitN(ev, ":", 3); parts[0] == "dot" && len(parts) > 1 {
			dots = append(dots, parts[0]+":"+parts[1])
		}
	}
	if want := []string{"dot:7", "dot:5", "dot:7"}; !slices.Equal(dots, want) {
		t.Fatalf("damage ticks run by the replay = %v, want %v: the fast poison's 2s tick, the slow poison's 3s tick, then the fast one's 4s tick", dots, want)
	}
	if !slices.Equal(events, []string{"owner:add", "owner:add", "icons"}) {
		t.Fatalf("owner events = %v, want both activations and one icon refresh for the replay", events)
	}
}
