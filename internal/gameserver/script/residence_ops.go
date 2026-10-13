package script

import (
	"github.com/fatal10110/acis_golang/internal/gameserver/castle"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/door"
	"github.com/fatal10110/acis_golang/internal/gameserver/siege"
)

// The doors and castles a behavior reads and moves (#3597).

// Door is a script's handle on one door of the world.
type Door struct {
	obj   *door.Object
	doors Doors
}

// Door returns the door named name, ignoring case, nil
// when there is none.
func (s *Script) Door(name string) *Door {
	if s.env == nil || s.env.Doors == nil {
		return nil
	}
	obj, ok := s.env.Doors.DoorByName(name)
	if !ok {
		return nil
	}
	return &Door{obj: obj, doors: s.env.Doors}
}

// Open opens the door as every opening does: its geodata is
// lifted, the players around it see it open, and its next automatic close
// or open is scheduled anew. A broken or open door is left as is.
func (d *Door) Open() { d.doors.SetDoorOpen(d.obj.DoorID(), true) }

// Close closes the door as every closing does. A broken or
// closed door is left as is.
func (d *Door) Close() { d.doors.SetDoorOpen(d.obj.DoorID(), false) }

// SiegeSide is the side a clan is registered on in a castle's siege.
type SiegeSide = siege.Side

// The siege sides a script checks.
const (
	SiegeOwner    = siege.SideOwner
	SiegeDefender = siege.SideDefender
	SiegeAttacker = siege.SideAttacker
)

// Castle is a script's handle on one castle.
type Castle struct {
	c *castle.Castle
	// siege is the castle's siege, nil when the server runs none.
	siege *siege.Siege
}

// Castle returns the castle the NPC belongs to, nil for
// none: the castle that placed it as a holy artifact, else the one its
// maker's spawn time names, none when that names a
// siegable clan hall, else the castle whose residence NPCs list the NPC's
// template. A handle on nothing belongs to none.
func (s *Script) Castle(n *NPC) *Castle {
	if s.env == nil || s.env.Castles == nil || n.combatant() == nil {
		return nil
	}
	c, ok := s.env.Castles.ForNPC(n.ObjectID(), int(n.NpcID()), n.instance().Maker, s.env.SiegableHall)
	if !ok {
		return nil
	}
	h := &Castle{c: c}
	if s.env.Sieges != nil {
		h.siege, _ = s.env.Sieges.Get(c.ID)
	}
	return h
}

// ID returns the castle's id.
func (c *Castle) ID() int32 { return int32(c.c.ID) }

// SiegeInProgress reports whether the castle's siege is being fought.
func (c *Castle) SiegeInProgress() bool { return c.siege != nil && c.siege.InProgress() }

// OnSiegeSide reports whether the player's clan is registered on one of
// sides in the castle's siege, on any side when sides is
// empty. A player in no clan is on none.
func (c *Castle) OnSiegeSide(p *Player, sides ...SiegeSide) bool {
	clanID := p.character().ClanID()
	return clanID != 0 && c.siege != nil && c.siege.CheckSides(clanID, sides...)
}

// InClan reports whether the player is in a clan.
func (p *Player) InClan() bool { return p.character().ClanID() != 0 }

// ClanCastleID returns the id of the castle the player's clan owns, 0 for
// none. A player in no clan panics.
func (p *Player) ClanCastleID() int32 { return p.clanCharacter().ClanCastleID() }
