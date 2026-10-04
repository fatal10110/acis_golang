package summon

import (
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/zone"
)

// ZoneOwner is an owner that stands in zones as a player: the zone rules
// that judge a summon by its owner (a boss lair's entry list) read it.
type ZoneOwner interface {
	ZonePlayer() (zone.Player, bool)
}

// zoneMember is a summon's zone membership: the zone.Actor its zones see.
// A nil zoneMember holds no zone and ignores every position change; so
// does one with no zone index.
type zoneMember struct {
	zone.Member
	ix *zone.Index
	a  *Actor
}

var (
	_ zone.Owned   = (*zoneMember)(nil)
	_ zone.Swimmer = (*zoneMember)(nil)
	_ zone.Wader   = (*zoneMember)(nil)
)

// newZoneMember gives a its membership of the zones q holds, when q is a
// zone index.
func newZoneMember(a *Actor, q ZoneQuery) *zoneMember {
	ix, _ := q.(*zone.Index)
	return &zoneMember{ix: ix, a: a}
}

func (m *zoneMember) ObjectID() int32 { return m.a.ObjectID() }

func (m *zoneMember) Position() location.Location {
	x, y, z := m.a.Position()
	return location.Location{X: x, Y: y, Z: z}
}

func (m *zoneMember) Class() zone.Class { return zone.ClassSummon }

// Teleporting reports a teleport under way.
func (m *zoneMember) Teleporting() bool { return m.a.Teleporting() }

// Owner is the summon's owner as its zones see it.
func (m *zoneMember) Owner() (zone.Player, bool) {
	owner, ok := m.a.currentOwner().(ZoneOwner)
	if !ok {
		return nil, false
	}
	return owner.ZonePlayer()
}

// SwimStateChanged switches the summon's movement into or out of swimming.
// Its observers are shown nothing: only a player's or an NPC's appearance
// follows the water.
func (m *zoneMember) SwimStateChanged(swimming bool) {
	if m.a.movementReady.Load() {
		m.a.movement.SetSwimming(swimming)
	}
}

// SwampStateChanged applies the move bonus of the first swamp at the
// summon's position while any swamp holds it, and none once it is out.
func (m *zoneMember) SwampStateChanged(z *zone.Swamp) {
	bonus := int32(0)
	if m.ZoneFlags().Has(zone.FlagSwamp) {
		bonus = int32(z.MoveBonus)
		pos := m.Position()
		if first, ok := zone.FindAt[*zone.Swamp](m.ix, pos.X, pos.Y, pos.Z); ok {
			bonus = int32(first.MoveBonus)
		}
	}
	if m.a.swampMoveBonus.Swap(bonus) != bonus {
		m.a.refreshMoveSpeed()
	}
}

func (m *zoneMember) has(flag zone.Flag) bool {
	return m != nil && m.ZoneFlags().Has(flag)
}

func (m *zoneMember) enter() {
	if m != nil {
		m.Enter(m.ix, m)
	}
}

func (m *zoneMember) step(previous location.Location) {
	if m != nil {
		m.Step(m.ix, m, previous)
	}
}

func (m *zoneMember) place(previous location.Location) {
	if m != nil {
		m.Place(m.ix, m, previous)
	}
}

func (m *zoneMember) settle() {
	if m != nil {
		m.Settle(m.ix, m)
	}
}

func (m *zoneMember) leave(at location.Location) {
	if m != nil {
		m.Leave(m.ix, m, at.X, at.Y)
	}
}

// EnterZones enters the summon into the zones at its position: it was
// placed beside its owner, or landed from a teleport.
func (a *Actor) EnterZones() { a.membership.enter() }

// LeaveZones takes the summon out of the zones around its position before a
// teleport moves it away; EnterZones puts it in those at the destination.
func (a *Actor) LeaveZones() { a.membership.leave(a.location()) }

// SettleZones revalidates the summon's zones at once, as a move ends.
func (a *Actor) SettleZones() { a.membership.settle() }

// InsideZone reports whether the summon's zones hold flag.
func (a *Actor) InsideZone(flag zone.Flag) bool { return a.membership.has(flag) }

func (a *Actor) location() location.Location {
	x, y, z := a.Position()
	return location.Location{X: x, Y: y, Z: z}
}
