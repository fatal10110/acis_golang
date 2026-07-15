package petitem

import "github.com/fatal10110/acis_golang/internal/gameserver/model/item"

// ForbiddenForPet reports whether an item cannot be placed in a pet inventory.
func ForbiddenForPet(inst *item.Instance, tmpl *item.Template) bool {
	if inst == nil || tmpl == nil {
		return true
	}
	if tmpl.HeroItem() || !inst.Dropable(tmpl) || !inst.Destroyable(tmpl) || !inst.Tradable(tmpl) {
		return true
	}
	return tmpl.EtcItem != nil && (tmpl.EtcItem.Type == item.EtcItemArrow || tmpl.EtcItem.Type == item.EtcItemShot)
}

// Equippable reports whether tmpl is a pet equipment item.
func Equippable(tmpl *item.Template) bool {
	if tmpl == nil {
		return false
	}
	return (tmpl.Weapon != nil && tmpl.Weapon.Type == item.WeaponPet) || (tmpl.Armor != nil && tmpl.Armor.Type == item.ArmorPet)
}
