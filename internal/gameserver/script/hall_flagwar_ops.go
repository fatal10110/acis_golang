package script

import (
	"github.com/fatal10110/acis_golang/internal/gameserver/clanhall"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/attackable"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/event"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/npc"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/player"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/zone"
	"github.com/fatal10110/acis_golang/internal/gameserver/siege"
)

// The operations the flag war hall scripts (#3502) run beyond the hall
// siege: their stored registrations, the players in the hall's grounds and
// around an NPC, the system messages their messenger sends around it, and
// the town teleport of a dropped participant.

// FlagWarClan is a clan stored as registered on a flag war: its flag, the
// NPC it picked (0 for none) and its registered members.
type FlagWarClan = clanhall.FlagWarClan

// SiegeMessage is a system message: ID, with Text as its first parameter
// when set, the castle or hall CastleID names after it when set, and
// Number last when HasNumber.
type SiegeMessage = siege.Message

// FlagWarClans returns the clans stored as registered on the hall's flag
// war, in the store's order, skipping clans that no longer exist; each
// with every member stored for it.
func (h *Hall) FlagWarClans() []FlagWarClan { return h.s.FlagWarClans() }

// FlagWarOwnerNPC returns the hall's stored owner NPC row: the clan it was
// saved for and the NPC; ok is false when there is none.
func (h *Hall) FlagWarOwnerNPC() (clanID, npcID int32, ok bool) { return h.s.FlagWarOwnerNPC() }

// StoreFlagWarClan stores clanID as registered on the hall's flag war under
// flag, with no NPC picked.
func (h *Hall) StoreFlagWarClan(clanID, flag int32) { h.s.StoreFlagWarClan(clanID, flag) }

// StoreFlagWarNPC stores npcID as the NPC clanID picked, on every flag war
// row of the clan.
func (h *Hall) StoreFlagWarNPC(clanID, npcID int32) { h.s.StoreFlagWarNPC(clanID, npcID) }

// StoreFlagWarMember stores the player objectID as registered for clanID.
func (h *Hall) StoreFlagWarMember(clanID, objectID int32) { h.s.StoreFlagWarMember(clanID, objectID) }

// StoreFlagWarOwnerNPC stores npcID as the NPC the hall's owner clanID
// fights with; a hall stores one, and a second store fails.
func (h *Hall) StoreFlagWarOwnerNPC(clanID, npcID int32) { h.s.StoreFlagWarOwnerNPC(clanID, npcID) }

// DeleteFlagWarOwnerNPC deletes the stored owner NPC rows whose clan id is
// the hall's id.
func (h *Hall) DeleteFlagWarOwnerNPC() { h.s.DeleteFlagWarOwnerNPC() }

// ClearFlagWar deletes the hall's stored flag war clans and members.
func (h *Hall) ClearFlagWar() { h.s.ClearFlagWar() }

// PlayersInside returns the players standing in the hall's grounds.
func (h *Hall) PlayersInside() []*Player {
	if h.env.Zones == nil || h.env.World == nil {
		return nil
	}
	var out []*Player
	for _, z := range zone.OfKind[*zone.ClanHall](h.env.Zones) {
		if int32(z.ResidenceID) != h.ID() {
			continue
		}
		for _, a := range z.Occupants() {
			if _, ok := a.(zone.Player); !ok {
				continue
			}
			if o, ok := h.env.World.Player(a.ObjectID()); ok {
				if self, ok := o.(attackable.Combatant); ok {
					out = append(out, PlayerOf(self))
				}
			}
		}
	}
	return out
}

// PlayersInRadius returns the players within radius of the NPC, body to
// body in 3D. A handle on nothing panics.
func (n *NPC) PlayersInRadius(radius int) []*Player {
	scan, ok := n.combatant().(knownScanner)
	if !ok {
		panic("script: the players around a handle on nothing")
	}
	var out []*Player
	scan.ForEachKnownCombatantInRadius(radius, func(c attackable.Combatant) {
		if _, ok := c.(player.CharacterHolder); ok {
			out = append(out, PlayerOf(c))
		}
	})
	return out
}

// SystemMessageInRadius sends m to the players within radius of the NPC,
// bodies included, however far that reaches past the players that know
// it. A handle on nothing panics.
func (n *NPC) SystemMessageInRadius(m SiegeMessage, radius int) {
	e := event.NpcSystemMessage{ID: m.ID, Text: m.Text, CastleID: m.CastleID, Number: m.Number, HasNumber: m.HasNumber, Radius: radius}
	switch o := n.combatant().(type) {
	case *npc.Hostile:
		o.SystemMessageInRadius(e)
	case *npc.Folk:
		o.SystemMessageInRadius(e)
	default:
		panic("script: a system message around a handle on nothing")
	}
}

// TeleportToTown sends the player where a restart to town takes it. From a
// hook on the player's own queue the teleport runs inside the call; from
// any other it runs on the player's queue once the calling task is done.
func (p *Player) TeleportToTown() {
	c := p.character()
	if c.OnOwnQueue() {
		c.RecallOnQueue(skill.RecallTown)
		return
	}
	c.Recall(skill.RecallTown)
}

// ClanHallID returns the id of the clan hall the clan owns, a siegable one
// included, 0 for none.
func (c *ClanHandle) ClanHallID() int32 { return c.c.Info().HallID }
