package npc

import "sync"

// AbsorbInfo is what a monster records of one player charging a soul
// crystal on it: the crystal instance the player used, whether the
// crystal's skill registered the player, and the monster's HP share, in
// percent, when it did.
type AbsorbInfo struct {
	Registered   bool
	ItemObjectID int32
	HPPercent    int
}

// Valid reports whether the crystal itemObjectID is the one recorded and
// the monster's HP share at registration was below half.
func (a AbsorbInfo) Valid(itemObjectID int32) bool {
	return a.ItemObjectID == itemObjectID && a.HPPercent < 50
}

// absorbers holds, per player object id, the soul crystal charges on one
// monster's life. Players charge it from their own queues and its dying
// hooks read it from the engine queue, so mu guards byPlayer and every
// entry. It is a leaf lock but for the inventory and HP reads of a
// registration.
type absorbers struct {
	mu       sync.Mutex
	byPlayer map[int32]*AbsorbInfo
}

// AddAbsorber records that the player playerID used the soul crystal
// crystalObjectID on h. A player already registered keeps the crystal it
// registered with.
func (h *Hostile) AddAbsorber(playerID, crystalObjectID int32) {
	a := &h.absorb
	a.mu.Lock()
	defer a.mu.Unlock()
	info := a.byPlayer[playerID]
	switch {
	case info == nil:
		if a.byPlayer == nil {
			a.byPlayer = map[int32]*AbsorbInfo{}
		}
		a.byPlayer[playerID] = &AbsorbInfo{ItemObjectID: crystalObjectID}
	case !info.Registered:
		info.ItemObjectID = crystalObjectID
	}
}

// RegisterAbsorber registers the player playerID, whose crystal skill just
// landed on h, when it used a crystal on h that it still holds (holds
// answers by object id). The first registration records h's HP share as
// the whole number of times its HP fills its maximum, times 100: 100 at
// full HP, 0 below it.
func (h *Hostile) RegisterAbsorber(playerID int32, holds func(itemObjectID int32) bool) {
	a := &h.absorb
	a.mu.Lock()
	defer a.mu.Unlock()
	info := a.byPlayer[playerID]
	if info == nil || !holds(info.ItemObjectID) || info.Registered {
		return
	}
	if max := h.MaxHPValue(); max > 0 {
		info.HPPercent = int(h.HP()/max) * 100
	}
	info.Registered = true
}

// Absorber returns what h records of the player playerID's soul crystal
// charge, false when it records nothing.
func (h *Hostile) Absorber(playerID int32) (AbsorbInfo, bool) {
	a := &h.absorb
	a.mu.Lock()
	defer a.mu.Unlock()
	info := a.byPlayer[playerID]
	if info == nil {
		return AbsorbInfo{}, false
	}
	return *info, true
}
