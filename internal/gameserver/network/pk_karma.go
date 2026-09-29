package network

import "github.com/fatal10110/acis_golang/internal/gameserver/model/item"

// applyPKKarmaSideEffects runs what a PK karma gain costs the killer, on its
// own queue: every equipped item whose conditions it no longer meets comes
// off, then its PvP flag task stops and the flag resets.
func (l *GameClientLink) applyPKKarmaSideEffects(live *livePlayer) {
	postLive(live, func() {
		if live.detached() {
			return
		}
		l.unequipRestrictedItems(live)
		if l.pvpFlags != nil {
			l.pvpFlags.Remove(live.Character, true)
		}
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
