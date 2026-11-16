package zone

import (
	"slices"
	"sync/atomic"
	"testing"
	"time"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
)

// racingStub is a memberStub whose own move (beforeReplay) lands just
// before a zone switch's replay takes its membership.
type racingStub struct {
	memberStub
	beforeReplay func()
}

func (s *racingStub) ReplayZoneRules(fn func()) {
	if s.beforeReplay != nil {
		s.beforeReplay()
		s.beforeReplay = nil
	}
	s.Member.ReplayZoneRules(fn)
}

// wantHolds fails unless a holds each of flags exactly want times.
func wantHolds(t *testing.T, a Actor, want int32, flags ...Flag) {
	t.Helper()
	f := a.ZoneFlags()
	f.mu.RLock()
	defer f.mu.RUnlock()
	for _, flag := range flags {
		if got := f.counts[flag]; got != want {
			t.Errorf("%v holds = %d, want %d", flag, got, want)
		}
	}
}

var battlefieldFlags = []Flag{FlagPvP, FlagSiege, FlagNoSummonFriend}

// TestSiegeStartBetweenAdmitAndEntryRaisesOnce: a siege starting after an
// actor was admitted but before its entry rules ran raises the battlefield
// holds once, so its exit leaves none.
func TestSiegeStartBetweenAdmitAndEntryRaisesOnce(t *testing.T) {
	siege := NewSiege(1, testCuboid(t, -100, 100, -100, 100))
	a := &memberStub{id: 1, class: ClassPlayer}
	siege.OnEnter(func(Actor) { siege.SetActive(true) })

	Revalidate(siege, a)
	wantHolds(t, a, 1, battlefieldFlags...)

	a.at = location.Location{X: 500}
	Revalidate(siege, a)
	wantHolds(t, a, 0, battlefieldFlags...)
}

// TestSiegeSwitchRacingExitStrandsNothing: an occupant the switch's
// snapshot still lists but that left before its replay keeps no
// battlefield hold, whether the siege starts or stops.
func TestSiegeSwitchRacingExitStrandsNothing(t *testing.T) {
	for _, start := range []bool{true, false} {
		siege := NewSiege(1, testCuboid(t, -100, 100, -100, 100))
		siege.SetActive(!start)
		a := &racingStub{memberStub: memberStub{id: 1, class: ClassPlayer}}
		Revalidate(siege, a)
		a.beforeReplay = func() {
			a.at = location.Location{X: 500}
			Revalidate(siege, a)
		}

		siege.SetActive(start)
		if siege.Inside(a) {
			t.Fatal("actor still inside after leaving")
		}
		wantHolds(t, a, 0, battlefieldFlags...)
	}
}

// TestSiegeStopReleasesOnlyItsOwnHolds: stopping a siege, even twice,
// releases only what it raised, leaving an overlapping arena's holds.
func TestSiegeStopReleasesOnlyItsOwnHolds(t *testing.T) {
	siege := NewSiege(1, testCuboid(t, -100, 100, -100, 100))
	arena := NewArena(2, testCuboid(t, -100, 100, -100, 100))
	a := &memberStub{id: 1, class: ClassPlayer}
	Revalidate(arena, a)
	Revalidate(siege, a)

	siege.SetActive(true)
	wantHolds(t, a, 2, FlagPvP)
	siege.SetActive(true)
	wantHolds(t, a, 2, FlagPvP)
	siege.SetActive(false)
	siege.SetActive(false)
	wantHolds(t, a, 1, FlagPvP, FlagNoSummonFriend)
	wantHolds(t, a, 0, FlagSiege)
}

// TestSiegeSwitchTellsEachChangeOnce: the combat notices follow the holds:
// one entry notice as the siege starts, none for a repeated start, one
// leave notice as it stops.
func TestSiegeSwitchTellsEachChangeOnce(t *testing.T) {
	siege := NewSiege(1, testCuboid(t, -100, 100, -100, 100))
	var notices []bool
	siege.CombatNotice = func(_ Actor, entering bool) { notices = append(notices, entering) }
	a := &memberStub{id: 1, class: ClassPlayer}
	Revalidate(siege, a)

	siege.SetActive(true)
	siege.SetActive(true)
	siege.SetActive(false)
	if want := []bool{true, false}; !slices.Equal(notices, want) {
		t.Fatalf("notices = %v, want %v", notices, want)
	}
}

// TestOlympiadSwitchInterleavings is the stadium twin: a fight starting
// between an actor's admission and its entry rules raises the ring's combat
// once, and an occupant leaving before the start's or end's replay keeps
// none.
func TestOlympiadSwitchInterleavings(t *testing.T) {
	t.Run("start between admit and entry", func(t *testing.T) {
		stadium := NewOlympiad(1, testCuboid(t, -100, 100, -100, 100))
		var running atomic.Bool
		stadium.BattleStarted = running.Load
		stadium.OnEnter(func(Actor) {
			running.Store(true)
			stadium.UpdateCombatStatus()
		})
		a := &memberStub{id: 1, class: ClassPlayer}

		Revalidate(stadium, a)
		wantHolds(t, a, 1, FlagPvP)
		a.at = location.Location{X: 500}
		Revalidate(stadium, a)
		wantHolds(t, a, 0, FlagPvP, FlagNoSummonFriend, FlagNoRestart)
	})
	for _, start := range []bool{true, false} {
		stadium := NewOlympiad(1, testCuboid(t, -100, 100, -100, 100))
		var running atomic.Bool
		running.Store(!start)
		stadium.BattleStarted = running.Load
		a := &racingStub{memberStub: memberStub{id: 1, class: ClassPlayer}}
		Revalidate(stadium, a)
		a.beforeReplay = func() {
			a.at = location.Location{X: 500}
			Revalidate(stadium, a)
		}

		running.Store(start)
		stadium.UpdateCombatStatus()
		wantHolds(t, a, 0, FlagPvP, FlagNoSummonFriend, FlagNoRestart)
	}
}

// TestReloadKeepsHoldsNoZoneRaised: a zone reload drops the holds the
// swapped-out zones raised but keeps those no zone raised, until they are
// released.
func TestReloadKeepsHoldsNoZoneRaised(t *testing.T) {
	ix := NewIndex()
	ix.Add(NewPeace(1, testCuboid(t, -100, 100, -100, 100)))
	a := &memberStub{id: 1, class: ClassPlayer}
	a.Enter(ix, a)
	a.ZoneFlags().Hold(FlagNoSummonFriend, true)

	ix.Swap(NewIndex())
	a.Reload(ix, a)
	if f := a.ZoneFlags(); f.Has(FlagPeace) || !f.Has(FlagNoSummonFriend) {
		t.Fatalf("after the reload peace %v, no-summon-friend %v; want only the held no-summon-friend", f.Has(FlagPeace), f.Has(FlagNoSummonFriend))
	}

	a.ZoneFlags().Hold(FlagPeace, true)
	a.ZoneFlags().Hold(FlagPeace, false)
	a.ZoneFlags().Hold(FlagPeace, false)
	a.ZoneFlags().Hold(FlagNoSummonFriend, false)
	if f := a.ZoneFlags(); f.Has(FlagPeace) || f.Has(FlagNoSummonFriend) {
		t.Fatal("released holds still read as active")
	}
}

// TestBossHandOverCarriesAndForwardsPermissions: a boss zone handed over
// gives its successor the permissions it holds as restored ones, their
// entry window lapsed, and forwards every later grant, window included, and
// revocation.
func TestBossHandOverCarriesAndForwardsPermissions(t *testing.T) {
	now := time.Unix(1_000, 0)
	old := NewBoss(1, testCuboid(t, -100, 100, -100, 100))
	old.now = func() time.Time { return now }
	next := NewBoss(1, testCuboid(t, -100, 100, -100, 100))
	next.now = old.now
	old.AllowEntry(7, time.Minute)
	old.AllowEntry(8, time.Minute)

	old.HandOver(next)
	old.AllowEntry(9, time.Minute)
	old.RevokeEntry(8)

	got := next.AllowedPlayers()
	slices.Sort(got)
	if !slices.Equal(got, []int32{7, 9}) {
		t.Fatalf("successor allows %v, want [7 9]", got)
	}
	if !next.consumeEntry(9) {
		t.Error("the grant forwarded to the successor lost its entry window")
	}
	if next.consumeEntry(7) {
		t.Error("the carried permission kept its entry window")
	}
}
