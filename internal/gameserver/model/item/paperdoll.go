package item

// PaperdollSlots is the number of equip-array positions the client's
// paperdoll model exposes (see Slot.PaperdollIndex).
const PaperdollSlots = 17

// PaperdollEntry is the contents of one equip-array position: which item
// occupies it, and at what enchant level. The zero value means nothing is
// equipped there.
type PaperdollEntry struct {
	ObjectID, TemplateID int32
	EnchantLevel         int
}

// Paperdoll builds the fixed-size equip array the client expects from a
// character's items, filling in whichever positions something is actually
// equipped and leaving the rest at the zero PaperdollEntry.
func Paperdoll(items []*Instance) [PaperdollSlots]PaperdollEntry {
	var out [PaperdollSlots]PaperdollEntry
	for _, inst := range items {
		st := inst.Snapshot()
		if st.Location != LocationPaperdoll {
			continue
		}
		if st.LocationData < 0 || st.LocationData >= PaperdollSlots {
			continue
		}
		out[st.LocationData] = PaperdollEntry{
			ObjectID:     st.ObjectID,
			TemplateID:   st.TemplateID,
			EnchantLevel: st.EnchantLevel,
		}
	}
	return out
}
