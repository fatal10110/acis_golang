package npc

import (
	"maps"
	"slices"
	"sync"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/attackable"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
)

// PartyMember is an NPC that can keep privates and be one: a hostile or a
// civilian NPC. A master and its privates of either kind make one party.
type PartyMember interface {
	attackable.Combatant
	NpcID() int
	// Template is the template the NPC was built from.
	Template() *Template
	// InTerritory reports whether the NPC stands in its spawn territory.
	InTerritory() bool
	// Scratch is the NPC's script memory.
	Scratch() *Scratch

	// Master returns the NPC's master, nil for none: an NPC that is no
	// private, or a private whose master died and let it go.
	Master() PartyMember
	// SetMaster links the NPC to master, or unlinks it for nil. Linking
	// makes the NPC a private of master for the rest of its life.
	SetMaster(master PartyMember)
	// SpawnMaster returns the master the NPC was spawned for, nil for an
	// NPC that is no private. Unlike Master it stays set once the master
	// died and let the private go.
	SpawnMaster() PartyMember
	// IsPrivate reports whether the NPC was spawned as a master's private.
	IsPrivate() bool
	// AddMinion records a private spawned for the NPC in its party's
	// privates: its master's when it has one, its own otherwise.
	AddMinion(minion PartyMember)
	// RemoveMinion forgets the private minion, in whichever life, from the
	// NPC's party's privates.
	RemoveMinion(minion PartyMember)
	// ClearMinions forgets every private of the NPC's party, leaving them
	// in the world.
	ClearMinions()
	// Minions returns a snapshot of the privates of the NPC's party, each
	// as its current life: its master's when it has one, its own
	// otherwise.
	Minions() []PartyMember
	// PartyMinions returns what Minions does. An NPC with no master keeps
	// privates of its own from then on: it is a master, with no private at
	// first.
	PartyMinions() []PartyMember
	// IsMaster reports whether the NPC has kept privates of its own, in
	// this life or an earlier one. A linked private records the privates it
	// spawns in its master's, so it is no master.
	IsMaster() bool
	// SpawnedPrivates reports whether a private was ever spawned for the
	// NPC, in this life or an earlier one, whichever party recorded it.
	SpawnedPrivates() bool

	links() *partyLinks
	inParty() bool
}

var (
	_ PartyMember = (*Hostile)(nil)
	_ PartyMember = (*Folk)(nil)
)

// Template returns the template the NPC was built from.
func (h *Hostile) Template() *Template { return h.Instance.Template }

// Template returns the template the NPC was built from.
func (f *Folk) Template() *Template { return f.Instance.Template }

// partyLinks is what one life of an NPC keeps of its party: the escort
// points around it. Hostile and Folk embed it. The party membership itself,
// master and privates, is the spawn slot's (party), so it carries over from
// one life to the next.
type partyLinks struct {
	// partyMu guards every field. A private claims its escort point under
	// its master's lock from the private's own queue (claimFollowSlot).
	partyMu sync.RWMutex
	// followSlots are the eight escort points around this NPC, occupied by
	// private object ids (0 is empty). Only a master uses them.
	followSlots [escortSlotCount]int32
	// lastFollowingLoc is the master position this private's last escort
	// step laid the slots out around, taken before the move; hasLastFollow
	// reports whether it is set.
	lastFollowingLoc location.Location
	hasLastFollow    bool
}

func (p *partyLinks) links() *partyLinks { return p }

// party is an NPC's party membership: its master and its privates. The
// spawn slot keeps it in its Scratch, so a respawned NPC is still the
// master or the private it was, as if the same NPC came back:
// a master that recorded privates once stays a master in every later life,
// and keeps the privates it had not forgotten.
type party struct {
	mu     sync.RWMutex
	master PartyMember
	// spawnMaster is the master this private was spawned for. It outlives
	// the master link, which the master's death clears: a private stays one
	// for good.
	spawnMaster PartyMember
	// minions are the privates, keyed by their spawn slot's Scratch: each
	// is read as the private's current life (Current). Only an NPC with no
	// master keeps them; a linked private's party uses its master's.
	minions map[*Scratch]PartyMember
	// spawned reports that a private was spawned for this NPC.
	spawned bool
}

// party returns the NPC's party membership, its spawn slot's.
func (b *spawnBinding) party() *party { return &b.scratch.party }

// Master returns the NPC's master, nil for none: an NPC that is no
// private, or a private whose master died and let it go. A master that has
// respawned since is returned as its current life.
func (b *spawnBinding) Master() PartyMember {
	p := b.party()
	p.mu.RLock()
	m := p.master
	p.mu.RUnlock()
	return Current(m)
}

// SetMaster links this NPC to its master, or unlinks it for nil. Linking
// makes the NPC a private of master for good.
func (b *spawnBinding) SetMaster(master PartyMember) {
	p := b.party()
	p.mu.Lock()
	p.master = master
	if master != nil {
		p.spawnMaster = master
	}
	p.mu.Unlock()
}

// SpawnMaster returns the master this private was spawned for, as its
// current life; nil for an NPC that is not a private. Unlike Master it
// stays set once the master died and let the private go.
func (b *spawnBinding) SpawnMaster() PartyMember {
	p := b.party()
	p.mu.RLock()
	m := p.spawnMaster
	p.mu.RUnlock()
	return Current(m)
}

// IsPrivate reports whether this NPC was spawned as a master's private.
func (b *spawnBinding) IsPrivate() bool { return b.SpawnMaster() != nil }

// holder returns the party whose privates this NPC's party uses: its top
// master's, following each master's own master, or its own when it has no
// master. Master links form no cycle: an NPC is linked only to an NPC that
// existed before it.
func (b *spawnBinding) holder() *party {
	m := b.Master()
	if m == nil {
		return b.party()
	}
	for next := m.Master(); next != nil; next = m.Master() {
		m = next
	}
	return &m.Scratch().party
}

// keptMinions returns the privates map of p, making p a master when it had
// none. The caller holds p.mu.
func (p *party) keptMinions() map[*Scratch]PartyMember {
	if p.minions == nil {
		p.minions = make(map[*Scratch]PartyMember)
	}
	return p.minions
}

// AddMinion records a private spawned for this NPC in its party's
// privates: its master's when it has one, its own otherwise.
func (b *spawnBinding) AddMinion(minion PartyMember) {
	if minion == nil {
		return
	}
	own := b.party()
	own.mu.Lock()
	own.spawned = true
	own.mu.Unlock()
	p := b.holder()
	p.mu.Lock()
	p.keptMinions()[minion.Scratch()] = minion
	p.mu.Unlock()
}

// RemoveMinion forgets the private minion, in whichever life, from this
// NPC's party's privates.
func (b *spawnBinding) RemoveMinion(minion PartyMember) {
	if minion == nil {
		return
	}
	p := b.holder()
	p.mu.Lock()
	delete(p.keptMinions(), minion.Scratch())
	p.mu.Unlock()
}

// ClearMinions forgets every private of this NPC's party, leaving them in
// the world.
func (b *spawnBinding) ClearMinions() {
	p := b.holder()
	p.mu.Lock()
	p.minions = make(map[*Scratch]PartyMember)
	p.mu.Unlock()
}

// Minions returns a stable snapshot of the privates of this NPC's party,
// each as its current life, dead and decayed ones included: its master's
// when it has one, its own otherwise.
func (b *spawnBinding) Minions() []PartyMember {
	return b.holder().snapshot(false)
}

// PartyMinions returns what Minions does. An NPC with no master keeps
// privates of its own from then on: it is a master, with no private at
// first.
func (b *spawnBinding) PartyMinions() []PartyMember {
	return b.holder().snapshot(true)
}

// snapshot returns p's privates, each as its current life; keep makes p a
// master when it had none.
func (p *party) snapshot(keep bool) []PartyMember {
	var minions []PartyMember
	if keep {
		p.mu.Lock()
		minions = slices.Collect(maps.Values(p.keptMinions()))
		p.mu.Unlock()
	} else {
		p.mu.RLock()
		minions = slices.Collect(maps.Values(p.minions))
		p.mu.RUnlock()
	}
	for i, minion := range minions {
		minions[i] = Current(minion)
	}
	return minions
}

// IsMaster reports whether this NPC has kept privates of its own, in this
// life or an earlier one.
func (b *spawnBinding) IsMaster() bool {
	p := b.party()
	p.mu.RLock()
	defer p.mu.RUnlock()
	return p.minions != nil
}

// SpawnedPrivates reports whether a private was ever spawned for this NPC,
// in this life or an earlier one.
func (b *spawnBinding) SpawnedPrivates() bool {
	p := b.party()
	p.mu.RLock()
	defer p.mu.RUnlock()
	return p.spawned
}

// inParty reports whether the NPC is a master or a linked private.
func (b *spawnBinding) inParty() bool {
	return b.IsMaster() || b.Master() != nil
}

// privateInTerritory decides InTerritory for a private: a linked one uses
// its master's territory, and a dead master leaves it in territory, as
// does a master that died and let it go. decided is false for an NPC that
// is no private, which answers from its own spawn.
func (b *spawnBinding) privateInTerritory() (in, decided bool) {
	if master := b.Master(); master != nil {
		if master.Dead() {
			return true, true
		}
		return master.InTerritory(), true
	}
	if b.IsPrivate() {
		return true, true
	}
	return false, false
}

// Current returns the NPC m's spawn slot placed last: m itself, unless the
// slot has respawned it since, as a new life. A respawn is the same NPC
// in a new life, so whatever holds an NPC acts on its current life. Nil
// stays nil.
func Current(m PartyMember) PartyMember {
	if m == nil {
		return nil
	}
	if s := m.Scratch(); s != nil {
		if life := s.Life(); life != nil {
			return life
		}
	}
	return m
}

// SameNPC reports whether a and b are lives of one NPC: placed by one
// spawn slot.
func SameNPC(a, b PartyMember) bool {
	if a == nil || b == nil {
		return a == b
	}
	return a.Scratch() == b.Scratch()
}

// propagatePartyAttacked calls the party of caller, an NPC in a party
// attacked by target for damage: caller itself, then its master when
// alive, then every live private of the party. The private loop skips
// caller unless includeSelfInMinionLoop, as an aggression effect's does
// not. Only a hostile NPC is called: no party hook is raised on a civilian
// NPC.
func propagatePartyAttacked(caller PartyMember, target attackable.Combatant, damage int, includeSelfInMinionLoop bool) {
	if !caller.inParty() {
		return
	}
	call := func(called PartyMember) {
		if h, ok := called.(*Hostile); ok {
			h.partyAttacked(caller, target, damage)
		}
	}
	call(caller)
	if master := caller.Master(); master != nil && !master.AlikeDead() {
		call(master)
	}
	for _, minion := range caller.Minions() {
		if minion.AlikeDead() {
			continue
		}
		if !includeSelfInMinionLoop && SameNPC(minion, caller) {
			continue
		}
		call(minion)
	}
}

// partyDied tells caller's party that caller has just died: caller itself,
// then its master, then every private of the party but caller, dead ones
// included. A dying master then lets its privates go: they no longer have
// a master, while it keeps them as its privates. The party-died hooks run
// on hostile and civilian NPCs alike.
func partyDied(hooks ScriptHooks, caller PartyMember) {
	if !caller.inParty() {
		return
	}
	if hooks != nil {
		hooks.PartyDied(caller, caller)
		if master := caller.Master(); master != nil && !SameNPC(master, caller) {
			hooks.PartyDied(caller, master)
		}
		for _, minion := range caller.Minions() {
			if !SameNPC(minion, caller) {
				hooks.PartyDied(caller, minion)
			}
		}
	}
	if caller.IsMaster() {
		for _, minion := range caller.Minions() {
			minion.SetMaster(nil)
		}
	}
}
