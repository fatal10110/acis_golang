package script

import (
	"github.com/fatal10110/acis_golang/internal/gameserver/model/spawn"
	"github.com/fatal10110/acis_golang/internal/gameserver/siege"
)

// The castle siege operations of a maker (#3805): the castle by id, the
// registration on its siege's status changes, the siege's status and the
// castle's standing life control towers.

// SiegeStatus is where a castle's siege stands in its cycle.
type SiegeStatus = siege.Status

// The siege statuses a maker switches on.
const (
	SiegeRegistrationOpened = siege.StatusRegistrationOpened
	SiegeRegistrationOver   = siege.StatusRegistrationOver
	SiegeInProgress         = siege.StatusInProgress
)

// Castle returns the castle id, nil when there is none.
func (m *Maker) Castle(id int32) *Castle {
	if m.world.Castles == nil {
		return nil
	}
	c, ok := m.world.Castles.Get(int(id))
	if !ok {
		return nil
	}
	h := &Castle{c: c}
	if m.world.Sieges != nil {
		h.siege, _ = m.world.Sieges.Get(c.ID)
	}
	return h
}

// AddMakerEvent registers the npcmaker g, run by m, on the castle's siege:
// each later status change of the siege runs m's siege event hook with
// the new status, after the makers registered before it, on the goroutine
// that changed it. A castle whose siege the server does not run registers
// nothing.
func (c *Castle) AddMakerEvent(m *Maker, g spawn.Group) {
	if c.siege == nil {
		return
	}
	if m.run == nil {
		panic("script: a maker the registry did not build cannot register on a siege")
	}
	run := m.run
	c.siege.OnStatusChange(func(st siege.Status) { run.SiegeEvent(g, c, st) })
}

// SiegeStatus returns where the castle's siege stands now; a siege the
// server does not run takes registrations.
func (c *Castle) SiegeStatus() SiegeStatus {
	if c.siege == nil {
		return SiegeRegistrationOpened
	}
	return c.siege.Status()
}

// AliveLifeTowers counts the castle's placed life control towers standing:
// every one outside the siege, the working ones while it is fought.
func (c *Castle) AliveLifeTowers() int {
	return c.c.AliveLifeTowers(c.siege != nil && c.siege.InProgress())
}
