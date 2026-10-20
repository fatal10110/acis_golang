package network

import (
	"github.com/fatal10110/acis_golang/internal/commons/rnd"
	"github.com/fatal10110/acis_golang/internal/commons/wire"
	"github.com/fatal10110/acis_golang/internal/gameserver/festival"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/ai"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/move"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/npc"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/zone"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/sevensigns"
)

// festivalWorld carries out the festival's runs on live players, the
// arenas' NPCs and the oracles. Every effect on a player runs on that
// player's queue, and every one on an NPC on the NPC's.
type festivalWorld struct{ l *GameClientLink }

var _ festival.World = festivalWorld{}

// FestivalWorld is how the festival reaches the players, clans and arenas
// through l.
func (l *GameClientLink) FestivalWorld() festival.World { return festivalWorld{l} }

// The skills a festival witch casts on herself as she arrives.
const (
	witchArrivalSkill   = 2003
	witchArrivalSkillTo = 2133
)

// swarmWeight is how strongly a festival monster wants to reach its
// arena's center when sent there.
const swarmWeight = 1000000

func (w festivalWorld) RewardClanOf(name string, points int) { w.l.RewardClanOf(name, points) }
func (w festivalWorld) RemoveBloodOfferings()                { w.l.RemoveBloodOfferings() }

// Shout sends text as a shout by sender to the players inside each peace
// zone holding a festival guide: the Dawn oracle's zones, then the Dusk
// one's, a player inside two of them hearing it twice.
func (w festivalWorld) Shout(sender, text string) {
	l := w.l
	if l.zones == nil || l.world == nil {
		return
	}
	var zones [2][]*zone.Peace
	peace := zone.OfKind[*zone.Peace](l.zones)
	for _, obj := range l.world.Objects() {
		f, ok := obj.(*npc.Folk)
		if !ok || f.Instance.Template.Type != "FestivalGuide" {
			continue
		}
		g, ok := festival.GuideOf(f.NpcID())
		if !ok {
			continue
		}
		s := 1
		if g.Cabal == sevensigns.Dawn {
			s = 0
		}
		x, y, z := f.Position()
		for _, pz := range peace {
			if pz.ContainsPoint(x, y, z) && !containsZone(zones[s], pz) {
				zones[s] = append(zones[s], pz)
			}
		}
	}
	for _, side := range zones {
		for _, pz := range side {
			var players []*livePlayer
			for _, a := range pz.Occupants() {
				if a.Class() != zone.ClassPlayer {
					continue
				}
				if live, ok := l.onlineLive(a.ObjectID()); ok {
					players = append(players, live)
				}
			}
			sendToMembers(players, func() wire.Frame {
				return serverpackets.FrameCreatureSay(0, serverpackets.SayTypeShout, sender, text)
			})
		}
	}
}

func containsZone(zs []*zone.Peace, z *zone.Peace) bool {
	for _, have := range zs {
		if have == z {
			return true
		}
	}
	return false
}

func (w festivalWorld) Locate(objectID int32) (festival.Spot, bool) {
	live, ok := w.l.onlineLive(objectID)
	if !ok {
		return festival.Spot{}, false
	}
	x, y, z := live.Position()
	return festival.Spot{X: x, Y: y, Z: z, Heading: live.Heading()}, true
}

// Admit stops what the player does, teleports it within 20 of (x, y, z),
// ends its effects but for those lasting through death and destroys its
// blood offerings, naming them.
func (w festivalWorld) Admit(objectID int32, x, y, z int) {
	w.postOnline(objectID, func(live *livePlayer) {
		live.tryToIdle(live.DenyAIAction())
		w.l.teleportLivePlayer(live, location.Location{X: x, Y: y, Z: z}, restartTeleportOffset)
		live.EffectList().StopAllExceptThoseThatLastThroughDeath()
		if inv := live.Inventory(); inv != nil {
			if inst := inv.ItemByTemplateID(festival.BloodOfferingID); inst != nil {
				destroyHeldItem(live, inst, inst.Snapshot().Count)
			}
		}
	})
}

// Relocate stops what the player does and teleports it within 20 of at, or
// to the nearest town, then tells it it left the arena.
func (w festivalWorld) Relocate(objectID int32, at *festival.Spot) {
	w.postOnline(objectID, func(live *livePlayer) {
		live.tryToIdle(live.DenyAIAction())
		if at != nil {
			w.l.teleportLivePlayer(live, location.Location{X: at.X, Y: at.Y, Z: at.Z}, restartTeleportOffset)
		} else if dest, ok := w.l.restartDestination(live); ok {
			w.l.teleportLivePlayer(live, dest, restartTeleportOffset)
		}
		sendText(live, "You have been removed from the festival arena.")
	})
}

func (w festivalWorld) Tell(objectID int32, text string) {
	w.postOnline(objectID, func(live *livePlayer) { sendText(live, text) })
}

// Settle waits for each player's queue to run what was posted to it.
func (w festivalWorld) Settle(objectIDs []int32) {
	for _, id := range objectIDs {
		if live, ok := w.l.onlineLive(id); ok {
			awaitLiveQueue(live)
		}
	}
}

// festivalDeparture is where live, a participant leaving the world during
// a festival, is saved: the party leave its detach makes takes it out of
// its arena, within 20 of where it came from or of the nearest town, before
// its position is stored. ok is false when it stays where it stands.
func (l *GameClientLink) festivalDeparture(live *livePlayer) (location.Location, bool) {
	if l.festival == nil || l.parties == nil || !l.parties.InParty(live.ObjectID()) {
		return location.Location{}, false
	}
	at, ok := l.festival.Departure(live.ObjectID())
	if !ok {
		return location.Location{}, false
	}
	var dest location.Location
	if at != nil {
		dest = location.Location{X: at.X, Y: at.Y, Z: at.Z}
	} else if dest, ok = l.restartDestination(live); !ok {
		return location.Location{}, false
	}
	return move.RandomNearbyLocation(l.geo, dest, restartTeleportOffset, func(at location.Location) bool {
		return live.Flying() || l.inWater(at)
	}), true
}

// postOnline runs fn on the queue of the player objectID while it is in the
// world.
func (w festivalWorld) postOnline(objectID int32, fn func(*livePlayer)) {
	if live, ok := w.l.onlineLive(objectID); ok {
		postLive(live, func() { fn(live) })
	}
}

// SpawnWitch places the witch, then shows her two arrival casts on
// herself.
func (w festivalWorld) SpawnWitch(npcID int, s festival.Spot) (festival.NPC, bool) {
	npcs := w.l.npcSpawns.Load()
	tmpl, ok := w.l.npcs.Get(npcID)
	if npcs == nil || !ok {
		return nil, false
	}
	f, err := npcs.SpawnFixedFolk(tmpl, s.X, s.Y, s.Z, s.Heading)
	if err != nil || f == nil {
		return nil, false
	}
	x, y, z := f.Position()
	for _, skill := range [...]int32{witchArrivalSkill, witchArrivalSkillTo} {
		f.BroadcastSkillUse(f.ObjectID(), x, y, z, skill, 1, 1, 0)
	}
	return f, true
}

func (w festivalWorld) Say(n festival.NPC, text string) {
	if f, ok := n.(*npc.Folk); ok {
		f.Say(text)
	}
}

// SpawnMonster places the monster facing a random way, worth bonus blood
// offerings a kill.
func (w festivalWorld) SpawnMonster(npcID, x, y, z, bonus int) (festival.NPC, bool) {
	npcs := w.l.npcSpawns.Load()
	tmpl, ok := w.l.npcs.Get(npcID)
	if npcs == nil || !ok {
		return nil, false
	}
	h, err := npcs.SpawnFixedHostile(tmpl, x, y, z, rnd.Get(65536))
	if err != nil {
		return nil, false
	}
	h.SetOfferingBonus(bonus)
	return h, true
}

// Swarm has the monster, alive and idle, run to (x, y, z).
func (w festivalWorld) Swarm(n festival.NPC, x, y, z int) {
	h, ok := n.(*npc.Hostile)
	if !ok {
		return
	}
	h.Queue().Post(func() {
		if h.Dead() || h.AI().CurrentIntention() != ai.IntentionIdle {
			return
		}
		h.ForceRunStance()
		h.AI().AddMoveToDesire(location.Location{X: x, Y: y, Z: z}, swarmWeight)
	})
}

// Unspawn deletes the NPC, alive or dead, when it is still the one placed.
func (w festivalWorld) Unspawn(n festival.NPC) {
	npcs := w.l.npcSpawns.Load()
	if npcs == nil || w.l.world == nil {
		return
	}
	if obj, ok := w.l.world.Object(n.ObjectID()); ok && obj == n {
		npcs.DeleteFixed(n.ObjectID())
	}
}
