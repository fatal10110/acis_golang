package npc

import (
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/attackable"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/event"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/item"
)

// heldItems is what a hostile NPC holds: the item ids observers see in its
// hands, and what they resolve to.
type heldItems struct {
	rightHand, leftHand int
	// weapon is the right-hand item's weapon detail, nil when the NPC is
	// unarmed: no right-hand item, an unknown one, or one that is not a
	// weapon. weaponCrystal is its crystal grade.
	weapon        *item.WeaponDetail
	weaponCrystal item.CrystalType
	// offHandMask is the armor-type bit of the shield in its left hand,
	// zero when it holds none.
	offHandMask int32
}

// attackerRef boxes a creature for an atomic pointer.
type attackerRef struct{ c attackable.Combatant }

// resolveHeld resolves rightHand and leftHand through items, nil resolving
// neither.
func resolveHeld(items *item.Table, rightHand, leftHand int) *heldItems {
	held := &heldItems{rightHand: rightHand, leftHand: leftHand}
	if items == nil {
		return held
	}
	if leftHand != 0 {
		if tmpl, ok := items.Get(int32(leftHand)); ok && tmpl.Kind == item.KindArmor && tmpl.Armor != nil {
			held.offHandMask = tmpl.Armor.Type.Mask()
		}
	}
	if rightHand != 0 {
		if tmpl, ok := items.Get(int32(rightHand)); ok && tmpl.Weapon != nil {
			held.weapon, held.weaponCrystal = tmpl.Weapon, tmpl.Crystal
		}
	}
	return held
}

// hands returns what h holds now: its template's items until Attach.
func (h *Hostile) hands() *heldItems {
	if held := h.held.Load(); held != nil {
		return held
	}
	return &heldItems{rightHand: h.Instance.Template.RightHand, leftHand: h.Instance.Template.LeftHand}
}

// holdTemplateItems puts the template's items back in h's hands.
func (h *Hostile) holdTemplateItems() {
	h.held.Store(resolveHeld(h.items, h.Instance.Template.RightHand, h.Instance.Template.LeftHand))
}

// EquipItem puts the item rightHand in h's right hand and leftHand in its
// left, 0 for an empty hand, and shows every player around it h's new
// look. The right-hand item decides h's attack style and reuse from then
// on. Its death puts the template's items back.
func (h *Hostile) EquipItem(rightHand, leftHand int32) {
	h.held.Store(resolveHeld(h.items, int(rightHand), int(leftHand)))
	h.emit(event.NPCInfoShown{})
}

// SetEnchantEffect sets the enchant glow of h's weapon and shows every
// player around it h's new look. Its death clears the glow.
func (h *Hostile) SetEnchantEffect(enchant int32) {
	h.enchantEffect.Store(enchant)
	h.UpdateAbnormalEffect()
}

// LastAttacker returns the last creature that damaged h in this life, nil
// before any did.
func (h *Hostile) LastAttacker() attackable.Combatant {
	if ref := h.lastAttacker.Load(); ref != nil {
		return ref.c
	}
	return nil
}
