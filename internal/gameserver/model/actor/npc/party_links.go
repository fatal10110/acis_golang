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
	// RemoveMinion forgets the private with object id.
	RemoveMinion(id int32)
	// ClearMinions forgets every private, leaving them in the world; the
	// NPC stays a master.
	ClearMinions()
	// Minions returns a snapshot of the NPC's current privates.
	Minions() []PartyMember
	// PartyMinions returns a snapshot of the privates of the NPC's party:
	// its master's when it has one. Otherwise they are its own, and the
	// NPC is a master from then on, with no private at first.
	PartyMinions() []PartyMember
	// IsMaster reports whether the NPC has ever recorded a private.
	IsMaster() bool

	links() *partyLinks
}

var (
	_ PartyMember = (*Hostile)(nil)
	_ PartyMember = (*Folk)(nil)
)

// Template returns the template the NPC was built from.
func (h *Hostile) Template() *Template { return h.Instance.Template }

// Template returns the template the NPC was built from.
func (f *Folk) Template() *Template { return f.Instance.Template }

// partyLinks is an NPC's place in its party: its master, its privates, and
// the escort points around it. Hostile and Folk embed it.
type partyLinks struct {
	// partyMu guards every field. A private claims its escort point under
	// its master's lock from the private's own queue (claimFollowSlot).
	partyMu sync.RWMutex
	master  PartyMember
	// spawnMaster is the master this private was spawned for. It outlives
	// the master link, which the master's death clears: a private stays one
	// for its whole life.
	spawnMaster PartyMember
	minions     map[int32]PartyMember
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

// Master returns the NPC's master, nil for none: an NPC that is no
// private, or a private whose master died and let it go.
func (p *partyLinks) Master() PartyMember {
	p.partyMu.RLock()
	defer p.partyMu.RUnlock()
	return p.master
}

// SetMaster links this NPC to its master, or unlinks it for nil. Linking
// makes the NPC a private of master for the rest of its life.
func (p *partyLinks) SetMaster(master PartyMember) {
	p.partyMu.Lock()
	p.master = master
	if master != nil {
		p.spawnMaster = master
	}
	p.partyMu.Unlock()
}

// SpawnMaster returns the master this private was spawned for, nil for an
// NPC that is not a private. Unlike Master it stays set once the master
// died and let the private go.
func (p *partyLinks) SpawnMaster() PartyMember {
	p.partyMu.RLock()
	defer p.partyMu.RUnlock()
	return p.spawnMaster
}

// IsPrivate reports whether this NPC was spawned as a master's private.
func (p *partyLinks) IsPrivate() bool { return p.SpawnMaster() != nil }

// AddMinion records a private spawned for this NPC.
func (p *partyLinks) AddMinion(minion PartyMember) {
	if minion == nil {
		return
	}
	p.partyMu.Lock()
	if p.minions == nil {
		p.minions = make(map[int32]PartyMember)
	}
	p.minions[minion.ObjectID()] = minion
	p.partyMu.Unlock()
}

// RemoveMinion forgets a private that has decayed or been removed.
func (p *partyLinks) RemoveMinion(id int32) {
	p.partyMu.Lock()
	delete(p.minions, id)
	p.partyMu.Unlock()
}

// ClearMinions forgets every private, leaving them in the world; the NPC
// stays a master.
func (p *partyLinks) ClearMinions() {
	p.partyMu.Lock()
	p.minions = make(map[int32]PartyMember)
	p.partyMu.Unlock()
}

// Minions returns a stable snapshot of this NPC's current privates.
func (p *partyLinks) Minions() []PartyMember {
	p.partyMu.RLock()
	defer p.partyMu.RUnlock()
	minions := make([]PartyMember, 0, len(p.minions))
	for _, minion := range p.minions {
		minions = append(minions, minion)
	}
	return minions
}

// PartyMinions returns a snapshot of the privates of this NPC's party: its
// master's when it has one. Otherwise they are its own, and the NPC is a
// master from then on, with no private at first.
func (p *partyLinks) PartyMinions() []PartyMember {
	if master := p.Master(); master != nil {
		return master.Minions()
	}
	p.partyMu.Lock()
	if p.minions == nil {
		p.minions = make(map[int32]PartyMember)
	}
	p.partyMu.Unlock()
	return p.Minions()
}

// IsMaster reports whether this NPC has ever recorded a private.
func (p *partyLinks) IsMaster() bool {
	p.partyMu.RLock()
	defer p.partyMu.RUnlock()
	return p.minions != nil
}

// inParty reports whether the NPC is a master or a linked private.
func (p *partyLinks) inParty() bool {
	return p.IsMaster() || p.Master() != nil
}

// partyMinions returns the privates of the NPC's party: its master's when
// it has one, its own otherwise.
func (p *partyLinks) partyMinions() []PartyMember {
	if master := p.Master(); master != nil {
		return master.Minions()
	}
	return p.Minions()
}

// privateInTerritory decides InTerritory for a private: a linked one uses
// its master's territory, and a dead master leaves it in territory, as
// does a master that died and let it go. decided is false for an NPC that
// is no private, which answers from its own spawn.
func (p *partyLinks) privateInTerritory() (in, decided bool) {
	if master := p.Master(); master != nil {
		if master.Dead() {
			return true, true
		}
		return master.InTerritory(), true
	}
	if p.IsPrivate() {
		return true, true
	}
	return false, false
}

// propagatePartyAttacked calls the party of caller, an NPC in a party
// attacked by target for damage: caller itself, then its master when
// alive, then every live private of the party. The private loop skips
// caller unless includeSelfInMinionLoop, as an aggression effect's does
// not. assist turns on the built-in party assist of the called NPCs that
// have no bound behavior. Only a hostile NPC is called: no party hook is
// raised on a civilian NPC, and it has no built-in assist.
func propagatePartyAttacked(caller PartyMember, target attackable.Combatant, damage int, includeSelfInMinionLoop, assist bool) {
	p := caller.links()
	if !p.inParty() {
		return
	}
	call := func(called PartyMember) {
		if h, ok := called.(*Hostile); ok {
			h.partyAttacked(caller, target, damage, assist)
		}
	}
	call(caller)
	if master := p.Master(); master != nil && !master.AlikeDead() {
		call(master)
	}
	for _, minion := range p.partyMinions() {
		if minion.AlikeDead() {
			continue
		}
		if !includeSelfInMinionLoop && minion == caller {
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
	p := caller.links()
	if !p.inParty() {
		return
	}
	if hooks != nil {
		call := func(called PartyMember) {
			if h, ok := called.(*Hostile); ok {
				hooks.HostilePartyDied(caller, h)
			}
		}
		call(caller)
		if master := p.Master(); master != nil && master != caller {
			call(master)
		}
		for _, minion := range p.partyMinions() {
			if minion != caller {
				call(minion)
			}
		}
	}
	if p.IsMaster() {
		for _, minion := range p.Minions() {
			minion.SetMaster(nil)
		}
	}
}
