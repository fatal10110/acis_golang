package network

import (
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/npc"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/summon"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/zone"
)

var _ summon.ZoneOwner = (*livePlayer)(nil)

// ZonePlayer is the player as its zones see it, which the zones read when
// they judge its summon by its owner.
func (p *livePlayer) ZonePlayer() (zone.Player, bool) {
	if p.zoneActor == nil {
		return nil, false
	}
	return p.zoneActor, true
}

// wireZoneOccupantHooks gives the zone rules that act on NPC and summon
// occupants their effects: a boss lair sends a raid NPC that leaves it, or
// every raid NPC once the last playable leaves, back home, and dismisses a
// summon whose owner may not enter; a siege battlefield dismisses a siege
// summon that leaves it or outlasts the siege.
//
// A zone rule runs while the occupant's zone membership is locked, so each
// effect is posted to the occupant's own queue rather than run in place.
func (l *GameClientLink) wireZoneOccupantHooks() {
	if l.zones == nil || l.world == nil {
		return
	}
	for _, boss := range zone.OfKind[*zone.Boss](l.zones) {
		boss.Unsummon = l.unsummonZoneOccupant
		boss.RecallNPC = l.recallRaidOccupant
		boss.RecallRaids = func() {
			for _, a := range boss.Occupants() {
				l.recallRaidOccupant(a)
			}
		}
	}
	for _, siege := range zone.OfKind[*zone.Siege](l.zones) {
		siege.Unsummon = l.unsummonSiegeOccupant
	}
}

// recallRaidOccupant sends a, when it is a raid-related NPC, back to its
// spawn. A raid NPC that leaves the lair by teleport is not sent: the
// reference judges it at its old position while the teleport keeps it from
// moving, so it only heads home later, when its own AI decides to.
func (l *GameClientLink) recallRaidOccupant(a zone.Actor) {
	obj, ok := l.world.Object(a.ObjectID())
	if !ok {
		return
	}
	h, ok := obj.(*npc.Hostile)
	if !ok || !h.RaidRelated() || h.Teleporting() {
		return
	}
	if q := h.Queue(); q != nil {
		q.Post(func() { h.ReturnHome() })
	}
}

// unsummonZoneOccupant dismisses a, when it is a summon.
func (l *GameClientLink) unsummonZoneOccupant(a zone.Actor) {
	if s, ok := l.zoneSummon(a); ok {
		if q := s.Queue(); q != nil {
			q.Post(s.Unsummon)
		}
	}
}

// unsummonSiegeOccupant dismisses a, when it is a siege summon.
func (l *GameClientLink) unsummonSiegeOccupant(a zone.Actor) {
	if s, ok := l.zoneSummon(a); ok && s.SiegeSummon() {
		if q := s.Queue(); q != nil {
			q.Post(s.Unsummon)
		}
	}
}

func (l *GameClientLink) zoneSummon(a zone.Actor) (*summon.Actor, bool) {
	obj, ok := l.world.Object(a.ObjectID())
	if !ok {
		return nil, false
	}
	s, ok := obj.(*summon.Actor)
	return s, ok
}
