package network

import (
	"github.com/fatal10110/acis_golang/internal/commons/wire"
	"github.com/fatal10110/acis_golang/internal/gameserver/castle"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/npc"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	castledata "github.com/fatal10110/acis_golang/internal/gameserver/model/residence/castle"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/zone"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/sevensigns"
	"github.com/fatal10110/acis_golang/internal/gameserver/skill/effect"
)

// wyvernRules are the wyvern ride settings cfg names, their defaults for
// none.
func wyvernRules(cfg *npc.WyvernRules) npc.WyvernRules {
	if cfg == nil {
		return npc.DefaultWyvernRules()
	}
	return *cfg
}

// aliveLifeTowers is the number of c's life control towers standing.
//
// ponytail: the control towers are not spawned yet, so every life control
// tower c lists counts as standing; #3857 switches this to the live count.
func aliveLifeTowers(c *castle.Castle) int {
	n := 0
	for _, t := range c.ControlTowers {
		if t.Type == castledata.TowerLifeControl {
			n++
		}
	}
	return n
}

// doormanDoors carries out a doorman's door command for live: it opens or
// closes the doors reply names, then shows reply's page, if any. It reports
// false when the command stopped the handling: a plain doorman naming an
// unknown door, or a door list that does not parse, after the doors before
// the fault.
func (l *GameClientLink) doormanDoors(live *livePlayer, f *npc.Folk, reply npc.BypassReply) bool {
	c, h := l.npcResidence(f.Instance)
	switch reply.DoorScope {
	case npc.DoorsOfHall:
		if h != nil {
			l.setHallDoors(h.Gates, reply.OpenDoors)
		}
	case npc.DoorsOfResidence, npc.DoorsOfWorld:
		var gates []string
		switch {
		case h != nil:
			gates = h.Gates
		case c != nil:
			gates = c.Gates
		}
		ids, malformed := reply.DoorIDs()
		for _, id := range ids {
			if reply.DoorScope == npc.DoorsOfResidence {
				l.setGateOfID(gates, id, reply.OpenDoors)
				continue
			}
			if l.doors == nil {
				return false
			}
			if _, ok := l.doors.Door(id); !ok {
				return false
			}
			l.doors.SetDoorOpen(id, reply.OpenDoors)
		}
		if malformed {
			return false
		}
	}
	if reply.HTML != "" {
		sendFilledHTML(live, f.ObjectID(), reply.HTML, 0)
	}
	return true
}

// setGateOfID opens or closes the door id when it is one of gates; any
// other id is passed over.
func (l *GameClientLink) setGateOfID(gates []string, id int, open bool) {
	if l.doors == nil {
		return
	}
	for _, name := range gates {
		if d, ok := l.doors.DoorByName(name); ok && d.DoorID() == id {
			l.doors.SetDoorOpen(id, open)
			return
		}
	}
}

// doormanBusy refuses live a doorman's gates during a siege: the busy page,
// ActionFailed, then the message saying gates stay as they are.
func doormanBusy(live *livePlayer, f *npc.Folk, html string) {
	sendFilledHTML(live, f.ObjectID(), html, 0)
	live.SendFrame(serverpackets.FrameActionFailed())
	live.SendFrame(serverpackets.FrameSystemMessage(serverpackets.SystemMessageGatesNotOpenedClosedDuringSiege))
}

// clanGate sends live to the portal of its clan leader: nothing while the
// leader is offline; the magician's page saying there is no portal, then
// ActionFailed, when the leader opened none; the refusal message when
// npc.ClanGateAllowed refuses; else the teleport to the leader.
func (l *GameClientLink) clanGate(live *livePlayer, f *npc.Folk, html string) {
	cl, ok := l.clanService().ClanOf(live.Character)
	if !ok {
		return
	}
	leader, ok := l.livePlayerByID(cl.LeaderID())
	if !ok {
		return
	}
	if !hasClanGate(leader) {
		sendFilledHTML(live, f.ObjectID(), html, 0)
		live.SendFrame(serverpackets.FrameActionFailed())
		return
	}
	if !npc.ClanGateAllowed(l.clanGateLeader(leader), l.clanGateTalker(live)) {
		live.SendFrame(serverpackets.FrameSystemMessageString(serverpackets.SystemMessageS1, npc.ClanGateRefusal))
		return
	}
	l.teleportLivePlayer(live, leader.CurrentLocation(), 0)
}

// hasClanGate reports whether live carries a clan gate effect, applied or
// held.
func hasClanGate(live *livePlayer) bool {
	for _, e := range live.EffectList().All() {
		if e != nil && e.Type == effect.TypeClanGate {
			return true
		}
	}
	return false
}

// clanGateLeader is what a castle magician reads of leader.
func (l *GameClientLink) clanGateLeader(leader *livePlayer) npc.ClanGateLeader {
	busy := leader.AlikeDead() || leader.Operating() || leader.EffectList().IsAffected(effect.FlagRooted) ||
		leader.InCombat() || leader.OlympiadMode() || leader.FestivalParticipant() || leader.ObserverMode() ||
		leader.NoSummonFriendZone()
	g := npc.ClanGateLeader{Busy: busy}
	if l.sevenSigns != nil {
		g.Cabal = l.sevenSigns.PlayerCabal(leader.ObjectID())
	}
	return g
}

// clanGateTalker is what a castle magician reads of live and the Seven
// Signs.
func (l *GameClientLink) clanGateTalker(live *livePlayer) npc.ClanGateTalker {
	t := npc.ClanGateTalker{InDungeon: live.In7sDungeon()}
	if l.sevenSigns != nil {
		t.SealValidation = l.sevenSigns.CurrentPeriod() == sevensigns.SealValidation
		t.Winning = l.sevenSigns.WinningCabal()
	}
	return t
}

// massTeleport arms mass gatekeeper f's teleport of its castle's
// defenders, unless one is pending, then shows reply's page.
func (l *GameClientLink) massTeleport(live *livePlayer, f *npc.Folk, reply npc.BypassReply) {
	if c, _ := l.npcResidence(f.Instance); c != nil {
		f.ArmMassTeleport(reply.Delay, func() { l.oustCastleDefenders(f, c) })
	}
	sendFilledHTML(live, f.ObjectID(), reply.HTML, 0)
}

// oustCastleDefenders is mass gatekeeper f's teleport of c's defenders:
// while c's siege runs, f shouts so to every player of its region; then
// every player in c's mass teleport room is sent into the inner castle.
func (l *GameClientLink) oustCastleDefenders(f *npc.Folk, c *castle.Castle) {
	if s, ok := l.castleSiege(int32(c.ID)); ok && s.InProgress() {
		x, y, z := f.Position()
		text := npc.MassTeleportShout(c.Name)
		l.broadcastToRegionAt(location.Location{X: x, Y: y, Z: z}, func() wire.Frame {
			return serverpackets.FrameNpcSay(f.ObjectID(), f.NpcID(), serverpackets.SayTypeShout, text)
		})
	}
	if l.zones == nil {
		return
	}
	for _, room := range zone.OfKind[*zone.CastleTeleport](l.zones) {
		if room.CastleID == c.ID {
			room.OustAll()
			return
		}
	}
}

// wireCastleTeleportZones has each mass teleport room of ix send the
// players it ousts to the point picked for them.
func (l *GameClientLink) wireCastleTeleportZones(ix *zone.Index) {
	for _, room := range zone.OfKind[*zone.CastleTeleport](ix) {
		room.Eject = func(a zone.Actor, to location.Location) {
			za, ok := a.(*liveZoneActor)
			if !ok {
				return
			}
			live := za.live
			// The oust walks the room under its lock; the teleport runs on
			// the player's own queue after it.
			postLive(live, func() {
				if !live.detached() {
					l.teleportLivePlayer(live, to, 0)
				}
			})
		}
	}
}

// mercenaryHire opens the untaxed buy window of the mercenary buylist reply
// names at f, then reply's page. A list f may not sell answers nothing.
func (l *GameClientLink) mercenaryHire(live *livePlayer, f *npc.Folk, reply npc.BypassReply) {
	list, ok := l.merchant.List(reply.ListID)
	inv := live.Inventory()
	if !ok || !list.AllowsNPC(f.NpcID()) || inv == nil {
		return
	}
	live.tempInventoryDisable()
	frame, err := serverpackets.FrameBuyList(list, l.merchant.Count, inv.Adena(), 0, l.merchant.Config().SiegeGuardsPriceRate, l.itemTemplates)
	if err != nil {
		l.log.Error().Err(err).Int("buylist", reply.ListID).Msg("build BuyList")
		return
	}
	live.SendFrame(frame)
	sendFilledHTML(live, f.ObjectID(), reply.HTML, 0)
}
