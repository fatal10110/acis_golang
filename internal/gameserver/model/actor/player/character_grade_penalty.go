package player

import "github.com/fatal10110/acis_golang/internal/gameserver/model/item"

const (
	expertiseSkillID    = 239
	gradePenaltySkillID = 4267
)

// RefreshExpertisePenalty recalculates the equipped-item grade penalty.
func (c *Character) RefreshExpertisePenalty() {
	if c == nil || c.Inventory() == nil {
		return
	}

	expertise := c.SkillLevel(expertiseSkillID)
	armorPenalty := 0
	weaponPenalty := false
	for _, inst := range c.Inventory().PaperdollItems() {
		tmpl, ok := c.Inventory().Templates().Get(inst.TemplateID)
		if !ok || (tmpl.EtcItem != nil && tmpl.EtcItem.Type == item.EtcItemArrow) || int(tmpl.Crystal) <= expertise {
			continue
		}
		if tmpl.Weapon != nil {
			weaponPenalty = true
			continue
		}
		if tmpl.Slot == item.SlotFullArmor {
			armorPenalty += 2
		} else {
			armorPenalty++
		}
	}
	if armorPenalty > 4 {
		armorPenalty = 4
	}

	c.stateMu.Lock()
	changed := c.weaponGradePenalty != weaponPenalty || c.armorGradePenalty != armorPenalty
	if changed {
		c.weaponGradePenalty = weaponPenalty
		c.armorGradePenalty = armorPenalty
	}
	update, refresh := c.updateGradePenalty, c.refreshItemStats
	c.stateMu.Unlock()
	if !changed {
		return
	}
	if weaponPenalty || armorPenalty > 0 {
		c.SetSkillLevel(gradePenaltySkillID, 1)
	} else {
		c.SetSkillLevel(gradePenaltySkillID, 0)
	}
	if update != nil {
		update()
	}
	if refresh != nil {
		refresh()
	}
}

// WeaponGradePenalty reports whether an equipped weapon exceeds Expertise.
func (c *Character) WeaponGradePenalty() bool {
	c.stateMu.RLock()
	defer c.stateMu.RUnlock()
	return c.weaponGradePenalty
}

// ArmorGradePenalty reports the capped count of over-grade armor pieces.
func (c *Character) ArmorGradePenalty() int {
	c.stateMu.RLock()
	defer c.stateMu.RUnlock()
	return c.armorGradePenalty
}

// WeaponSkillsAllowed reports whether Expertise permits a weapon's item skills.
func (c *Character) WeaponSkillsAllowed(crystal item.CrystalType) bool {
	return c.SkillLevel(expertiseSkillID) >= int(crystal)
}

// SetGradePenaltyUpdater records the packet-layer notification for a changed penalty state.
func (c *Character) SetGradePenaltyUpdater(update func()) {
	c.stateMu.Lock()
	defer c.stateMu.Unlock()
	c.updateGradePenalty = update
}

// SetItemStatsRefresher records the item-passive refresh run after a penalty change.
func (c *Character) SetItemStatsRefresher(refresh func()) {
	c.stateMu.Lock()
	defer c.stateMu.Unlock()
	c.refreshItemStats = refresh
}
