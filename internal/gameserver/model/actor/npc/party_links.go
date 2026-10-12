package npc

import (
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
	// AddMinion records a private spawned for the NPC.
	AddMinion(minion PartyMember)
	// RemoveMinion forgets the private minion, in whichever life.
	RemoveMinion(minion PartyMember)
	// ClearMinions forgets every private, leaving them in the world; the
	// NPC stays a master.
	ClearMinions()
	// Minions returns a snapshot of the NPC's privates, each as its
	// current life.
	Minions() []PartyMember
	// PartyMinions returns a snapshot of the privates of the NPC's party:
	// its master's when it has one. Otherwise they are its own, and the
	// NPC is a master from then on, with no private at first.
	PartyMinions() []PartyMember
	// IsMaster reports whether the NPC has ever recorded a private, in
	// this life or an earlier one.
	IsMaster() bool

	links() *partyLinks
	inParty() bool
	partyMinions() []PartyMember
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
// master or the private it was, as the reference's reused NPC object is:
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
	// is read as the private's current life (Current).
	minions map[*Scratch]PartyMember
}

// party returns the NPC's party membership, its spawn slot's.
func (b *spawnBinding) party() *party { return &b.scratch.party }

// Master returns the NPC's master, nil for none: an NPC that is no
// private, or a private whose master died and let it go. A master that has
// respawned since is returned as its current life.
func (b *spawnBinding) Master() PartyMember {
	p := b.party()
	p.mu.RLock()
	defer p.mu.RUnlock()
	return Current(p.master)
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
	defer p.mu.RUnlock()
	return Current(p.spawnMaster)
}

// IsPrivate reports whether this NPC was spawned as a master's private.
func (b *spawnBinding) IsPrivate() bool { return b.SpawnMaster() != nil }

// AddMinion records a private spawned for this NPC.
func (b *spawnBinding) AddMinion(minion PartyMember) {
	if minion == nil {
		return
	}
	p := b.party()
	p.mu.Lock()
	if p.minions == nil {
		p.minions = make(map[*Scratch]PartyMember)
	}
	p.minions[minion.Scratch()] = minion
	p.mu.Unlock()
}

// RemoveMinion forgets the private minion, in whichever life.
func (b *spawnBinding) RemoveMinion(minion PartyMember) {
	if minion == nil {
		return
	}
	p := b.party()
	p.mu.Lock()
	delete(p.minions, minion.Scratch())
	p.mu.Unlock()
}

// ClearMinions forgets every private, leaving them in the world; the NPC
// stays a master.
func (b *spawnBinding) ClearMinions() {
	p := b.party()
	p.mu.Lock()
	p.minions = make(map[*Scratch]PartyMember)
	p.mu.Unlock()
}

// Minions returns a stable snapshot of this NPC's privates, each as its
// current life, dead and decayed ones included.
func (b *spawnBinding) Minions() []PartyMember {
	p := b.party()
	p.mu.RLock()
	minions := make([]PartyMember, 0, len(p.minions))
	for _, minion := range p.minions {
		minions = append(minions, minion)
	}
	p.mu.RUnlock()
	for i, minion := range minions {
		minions[i] = Current(minion)
	}
	return minions
}

// PartyMinions returns a snapshot of the privates of this NPC's party: its
// master's when it has one. Otherwise they are its own, and the NPC is a
// master from then on, with no private at first.
func (b *spawnBinding) PartyMinions() []PartyMember {
	if b.Master() == nil {
		p := b.party()
		p.mu.Lock()
		if p.minions == nil {
			p.minions = make(map[*Scratch]PartyMember)
		}
		p.mu.Unlock()
	}
	return b.partyMinions()
}

// IsMaster reports whether this NPC has ever recorded a private, in this
// life or an earlier one.
func (b *spawnBinding) IsMaster() bool {
	p := b.party()
	p.mu.RLock()
	defer p.mu.RUnlock()
	return p.minions != nil
}

// inParty reports whether the NPC is a master or a linked private.
func (b *spawnBinding) inParty() bool {
	return b.IsMaster() || b.Master() != nil
}

// partyMinions returns the privates of the NPC's party: its master's when
// it has one, its own otherwise.
func (b *spawnBinding) partyMinions() []PartyMember {
	if master := b.Master(); master != nil {
		return master.Minions()
	}
	return b.Minions()
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
// slot has respawned it since, as a new life. The reference respawns the
// same NPC object, so whatever holds an NPC acts on its current life. Nil
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
// not. assist turns on the built-in party assist of the called NPCs that
// have no bound behavior. Only a hostile NPC is called: no party hook is
// raised on a civilian NPC, and it has no built-in assist.
func propagatePartyAttacked(caller PartyMember, target attackable.Combatant, damage int, includeSelfInMinionLoop, assist bool) {
	if !caller.inParty() {
		return
	}
	call := func(called PartyMember) {
		if h, ok := called.(*Hostile); ok {
			h.partyAttacked(caller, target, damage, assist)
		}
	}
	call(caller)
	if master := caller.Master(); master != nil && !master.AlikeDead() {
		call(master)
	}
	for _, minion := range caller.partyMinions() {
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
// a master, while it keeps them as its privates. Only a hostile NPC's
// party-died hooks run: none is raised on a civilian NPC.
func partyDied(hooks ScriptHooks, caller PartyMember) {
	if !caller.inParty() {
		return
	}
	if hooks != nil {
		call := func(called PartyMember) {
			if h, ok := called.(*Hostile); ok {
				hooks.HostilePartyDied(caller, h)
			}
		}
		call(caller)
		if master := caller.Master(); master != nil && !SameNPC(master, caller) {
			call(master)
		}
		for _, minion := range caller.partyMinions() {
			if !SameNPC(minion, caller) {
				call(minion)
			}
		}
	}
	if caller.IsMaster() {
		for _, minion := range caller.Minions() {
			minion.SetMaster(nil)
		}
	}
}
