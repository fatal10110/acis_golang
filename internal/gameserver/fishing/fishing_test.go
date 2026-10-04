package fishing

import (
	"testing"
	"time"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/fish"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/item"
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

// TestFishTypeTables pins every lure's type ranges, the inclusive bounds
// of FishingStance.getRandomFishType read off the reference switch: the
// check value at and just past each bound.
func TestFishTypeTables(t *testing.T) {
	for _, tc := range []struct {
		lure   int32
		checks []int
		want   []int
	}{
		{7807, []int{54, 55, 77, 78}, []int{5, 4, 4, 6}},
		{7808, []int{54, 55, 77, 78}, []int{4, 6, 6, 5}},
		{7809, []int{54, 55, 77, 78}, []int{6, 5, 5, 4}},
		{8486, []int{33, 34, 66, 67}, []int{4, 5, 5, 6}},
		{7612, []int{0, 99}, []int{3, 3}},
		{6520, []int{54, 55, 74, 75, 94, 95}, []int{1, 0, 0, 2, 2, 3}},
		{8505, []int{54, 55, 74, 75, 94, 95}, []int{1, 0, 0, 2, 2, 3}},
		{6523, []int{54, 55, 74, 75, 94, 95}, []int{0, 1, 1, 2, 2, 3}},
		{6526, []int{55, 56, 74, 75, 94, 95}, []int{2, 1, 1, 0, 0, 3}},
		{8484, []int{33, 34, 66, 67}, []int{0, 1, 1, 2}},
		{8506, []int{54, 55, 77, 78}, []int{8, 7, 7, 9}},
		{8509, []int{54, 55, 77, 78}, []int{7, 9, 9, 8}},
		{8512, []int{54, 55, 77, 78}, []int{9, 8, 8, 7}},
		{8485, []int{33, 34, 66, 67}, []int{7, 8, 8, 9}},
		// A lure no table names draws type 1 in its group.
		{8548, []int{0, 99}, []int{1, 1}},
	} {
		group := LureGroup(tc.lure)
		for i, check := range tc.checks {
			if got := fishType(group, tc.lure, check); got != tc.want[i] {
				t.Errorf("lure %d check %d: type %d, want %d", tc.lure, check, got, tc.want[i])
			}
		}
	}
}

func TestLureGroupsAndNightLures(t *testing.T) {
	for lure, group := range map[int32]int{7807: 0, 7808: 0, 7809: 0, 8486: 0, 8506: 2, 8509: 2, 8512: 2, 8485: 2, 6519: 1, 8505: 1, 8484: 1, 8548: 1} {
		if got := LureGroup(lure); got != group {
			t.Errorf("LureGroup(%d) = %d, want %d", lure, got, group)
		}
	}
	for lure, night := range map[int32]bool{8504: false, 8505: true, 8513: true, 8514: false, 8485: true, 8484: false, 6520: false} {
		if got := NightLure(lure); got != night {
			t.Errorf("NightLure(%d) = %v, want %v", lure, got, night)
		}
	}
}

// TestFishLevel pins the level spread: no expertise draws level 1 with no
// roll; otherwise checks 0-34 lower it, 35-49 raise it, the rest keep it,
// clamped to 1-27.
func TestFishLevel(t *testing.T) {
	if got := fishLevel(0, script(t)); got != 1 {
		t.Fatalf("fishLevel(0) = %d, want 1 without a roll", got)
	}
	for _, tc := range []struct{ base, check, want int }{
		{5, 34, 4}, {5, 35, 6}, {5, 49, 6}, {5, 50, 5}, {1, 0, 1}, {27, 40, 27}, {30, 99, 27},
	} {
		if got := fishLevel(tc.base, script(t, [2]int{100, tc.check})); got != tc.want {
			t.Errorf("fishLevel(%d) check %d = %d, want %d", tc.base, tc.check, got, tc.want)
		}
	}
}

// TestCheckDelay pins the look period, Math.round((float)(gutsCheckTime *
// scale)) per lure grade; the products were worked out by hand: 2000*1.33
// = 2660, 3000*0.66 = 1980, and an unnamed lure has none.
func TestCheckDelay(t *testing.T) {
	for _, tc := range []struct {
		lure int32
		guts int
		want time.Duration
	}{
		{6519, 2000, 2660 * time.Millisecond},
		{8508, 5000, 6650 * time.Millisecond},
		{6520, 3000, 3000 * time.Millisecond},
		{8507, 4000, 4000 * time.Millisecond},
		{8513, 4000, 4000 * time.Millisecond},
		{7808, 2000, 2000 * time.Millisecond},
		{6521, 3000, 1980 * time.Millisecond},
		{6527, 5000, 3300 * time.Millisecond},
		{6528, 5000, 0},
		{8548, 5000, 0},
	} {
		if got := CheckDelay(tc.lure, tc.guts); got != tc.want {
			t.Errorf("CheckDelay(%d, %d) = %v, want %v", tc.lure, tc.guts, got, tc.want)
		}
	}
}

// TestDamage pins (int)(power * (1 + grade*0.1) * shot), less 50 for a
// skill three levels above the expertise: 24*1.1 = 26.4, 37*1.2*2 = 88.8,
// 70*1.0 - 50 = 20.
func TestDamage(t *testing.T) {
	for _, tc := range []struct {
		power             float32
		rod               item.CrystalType
		shot              bool
		level, expertise  int
		damage, penalties int
	}{
		{24, item.CrystalD, false, 1, 1, 26, 0},
		{24, item.CrystalD, true, 1, 1, 52, 0},
		{37, item.CrystalC, true, 2, 2, 88, 0},
		{70, item.CrystalNone, false, 5, 2, 20, 50},
		{70, item.CrystalNone, false, 4, 2, 70, 0},
		{155, item.CrystalS, true, 27, 27, 465, 0},
	} {
		damage, penalty := Damage(tc.power, tc.rod, tc.shot, tc.level, tc.expertise)
		if damage != tc.damage || penalty != tc.penalties {
			t.Errorf("Damage(%v, %v, %v, %d, %d) = %d, %d; want %d, %d", tc.power, tc.rod, tc.shot, tc.level, tc.expertise, damage, penalty, tc.damage, tc.penalties)
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
