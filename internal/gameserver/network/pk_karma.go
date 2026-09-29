package network

import "github.com/fatal10110/acis_golang/internal/gameserver/model/item"

// applyPKKarmaSideEffects runs what a PK karma gain costs the killer, on its
// own queue: every equipped item whose conditions it no longer meets comes
// off, then its PvP flag task stops and the flag resets. Until the task has
// run, live's PvP flag requests queue behind it (see applyPvPFlag).
func (l *GameClientLink) applyPKKarmaSideEffects(live *livePlayer) {
	live.pkSideEffectsPending.Add(1)
	if !postLive(live, func() {
		defer live.pkSideEffectsPending.Add(-1)
		if live.detached() {
			return
		}
		l.unequipRestrictedItems(live)
		if l.pvpFlags != nil {
			l.pvpFlags.Remove(live.Character, true)
		}
	}) {
		live.pkSideEffectsPending.Add(-1)
	}
}

// applyPvPFlag starts or refreshes live's PvP flag window. A flag request
// that arrives while a PK side-effect task is pending is posted behind it,
// so the queue runs the reset first: an offensive skill that kills an
// innocent player flags its caster again after the kill, and the caster
// ends flagged.
func (l *GameClientLink) applyPvPFlag(live *livePlayer, useFlaggedDuration bool) {
	if l.pvpFlags == nil {
		return
	}
	apply := func() {
		if useFlaggedDuration {
			l.pvpFlags.AddFlagged(live.Character)
			return
		}
		l.pvpFlags.AddNormal(live.Character)
	}
	if live.pkSideEffectsPending.Load() == 0 {
		apply()
		return
	}
	postLive(live, func() {
		if live.detached() {
			return
		}
		apply()
	})
}

// unequipRestrictedItems takes off, through the UseItem equip toggle, every
// paperdoll item whose use conditions live fails. Taking off a weapon also
// aborts the attack in progress. An item an earlier removal in the same
// pass already took off is not toggled back on.
func (l *GameClientLink) unequipRestrictedItems(live *livePlayer) {
	inv := live.Inventory()
	if inv == nil || l.inventory == nil {
		return
	}
	for _, inst := range inv.PaperdollItems() {
		tmpl, ok := inv.Templates().Get(inst.TemplateID)
		if !ok || !inst.Equipped() || useConditionsHold(live, tmpl) {
			continue
		}
		l.toggleEquipItem(live, inv, inst, tmpl, tmpl.Kind == item.KindWeapon)
	}
}
