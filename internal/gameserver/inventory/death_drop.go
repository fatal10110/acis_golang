package inventory

import (
	"iter"
	"slices"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/item"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/itemcontainer"
)

// DeathDroppable reports whether a player's death may drop inst from inv,
// and returns its template. Only an item that may be dropped at all can
// go; a shadow item, adena, a quest item and any template kept lists stay
// whatever the roll.
func DeathDroppable(inv *itemcontainer.Inventory, inst *item.Instance, kept []int32) (*item.Template, bool) {
	if inv == nil || inst == nil || inst.TemplateID == item.AdenaID || slices.Contains(kept, inst.TemplateID) {
		return nil, false
	}
	tmpl, ok := inv.Templates().Get(inst.TemplateID)
	if !ok || !inst.Dropable(tmpl) || inst.ShadowItem(tmpl) || inst.QuestItem(tmpl) {
		return nil, false
	}
	return tmpl, true
}

// DeathDropRates are the rules one player's death rolls its item drops
// under: with Chance percent the death drops some, then each droppable item
// drops with WeaponChance percent for a worn weapon, EquipChance for any
// other worn item and ItemChance for one not worn, until Limit items have
// dropped. A game master drops nothing unless GMDrops is set, and no item
// whose template is in Kept ever drops.
type DeathDropRates struct {
	Chance       int
	EquipChance  int
	WeaponChance int
	ItemChance   int
	Limit        int
	GMDrops      bool
	Kept         []int32
}

// DeathDropper is the dead player whose items DeathDrops rolls.
type DeathDropper interface {
	// Roll returns a random number in [0, n).
	Roll(n int) int
	// ControlItemInUse reports whether objectID is the collar of the pet
	// the player has out or rides.
	ControlItemInUse(objectID int32) bool
}

// DeathDrops yields, in container order, the items of inv that the death
// of p, a game master when gm, drops at rates. Nothing drops for a game
// master unless rates lets it, nor unless the first roll passes. A worn
// candidate comes off before its own roll, whether or not it then drops,
// and unequipped hears what that took off. The collar of the pet p has out
// or rides never drops. It stops once Limit items were yielded; the caller
// drops each one before the next roll.
func DeathDrops(inv *itemcontainer.Inventory, gm bool, rates DeathDropRates, p DeathDropper, unequipped func(changed []*item.Instance)) iter.Seq[*item.Instance] {
	return func(yield func(*item.Instance) bool) {
		if gm && !rates.GMDrops || inv == nil {
			return
		}
		if rates.Chance <= 0 || p.Roll(100) >= rates.Chance {
			return
		}
		dropped := 0
		for _, inst := range inv.Items() {
			tmpl, ok := DeathDroppable(inv, inst, rates.Kept)
			if !ok || p.ControlItemInUse(inst.ObjectID) {
				continue
			}
			chance := rates.ItemChance
			if inst.Equipped() {
				chance = rates.EquipChance
				if tmpl.Weapon != nil {
					chance = rates.WeaponChance
				}
				if changed := inv.UnequipItem(inst); len(changed) > 0 {
					unequipped(changed)
				}
			}
			if p.Roll(100) >= chance {
				continue
			}
			if !yield(inst) {
				return
			}
			dropped++
			if dropped >= rates.Limit {
				return
			}
		}
	}
}
