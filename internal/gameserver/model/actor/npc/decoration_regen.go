package npc

import (
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/creature"
	"github.com/fatal10110/acis_golang/internal/gameserver/skill/stat"
)

// HPRegenRate returns the HP the decoration regenerates per tick: its
// template's base rate times the configured multiple, through its stats.
func (d *Decoration) HPRegenRate() float64 {
	return d.stats.calc(stat.RegenerateHPRate, d.Instance.Template.HPRegen*regenInForce(d.regenSettings).HP)
}

// TickRegen applies one regeneration step: a living decoration short of its
// maximum HP gains its regen rate, at least 1, capped at the maximum. Its MP
// is always full. Nobody watches its health bar, so the gain shows nobody.
// The phase is then settled, so a tick that fills the decoration, or finds
// it dead or gone, stops it.
func (d *Decoration) TickRegen() {
	maxHP, rate := d.MaxHPValue(), max(1, d.HPRegenRate())
	d.vitalsMu.Lock()
	if !d.dead && !d.removed && d.hp < maxHP {
		d.hp = min(d.hp+rate, maxHP)
	}
	d.vitalsMu.Unlock()
	d.SettleRegen()
}

// Regen returns the decoration's regeneration phase, which the regeneration
// sweep polls and claims.
func (d *Decoration) Regen() *creature.Regen { return &d.regen }

// SettleRegen arms the decoration's regeneration task when it is in the
// world, alive and short of its maximum HP, its first tick one period from
// now, and disarms it otherwise; every HP change settles it so.
func (d *Decoration) SettleRegen() {
	d.regen.Settle(d.queue, d.RegenShort)
}

// RegenShort reports whether the decoration regenerates: spawned, alive,
// not removed and below its maximum HP.
func (d *Decoration) RegenShort() bool {
	if !d.Spawned() {
		return false
	}
	maxHP := d.MaxHPValue()
	d.vitalsMu.Lock()
	defer d.vitalsMu.Unlock()
	return !d.dead && !d.removed && d.hp < maxHP
}
