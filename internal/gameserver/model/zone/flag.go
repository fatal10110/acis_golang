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
// Holds come from two sources kept apart: counts are the zones' own holds,
// which a zone reload drops with the zones that raised them, and held are
// those something other than a zone raised (a boat keeping its passengers
// in peace), which a zone reload leaves alone.
//
// mu guards counts, held and synced. Revalidation writes them from whichever
// queue moved the actor (a summon-friend teleport runs on the caster's),
// while movement and exit checks read them from the actor's own queue.
// synced is the zone set the holds were last raised against.
type Flags struct {
	mu     sync.RWMutex
	counts [FlagCount]int32
	held   [FlagCount]int32
	synced *zoneSet
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

// Hold raises (state true) or releases (state false) one hold on flag that
// no zone imposes: it counts toward Has like a zone's hold, but a zone
// reload keeps it. Releasing an already-clear hold is a no-op.
func (f *Flags) Hold(flag Flag, state bool) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if state {
		f.held[flag]++
	} else if f.held[flag] > 0 {
		f.held[flag]--
	}
}

// Has reports whether flag is active. FlagPvP is special-cased: peace
// overrides combat, so it reads active only while no peace hold exists.
func (f *Flags) Has(flag Flag) bool {
	f.mu.RLock()
	defer f.mu.RUnlock()
	if flag == FlagPvP {
		return f.total(FlagPvP) > 0 && f.total(FlagPeace) == 0
	}
	return f.total(flag) > 0
}

// total is every hold on flag, from zones or not. Callers hold mu.
func (f *Flags) total(flag Flag) int32 { return f.counts[flag] + f.held[flag] }

// resync records set as the zone set the holds are raised against from now
// on. When the holds were raised against another set, it releases every
// zone hold and returns that set; otherwise it returns nil. The holds no
// zone raised (Hold) stay.
func (f *Flags) resync(set *zoneSet) *zoneSet {
	f.mu.Lock()
	defer f.mu.Unlock()
	prev := f.synced
	f.synced = set
	if prev == nil || prev == set {
		return nil
	}
	f.counts = [FlagCount]int32{}
	return prev
}
