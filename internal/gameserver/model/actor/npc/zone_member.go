package npc

import (
	"sync/atomic"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/move"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/zone"
)

// zoneNPC is the NPC a zoneMember stands for.
type zoneNPC interface {
	ObjectID() int32
	Position() (x, y, z int)
	// zoneTeleporting reports a teleport under way: until it lands, the
	// NPC's zones are not revalidated.
	zoneTeleporting() bool
	// swimStateChanged switches the NPC's movement into or out of swimming
	// and shows its observers its new state.
	swimStateChanged(swimming bool)
}

// zoneMember is an NPC's zone membership: the zone.Actor its zones see. A
// nil zoneMember holds no zone and ignores every position change; so does
// one with no zone index.
type zoneMember struct {
	zone.Member
	ix  *zone.Index
	npc zoneNPC
	// swimming is the swim move type the water zones set and clear.
	swimming atomic.Bool
}

var (
	_ zone.Actor   = (*zoneMember)(nil)
	_ zone.Swimmer = (*zoneMember)(nil)
)

func newZoneMember(n zoneNPC) *zoneMember { return &zoneMember{npc: n} }

func (m *zoneMember) ObjectID() int32 { return m.npc.ObjectID() }

func (m *zoneMember) Position() location.Location {
	x, y, z := m.npc.Position()
	return location.Location{X: x, Y: y, Z: z}
}

func (m *zoneMember) Class() zone.Class { return zone.ClassNPC }

// Teleporting reports a teleport under way.
func (m *zoneMember) Teleporting() bool { return m.npc.zoneTeleporting() }

// SwimStateChanged is the NPC's reaction to crossing a water zone boundary.
func (m *zoneMember) SwimStateChanged(swimming bool) {
	m.swimming.Store(swimming)
	m.npc.swimStateChanged(swimming)
}

// has reports whether the NPC's zones hold flag.
func (m *zoneMember) has(flag zone.Flag) bool {
	return m != nil && m.ZoneFlags().Has(flag)
}

// moveType is the move type the NPC's client view shows: swimming while a
// water zone holds it, else on the ground.
func (m *zoneMember) moveType() int {
	if m != nil && m.swimming.Load() {
		return int(move.MoveSwim)
	}
	return int(move.MoveGround)
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

func (m *zoneMember) settle() {
	if m != nil {
		m.Settle(m.ix, m)
	}
}

func (m *zoneMember) place(previous location.Location) {
	if m != nil {
		m.Place(m.ix, m, previous)
	}
}

func (m *zoneMember) leave(at location.Location) {
	if m != nil {
		m.Leave(m.ix, m, at.X, at.Y)
	}
}
