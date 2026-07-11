package zone

import (
	"fmt"
	"sync"
)

// Flag is one of the world-state markers zones impose on the actors inside
// them (combat rules, movement rules, shop/restart restrictions, ...). The
// numeric values are a data contract shared with game rules and scripts;
// do not reorder.
type Flag uint8

// The full flag set, in contract order.
const (
	FlagPvP Flag = iota
	FlagPeace
	FlagSiege
	FlagMotherTree
	FlagClanHall
	FlagNoLanding
	FlagWater
	FlagJail
	FlagMonsterTrack
	FlagCastle
	FlagSwamp
	FlagNoSummonFriend
	FlagNoStore
	FlagTown
	FlagHQ
	FlagDanger
	FlagCastOnArtifact
	FlagNoRestart
	FlagScript
	FlagBoss

	// FlagCount is the number of defined flags.
	FlagCount
)

var flagNames = [FlagCount]string{
	"PvP", "Peace", "Siege", "MotherTree", "ClanHall", "NoLanding", "Water",
	"Jail", "MonsterTrack", "Castle", "Swamp", "NoSummonFriend", "NoStore",
	"Town", "HQ", "Danger", "CastOnArtifact", "NoRestart", "Script", "Boss",
}

// String names the flag.
func (f Flag) String() string {
	if f < FlagCount {
		return flagNames[f]
	}
	return fmt.Sprintf("Flag(%d)", uint8(f))
}

// Flags tracks, per Flag, how many zones currently impose it on one actor.
// Overlapping zones raising the same flag stack, and the flag reads as
// active until every zone that raised it has released it. The zero value
// is ready to use.
//
// mu guards counts.
type Flags struct {
	mu     sync.RWMutex
	counts [FlagCount]int32
}

// Set raises (state true) or releases (state false) one hold on flag.
// Releasing an already-clear flag is a no-op: the count never drops below
// zero.
func (f *Flags) Set(flag Flag, state bool) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if state {
		f.counts[flag]++
	} else if f.counts[flag] > 0 {
		f.counts[flag]--
	}
}

// Has reports whether flag is active. FlagPvP is special-cased: peace
// overrides combat, so it reads active only while no peace hold exists.
func (f *Flags) Has(flag Flag) bool {
	f.mu.RLock()
	defer f.mu.RUnlock()
	if flag == FlagPvP {
		return f.counts[FlagPvP] > 0 && f.counts[FlagPeace] == 0
	}
	return f.counts[flag] > 0
}
