package siege

import (
	skilltarget "github.com/fatal10110/acis_golang/internal/gameserver/handler/target"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/attackable"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/door"
)

// swoopCannonID is the anti-infantry siege summon, which neither damages a
// castle door nor attacks a wall.
const swoopCannonID = 14839

// A siege is the door.Siege of its castle's gates (castles.xml <gates>).
var _ door.Siege = (*Siege)(nil)

// siegeSummon is a summon that tells whether its template is a siege
// summon, and which.
type siegeSummon interface {
	SiegeSummon() bool
	NPCID() int
}

// siegeWeapon reports whether a is a siege summon other than the Swoop
// Cannon: the only attacker that may attack a castle wall.
func siegeWeapon(a any) bool {
	s, ok := a.(siegeSummon)
	return ok && s.SiegeSummon() && s.NPCID() != swoopCannonID
}

// swoopCannon reports whether a is the Swoop Cannon siege summon.
func swoopCannon(a any) bool {
	s, ok := a.(siegeSummon)
	return ok && s.SiegeSummon() && s.NPCID() == swoopCannonID
}

// actingClanID is the clan of the player acting through a: a summon's
// owner's, or a's own; 0 for none.
func actingClanID(a any) int32 {
	if s, ok := a.(interface {
		Owner() (attackable.Combatant, bool)
	}); ok {
		if owner, ok := s.Owner(); ok && owner != nil {
			a = owner
		}
	}
	if c, ok := a.(interface{ ClanID() int32 }); ok {
		return c.ClanID()
	}
	return 0
}

// AllowsDoorAttack reports whether attacker may attack one of the
// castle's doors, wall telling a wall: only while the siege is in
// progress, only for a player of an attacking clan or its summon, and a
// wall only through a siege summon other than the Swoop Cannon.
func (s *Siege) AllowsDoorAttack(attacker skilltarget.Actor, wall bool) bool {
	clanID := actingClanID(attacker)
	s.e.mu.Lock()
	attacking := s.status == StatusInProgress && clanID != 0 && s.sideLocked(clanID) == SideAttacker
	s.e.mu.Unlock()
	if !attacking {
		return false
	}
	return !wall || siegeWeapon(attacker)
}

// AllowsDoorDamage reports whether a hit lowers one of the castle's doors'
// HP: while the siege is in progress, from any attacker but the Swoop
// Cannon.
func (s *Siege) AllowsDoorDamage(attacker attackable.Combatant) bool {
	return s.InProgress() && !swoopCannon(attacker)
}

// ShowsDoorHP reports whether the castle's doors show their HP: while the
// siege is in progress.
func (s *Siege) ShowsDoorHP() bool { return s.InProgress() }

// DoorBroken tells the owner and the defenders, while the siege is in
// progress, that a wall of the castle took damage or a gate was broken
// down.
func (s *Siege) DoorBroken(wall bool) {
	id := MsgCastleGateBrokenDown
	if wall {
		id = MsgCastleWallDamaged
	}
	var fx effects
	s.e.mu.Lock()
	if s.status == StatusInProgress {
		s.tellLocked(&fx, Message{ID: id}, SideDefender)
	}
	s.e.mu.Unlock()
	fx.run()
}

// closeGatesLocked closes the castle's gates.
func (s *Siege) closeGatesLocked(fx *effects) {
	n, gates := s.e.notify, s.castle.Gates
	if n == nil || len(gates) == 0 {
		return
	}
	fx.add(func() { n.CloseDoors(gates) })
}

// respawnGatesLocked stands the castle's gates again, closed, at full HP or
// at half of it when weak.
func (s *Siege) respawnGatesLocked(fx *effects, weak bool) {
	n, gates := s.e.notify, s.castle.Gates
	if n == nil || len(gates) == 0 {
		return
	}
	fx.add(func() { n.RespawnDoors(gates, weak) })
}

// spawnGuardsLocked spawns the castle's siege guards.
func (s *Siege) spawnGuardsLocked(fx *effects) {
	if n := s.e.notify; n != nil {
		c := s.castle
		fx.add(func() { n.SpawnGuards(c) })
	}
}

// despawnGuardsLocked deletes the castle's siege guards.
func (s *Siege) despawnGuardsLocked(fx *effects) {
	if n := s.e.notify; n != nil {
		c := s.castle
		fx.add(func() { n.DespawnGuards(c) })
	}
}
