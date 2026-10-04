package inventory

import (
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
