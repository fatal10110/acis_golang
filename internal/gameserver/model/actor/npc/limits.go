package npc

import "sync/atomic"

// Limits are the settings every NPC reads at each use and //reload config
// puts in force whole: the base buff-slot count (players.properties
// MaxBuffsAmount) and the drift range (npcs.properties MaxDriftRange), how
// far around its spawn point an NPC spawned at a point keeps its territory,
// how far it strays before it heads back, and how far a script makes it
// flee.
type Limits struct {
	MaxBuffs   int
	DriftRange int
}

// DefaultLimits are the shipped limits: 20 buff slots and a drift range of
// 200.
func DefaultLimits() Limits {
	return Limits{MaxBuffs: maxBuffCount, DriftRange: defaultDriftRange}
}

// LimitBook holds the Limits in force, shared by every NPC that reads it
// (Instance.Limits), so storing new ones reaches the NPCs already in the
// world and the ones still to come at once.
type LimitBook struct {
	v atomic.Pointer[Limits]
}

// NewLimitBook returns a LimitBook holding l.
func NewLimitBook(l Limits) *LimitBook {
	b := &LimitBook{}
	b.Set(l)
	return b
}

// Set puts l in force in place of the limits before.
func (b *LimitBook) Set(l Limits) { b.v.Store(&l) }

// Limits returns the limits in force; a nil book reads DefaultLimits.
func (b *LimitBook) Limits() Limits {
	if b == nil {
		return DefaultLimits()
	}
	return *b.v.Load()
}

// maxBuffsBase is the base buff-slot count of inst, before its Divine
// Inspiration.
func (inst *Instance) maxBuffsBase() int {
	return inst.Limits.Limits().MaxBuffs
}
