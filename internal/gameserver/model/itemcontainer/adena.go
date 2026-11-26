package itemcontainer

import "github.com/fatal10110/acis_golang/internal/gameserver/model/item"

// PayAdena takes count adena from inv. It reports false, taking nothing,
// when inv holds less or the adena cannot be destroyed; a count of 0 or less
// takes nothing and reports true. paid is the adena stack the count came out
// of, nil when nothing was taken. The caller sends its own notices.
func (inv *Inventory) PayAdena(count int) (paid *item.Instance, ok bool) {
	if count > inv.Adena() {
		return nil, false
	}
	if count <= 0 {
		return nil, true
	}
	paid = inv.DestroyByTemplateID(item.AdenaID, count)
	return paid, paid != nil
}
