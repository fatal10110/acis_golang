package player

import (
	"math"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/creature"
)

// TickRegen restores each resource below its current maximum once, then
// settles the regeneration task, so a tick that fills the character, or
// finds it dead, stops it. A character feigning death keeps its task but
// regenerates nothing.
func (c *Character) TickRegen() {
	if c.AlikeDead() {
		c.SettleRegen()
		return
	}
	changed := c.AddHP(math.Max(1, c.HPRegenRate())) > 0
	// A regeneration write reports its status only once the tick is done,
	// so its low-HP event comes first.
	c.lowHPNotice()
	changed = c.AddMP(math.Max(1, c.MPRegenRate())) > 0 || changed
	changed = c.AddCP(math.Max(1, c.CPRegenRate())) > 0 || changed
	if changed {
		c.BroadcastStatus()
		return
	}
	c.SettleRegen()
}

// Regen returns c's regeneration phase, which the regeneration sweep
// polls and claims.
func (c *Character) Regen() *creature.Regen { return &c.regen }

// SettleRegen arms c's regeneration task when it is in the world, alive and
// short of HP, MP or CP, its first tick one period from now, and disarms
// it otherwise (CreatureStatus.setHp/setMp and PlayerStatus.setCp's start and
// stop). A character not yet attached has no queue and stays idle. A
// teleport keeps the task on its grid: off the grid until Appearing, c is
// still in the world.
//
// Every status report (BroadcastStatus) and every vitals write except
// AddHP, SetHP and ReduceCurrentHP reaches it once its lock is released,
// so it then raises the low-HP tutorial event an HP write owes
// (lowHPNotice). Those three run settleRegenTask instead and leave the
// event to their caller's status report (writeHPLocked); a caller that
// skips the report delays the event to the next settle.
func (c *Character) SettleRegen() {
	c.settleRegenTask()
	c.lowHPNotice()
}

// SettleEnteredRegen arms c's regeneration task as c enters the world short
// of HP, MP or CP restored before it was in the world, its first tick one
// period from the entry. It raises no tutorial event: the restore reported
// its HP at the selection (NotifyRestoredHP).
func (c *Character) SettleEnteredRegen() {
	c.settleRegenTask()
}

// settleRegenTask is SettleRegen without the low-HP tutorial event: the
// HP writers whose caller reports the status run it, so the event follows
// that report (writeHPLocked).
func (c *Character) settleRegenTask() {
	c.regen.Settle(c.liveLocked().Queue(), c.regenShort)
}

// regenShort reports whether c regenerates: in the world (on the grid or
// off it mid-teleport), not dead, and below its maximum HP, MP or CP.
func (c *Character) regenShort() bool {
	if !c.Spawned() || c.Dead() {
		return false
	}
	res := c.ResourceValues()
	return res.CurrentHP < res.MaxHP || res.CurrentMP < res.MaxMP || res.CurrentCP < res.MaxCP
}
