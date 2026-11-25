package zone

import (
	"fmt"
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
)

// besiegerStub is a player occupant taking part in a siege while
// siegeState is not 0.
type besiegerStub struct {
	memberStub
	siegeState int32
}

func (s *besiegerStub) SiegeState() int32 { return s.siegeState }

// walk moves a to x and revalidates it against every zone of ks, in order.
func walk(a *besiegerStub, x int, ks ...Kind) {
	a.at = location.Location{X: x}
	for _, k := range ks {
		Revalidate(k, a)
	}
}

// TestTownExitReleasesOnlyItsOwnHolds: a town's exit gives back only the
// holds its entry raised, under every combat rule and either overlap
// order, so an overlapping peace zone keeps its peace hold (#4957).
func TestTownExitReleasesOnlyItsOwnHolds(t *testing.T) {
	// Town covers x in [-100, 100], peace [0, 200].
	const townOnly, both, peaceOnly, outside = -50, 50, 150, 500
	paths := map[string][]int{
		"town first":  {townOnly, both, peaceOnly, outside},
		"peace first": {peaceOnly, both, townOnly, outside},
	}
	for rule := range 3 {
		for _, besieger := range []bool{false, true} {
			for name, path := range paths {
				t.Run(fmt.Sprintf("rule %d besieger %v %s", rule, besieger, name), func(t *testing.T) {
					town := NewTown(1, testCuboid(t, -100, 100, -100, 100))
					town.CombatRule = rule
					peace := NewPeace(2, testCuboid(t, 0, 200, -100, 100))
					a := &besiegerStub{memberStub: memberStub{id: 1, class: ClassPlayer}}
					if besieger {
						a.siegeState = 1
					}
					townRaises := rule != 1 || !besieger
					for _, x := range path {
						walk(a, x, town, peace)
						inTown, inPeace := town.Inside(a), peace.Inside(a)
						var wantTown, wantPeace int32
						if inTown && townRaises {
							wantTown = 1
							if rule != 2 {
								wantPeace++
							}
						}
						if inPeace {
							wantPeace++
						}
						wantHolds(t, a, wantTown, FlagTown)
						wantHolds(t, a, wantPeace, FlagPeace)
					}
				})
			}
		}
	}
}

// TestTownExitFollowsItsEntryNotTheSiegeState: a player's siege state
// changing while it stands in a town does not change what the town gives
// back on its way out (#4957).
func TestTownExitFollowsItsEntryNotTheSiegeState(t *testing.T) {
	for _, joined := range []bool{true, false} {
		t.Run(fmt.Sprintf("joined siege %v", joined), func(t *testing.T) {
			town := NewTown(1, testCuboid(t, -100, 100, -100, 100))
			town.CombatRule = 1
			peace := NewPeace(2, testCuboid(t, -100, 300, -100, 100))
			a := &besiegerStub{memberStub: memberStub{id: 1, class: ClassPlayer}}
			if !joined {
				a.siegeState = 1
			}
			walk(a, 0, town, peace)
			a.siegeState = 1 - a.siegeState

			walk(a, 200, town, peace)
			wantHolds(t, a, 1, FlagPeace)
			wantHolds(t, a, 0, FlagTown)
			walk(a, 500, town, peace)
			wantHolds(t, a, 0, FlagPeace, FlagTown)
		})
	}
}

// TestSiegeExitReleasesNothingItDidNotRaise: leaving a battlefield after an
// entry while no siege ran, or after the siege stopped, leaves an
// overlapping arena's holds; a siege that started after the entry raised
// its holds and the exit gives back only those (#4957, #4841).
func TestSiegeExitReleasesNothingItDidNotRaise(t *testing.T) {
	for _, tc := range []struct {
		name    string
		atEntry bool
		after   []bool
	}{
		{"inactive entry", false, nil},
		{"stopped after entry", true, []bool{false}},
		{"started after entry", false, []bool{true}},
		{"restarted", true, []bool{false, true}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			siege := NewSiege(1, testCuboid(t, -100, 100, -100, 100))
			arena := NewArena(2, testCuboid(t, -100, 300, -100, 100))
			siege.SetActive(tc.atEntry)
			a := &besiegerStub{memberStub: memberStub{id: 1, class: ClassPlayer}}
			walk(a, 0, siege, arena)
			for _, on := range tc.after {
				siege.SetActive(on)
			}

			walk(a, 200, siege, arena)
			wantHolds(t, a, 1, FlagPvP, FlagNoSummonFriend)
			wantHolds(t, a, 0, FlagSiege)
		})
	}
}

// TestStadiumExitKeepsCombatItDidNotRaise: a fight that started after the
// entry and whose switch has not reached the occupant yet raised no ring
// combat on it, so the exit leaves an overlapping arena's (#4957).
func TestStadiumExitKeepsCombatItDidNotRaise(t *testing.T) {
	stadium := NewOlympiad(1, testCuboid(t, -100, 100, -100, 100))
	arena := NewArena(2, testCuboid(t, -100, 300, -100, 100))
	running := false
	wire(&fxStub{fighting: func() bool { return running }}, stadium)
	a := &besiegerStub{memberStub: memberStub{id: 1, class: ClassPlayer}}
	walk(a, 0, stadium, arena)
	running = true

	walk(a, 200, stadium, arena)
	wantHolds(t, a, 1, FlagPvP, FlagNoSummonFriend)
	wantHolds(t, a, 0, FlagNoRestart)
}

// TestDormantTrapExitKeepsOverlappingHolds: a castle trap dormant on entry
// raised no danger or swamp hold, so its exit, even once the trap woke,
// leaves an overlapping live trap's (#4957).
func TestDormantTrapExitKeepsOverlappingHolds(t *testing.T) {
	for _, wakes := range []bool{false, true} {
		t.Run(fmt.Sprintf("wakes %v", wakes), func(t *testing.T) {
			siegeRunning := false
			fx := &fxStub{siegeRunning: func() bool { return siegeRunning }}
			castleDamage := NewDamage(1, testCuboid(t, -100, 100, -100, 100))
			castleDamage.CastleID, castleDamage.Armed = 1, true
			damage := NewDamage(2, testCuboid(t, -100, 300, -100, 100))
			castleSwamp := NewSwamp(3, testCuboid(t, -100, 100, -100, 100))
			castleSwamp.CastleID, castleSwamp.Armed = 1, true
			swamp := NewSwamp(4, testCuboid(t, -100, 300, -100, 100))
			ks := []Kind{castleDamage, damage, castleSwamp, swamp}
			wire(fx, ks...)
			a := &besiegerStub{memberStub: memberStub{id: 1, class: ClassPlayer}}

			walk(a, 0, ks...)
			wantHolds(t, a, 1, FlagDanger, FlagSwamp)
			siegeRunning = wakes
			walk(a, 200, ks...)
			wantHolds(t, a, 1, FlagDanger, FlagSwamp)
			walk(a, 500, ks...)
			wantHolds(t, a, 0, FlagDanger, FlagSwamp)
		})
	}
}
