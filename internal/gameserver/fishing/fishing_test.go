package fishing

import (
	"testing"
	"time"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/fish"
)

// script returns a Roll answering the given values in order, failing the
// test on a bound it did not expect or a draw past the end.
func script(t *testing.T, draws ...[2]int) Roll {
	t.Helper()
	i := 0
	return func(n int) int {
		t.Helper()
		if i >= len(draws) {
			t.Fatalf("roll(%d): no draw scripted", n)
		}
		d := draws[i]
		i++
		if d[0] != n {
			t.Fatalf("draw %d: roll(%d), want roll(%d)", i, n, d[0])
		}
		return d[1]
	}
}

// TestNightLures pins the night lure id range.
func TestNightLures(t *testing.T) {
	for lure, night := range map[int32]bool{8504: false, 8505: true, 8513: true, 8514: false, 8485: true, 8484: false, 6520: false} {
		if got := NightLure(lure); got != night {
			t.Errorf("NightLure(%d) = %v, want %v", lure, got, night)
		}
	}
}

func TestPenaltyMonster(t *testing.T) {
	for level, id := range map[int]int{1: 18319, 10: 18319, 11: 18320, 76: 18325, 77: 18326, 80: 18326} {
		if got := PenaltyMonsterID(level); got != id {
			t.Errorf("PenaltyMonsterID(%d) = %d, want %d", level, got, id)
		}
	}
	if !CaughtMonster(4) || CaughtMonster(5) {
		t.Fatal("CaughtMonster must hold for checks 0-4 only")
	}
}

var testFish = fish.Fish{ID: 6411, Level: 1, HP: 100, HPRegen: 4, Type: 1, Group: 1, Guts: 500, GutsCheckTime: 5000, WaitTime: 20000, CombatTime: 24000}

// TestChooseDrawsLevelThenTypeThenRow pins the draw order: level, type,
// then the row.
func TestChooseDrawsLevelThenTypeThenRow(t *testing.T) {
	other := testFish
	other.ID = 9999
	table := fish.NewTable([]fish.Fish{testFish, {ID: 6412, Level: 1, Type: 2, Group: 1}, other})
	got, ok := Choose(table, 6520, 1, script(t, [2]int{100, 50}, [2]int{100, 54}, [2]int{2, 1}))
	if !ok || got.ID != 9999 {
		t.Fatalf("Choose = %+v, %v; want fish 9999", got, ok)
	}
	if _, ok := Choose(table, 6520, 1, script(t, [2]int{100, 0}, [2]int{100, 99})); ok {
		t.Fatal("Choose found a type 3 fish the table lacks")
	}
}

// TestStanceWaitBiteAndFight drives a line from the cast to the catch.
func TestStanceWaitBiteAndFight(t *testing.T) {
	var s Stance
	start := time.Unix(1000, 0)
	s.Cast(6520)
	s.Hook(testFish)
	s.Wait(start)
	if !s.Fishing() || s.Fighting() {
		t.Fatal("a waiting line must be fishing and not fighting")
	}
	if kind, _ := s.Look(start.Add(10*time.Second), false, script(t, [2]int{1000, 500})); kind != LookNothing {
		t.Fatalf("guts 500 vs roll 500: %v, want no bite", kind)
	}
	kind, combat := s.Look(start.Add(15*time.Second), false, script(t, [2]int{1000, 499}, [2]int{100, 80}))
	if kind != LookBite || combat != (Combat{Time: 24, HP: 100, Mode: 1, LureType: 1}) {
		t.Fatalf("bite = %v %+v", kind, combat)
	}
	// Fighting mode, no deception: the fish regains HP each second; the
	// mode roll comes every other second.
	k, g, b := s.Tick(script(t, [2]int{100, 69}))
	if k != TickGauge || b || g != (HPRegen{Time: 23, HP: 104, Mode: 1}) {
		t.Fatalf("tick 1 = %v %+v %v", k, g, b)
	}
	k, g, _ = s.Tick(script(t))
	if k != TickGauge || g != (HPRegen{Time: 22, HP: 108, Mode: 1}) {
		t.Fatalf("tick 2 = %v %+v", k, g)
	}
	// Reeling succeeds while the fish fights.
	a := s.Reel(26, 0, script(t, [2]int{100, 90}))
	if a.Message != ActionReeled || a.Damage != 26 || a.End != EndNone || a.Gauge != (HPRegen{Time: 22, HP: 82, Mode: 1, GoodUse: 1, Anim: 2}) {
		t.Fatalf("reel = %+v", a)
	}
	// Pumping fails while it fights: it regains the damage.
	a = s.Pump(26, 0, script(t, [2]int{100, 0}))
	if a.Message != ActionPumpFailed || a.Gauge != (HPRegen{Time: 22, HP: 108, Mode: 1, GoodUse: 2, Anim: 1}) {
		t.Fatalf("pump = %+v", a)
	}
	// A resisted attempt changes nothing but reports the penalty.
	a = s.Reel(26, 50, script(t, [2]int{100, 91}))
	if a.Message != ActionResisted || a.Gauge != (HPRegen{Time: 22, HP: 108, Mode: 1, Anim: 2, Penalty: 50}) {
		t.Fatalf("resisted = %+v", a)
	}
	a = s.Reel(200, 0, script(t, [2]int{100, 0}))
	if a.End != EndCaught || a.Gauge.HP != 0 {
		t.Fatalf("finishing reel = %+v", a)
	}
}

// TestStanceEnds pins the other ways a line comes out: the wait running out,
// the fish regaining twice its HP, the fight's time running out, a night
// lure by day.
func TestStanceEnds(t *testing.T) {
	start := time.Unix(1000, 0)
	newStance := func(lure int32) *Stance {
		s := &Stance{}
		s.Cast(lure)
		s.Hook(testFish)
		s.Wait(start)
		return s
	}

	s := newStance(6520)
	if kind, _ := s.Look(start.Add(30*time.Second), false, script(t)); kind != LookTimeout {
		t.Fatalf("look at the deadline = %v, want timeout", kind)
	}

	s = newStance(8505)
	if s.FishType(false) != -1 || s.FishType(true) != 1 {
		t.Fatal("a night lure shows type -1 by day only")
	}
	if kind, _ := s.Look(start.Add(10*time.Second), false, script(t)); kind != LookNothing {
		t.Fatalf("night lure by day = %v, want no bite and no roll", kind)
	}

	s = newStance(6520)
	s.Look(start, false, script(t, [2]int{1000, 0}, [2]int{100, 0}))
	if a := s.Reel(150, 0, script(t, [2]int{100, 0})); a.Message != ActionReelFailed || a.End != EndLost || a.Gauge.HP != 250 {
		t.Fatalf("failed reel past twice the HP = %+v, want lost with the gauge at 250", a)
	}

	s = newStance(6520)
	s.Look(start, false, script(t, [2]int{1000, 0}, [2]int{100, 0}))
	s.Pump(-100, 0, script(t, [2]int{100, 0}))
	if k, _, _ := s.Tick(script(t)); k != TickStolen {
		t.Fatalf("tick at twice the HP = %v, want stolen", k)
	}

	s = newStance(6520)
	s.Look(start, false, script(t, [2]int{1000, 0}, [2]int{100, 0}))
	for i := range 24 {
		if k, _, _ := s.Tick(func(int) int { return 0 }); k != TickGauge {
			t.Fatalf("tick %d = %v, want gauge", i, k)
		}
	}
	if k, _, _ := s.Tick(script(t)); k != TickSpat {
		t.Fatalf("tick past the time = %v, want spat", k)
	}
}
