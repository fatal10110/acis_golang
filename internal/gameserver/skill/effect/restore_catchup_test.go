package effect

import (
	"slices"
	"strings"
	"testing"
	"time"

	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/sim"
)

// restoredPoison is a saved poison: a damage-over-time effect whose ticks
// each take 5 HP.
func restoredPoison(count, period int) []modelskill.EffectTemplate {
	return []modelskill.EffectTemplate{{
		Name: "DamOverTime", Count: count, Time: period, Value: 5,
		StackType: "poison", StackOrder: 1, Icon: true,
	}}
}

// catchUpList is a list whose clock reads now, owned by an owner that
// records its icon refreshes and messages into events.
func catchUpList(now time.Time, events *[]string) *List {
	l := NewList(iconEventOwner{eventOwner{events: events}})
	l.SetQueue(sim.NewInline(now).NewQueue("test"))
	return l
}

func damageTicks(events []string) int {
	n := 0
	for _, ev := range events {
		if strings.HasPrefix(ev, "dot:") {
			n++
		}
	}
	return n
}

// A poison restored 3s before its replay, with a 1s period and no time into
// it, has had three ticks due on the loading screen: the replay runs their
// damage, in order, before the one icon refresh, and the poison keeps the
// seven ticks left (#3266).
func TestApplyRestoredRunsTheTicksDueOnTheLoadingScreen(t *testing.T) {
	now := time.Unix(5000, 0)
	var events []string
	list := catchUpList(now, &events)
	target := &liveEffectTarget{hp: 100, list: list}

	ApplyRestored(list, target, target, Skill{ID: 84, Level: 1}, restoredPoison(10, 1), 10, 0, now.Add(-3*time.Second))

	if target.hp != 85 {
		t.Fatalf("HP after the replay = %g, want 85: three 5 HP ticks came due on the loading screen", target.hp)
	}
	if got := damageTicks(target.events); got != 3 {
		t.Fatalf("damage ticks run by the replay = %d, want 3 (%v)", got, target.events)
	}
	held := list.All()
	if len(held) != 1 || held[0].Remaining() != 7 {
		t.Fatalf("list after the replay = %d effects, want the poison with 7 ticks left", len(held))
	}
	if count, elapsed := held[0].SaveState(now); count != 7 || elapsed != 0 {
		t.Fatalf("poison SaveState right after the replay = (%d, %d), want (7, 0)", count, elapsed)
	}
	if !slices.Equal(events, []string{"owner:add", "icons"}) {
		t.Fatalf("owner events = %v, want the activation and the one icon refresh, no message", events)
	}
}

// A loading screen held past a poison's whole run ends it with every tick's
// damage taken: holding it cannot clear the poison for free. The poison
// leaves the list silently, before the replay's icon refresh.
func TestApplyRestoredEndsAPoisonWhoseWholeRunPassedWithItsDamage(t *testing.T) {
	now := time.Unix(5000, 0)
	var events []string
	list := catchUpList(now, &events)
	target := &liveEffectTarget{hp: 100, list: list}

	ApplyRestored(list, target, target, Skill{ID: 84, Level: 1}, restoredPoison(10, 3), 10, 1, now.Add(-40*time.Second))

	if target.hp != 50 {
		t.Fatalf("HP after the replay = %g, want 50: all ten 5 HP ticks came due on the loading screen", target.hp)
	}
	if held := list.All(); len(held) != 0 {
		t.Fatalf("list after the replay holds %d effects, want the ended poison gone", len(held))
	}
	if !slices.Equal(events, []string{"owner:add", "owner:remove:DamOverTime", "icons"}) {
		t.Fatalf("owner events = %v, want the activation, the silent removal, then the one icon refresh", events)
	}
}

// A restored poison from a skill whose damage-over-time may kill still runs
// from the replay: none of its ticks run on the loading screen (#3394).
func TestApplyRestoredLeavesALethalPoisonToTheReplay(t *testing.T) {
	now := time.Unix(5000, 0)
	var events []string
	list := catchUpList(now, &events)
	target := &liveEffectTarget{hp: 100, list: list}

	ApplyRestored(list, target, target, Skill{ID: 4082, Level: 1, KillByDOT: true}, restoredPoison(10, 3), 10, 1, now.Add(-40*time.Second))

	if target.hp != 100 || damageTicks(target.events) != 0 {
		t.Fatalf("HP after the replay = %g (%v), want 100: a lethal poison waits for the replay", target.hp, target.events)
	}
	held := list.All()
	if len(held) != 1 || held[0].Remaining() != 10 {
		t.Fatalf("list after the replay = %d effects, want the lethal poison with all 10 ticks left", len(held))
	}
}

// An action that ends the effect on its tick ends it in the catch-up, as a
// live tick does: a dead character's poison stops at its first due tick.
func TestApplyRestoredEndsAnEffectWhoseActionReportsFalse(t *testing.T) {
	now := time.Unix(5000, 0)
	var events []string
	list := catchUpList(now, &events)
	target := &liveEffectTarget{hp: 100, dead: true, list: list}

	ApplyRestored(list, target, target, Skill{ID: 84, Level: 1}, restoredPoison(10, 1), 10, 0, now.Add(-3*time.Second))

	if held := list.All(); len(held) != 0 {
		t.Fatalf("list after the replay holds %d effects, want the poison ended by its first tick", len(held))
	}
	if damageTicks(target.events) != 0 {
		t.Fatalf("damage on a dead character: %v", target.events)
	}
}
