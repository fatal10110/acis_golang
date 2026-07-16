package itemcontainer

import "github.com/fatal10110/acis_golang/internal/gameserver/model/item"

// PackageSendableItems returns carried inventory items that may be sent as freight.
func (inv *Inventory) PackageSendableItems() []*item.Instance {
	if inv == nil {
		return nil
	}
	items := inv.Items()
	out := make([]*item.Instance, 0, len(items))
	for _, inst := range items {
		if inst == nil || inst.Location != item.LocationInventory {
			continue
		}
		tmpl, ok := inv.Templates().Get(inst.TemplateID)
		if !ok || inst.QuestItem(tmpl) || !inst.Tradable(tmpl) {
			continue
		}
		out = append(out, inst)
	}
	return out
}
