package script

import (
	"maps"
	"slices"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/attackable"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/questlog"
)

// ItemUsed runs the item-use hook of every script bound to itemID, in list
// order, for which p's quest state is started: p used the item objectID,
// of template itemID, with target selected. It runs on p's queue, after
// the item's own use.
func (r *Registry) ItemUsed(p *Player, itemID, objectID int32, target attackable.Combatant) {
	if r == nil {
		return
	}
	list := r.items[itemID]
	if len(list) == 0 {
		return
	}
	journal := p.character().Quests()
	e := ItemUse{Player: p, ItemID: itemID, ObjectID: objectID, Target: creatureOf(target)}
	for _, s := range list {
		if st := journal.State(s.Name); st == nil || st.Status() != questlog.StatusStarted {
			continue
		}
		r.run(s, hookItemUse, func() { s.Hooks.ItemUse(s, e) })
	}
}

// ZoneEnterIDs returns, sorted, the ids of the zones whose entry reaches a
// script.
func (r *Registry) ZoneEnterIDs() []int32 {
	if r == nil {
		return nil
	}
	return slices.Sorted(maps.Keys(r.zones))
}

// ZoneEntered runs the zone-enter hook of every script bound to zoneID, in
// list order: c entered the zone. It runs on the goroutine that moved c,
// once that move's zone changes are done and no zone lock is held.
func (r *Registry) ZoneEntered(zoneID int32, c attackable.Combatant) {
	if r == nil {
		return
	}
	list := r.zones[zoneID]
	if len(list) == 0 {
		return
	}
	e := ZoneEnter{Creature: creatureOf(c), ZoneID: zoneID}
	for _, s := range list {
		r.run(s, hookZoneEnter, func() { s.Hooks.ZoneEnter(s, e) })
	}
}

// SendScriptEvent runs the script-event hook of every script bound to n's
// template, in list order, before it returns: n received eventID with arg1
// and arg2. The hooks run on the caller's goroutine. A handle on nothing
// receives nothing.
func (s *Script) SendScriptEvent(n *NPC, eventID, arg1, arg2 int32) {
	s.registry.scriptEvent(n, eventID, arg1, arg2)
}

// BroadcastScriptEvent sends eventID with arg1, and 0 as the second
// argument, to every NPC within radius of caller, caller excluded, as
// SendScriptEvent does: radius counts from body to body in 3D.
func (s *Script) BroadcastScriptEvent(caller *NPC, eventID, arg1 int32, radius int) {
	s.registry.broadcastScriptEvent(caller, eventID, arg1, 0, radius)
}

// BroadcastScriptEventEx is BroadcastScriptEvent with arg2 as the second
// argument.
func (s *Script) BroadcastScriptEventEx(caller *NPC, eventID, arg1, arg2 int32, radius int) {
	s.registry.broadcastScriptEvent(caller, eventID, arg1, arg2, radius)
}

func (r *Registry) scriptEvent(n *NPC, eventID, arg1, arg2 int32) {
	if n.combatant() == nil {
		return
	}
	list := r.scripts(n.NpcID(), EventScriptEvent)
	if len(list) == 0 {
		return
	}
	e := ScriptEvent{NPC: n, EventID: eventID, Arg1: arg1, Arg2: arg2}
	for _, s := range list {
		r.run(s, hookScriptEvent, func() { s.Hooks.ScriptEvent(s, e) })
	}
}

// knownScanner is an NPC that visits the creatures around it.
type knownScanner interface {
	ForEachKnownCombatantInRadius(radius int, fn func(attackable.Combatant))
}

func (r *Registry) broadcastScriptEvent(caller *NPC, eventID, arg1, arg2 int32, radius int) {
	scan, ok := caller.combatant().(knownScanner)
	if !ok {
		return
	}
	// The receivers are gathered first, so their hooks run outside the
	// world scan.
	var receivers []*NPC
	scan.ForEachKnownCombatantInRadius(radius, func(c attackable.Combatant) {
		if n := NPCOf(c); n != nil {
			receivers = append(receivers, n)
		}
	})
	for _, n := range receivers {
		r.scriptEvent(n, eventID, arg1, arg2)
	}
}

// ActingPlayer returns the player acting through c: c itself when it is a
// player, its owner when it is a summon; nil otherwise.
func ActingPlayer(c Creature) *Player {
	self := combatantOf(c)
	if self == nil {
		return nil
	}
	switch self.Kind() {
	case actor.KindPlayer:
		if p, ok := c.(*Player); ok {
			return p
		}
		return PlayerOf(self)
	case actor.KindSummon:
		if owner, ok := self.Owner(); ok && owner != nil {
			return PlayerOf(owner)
		}
	}
	return nil
}
