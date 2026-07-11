package item

import (
	"fmt"

	"github.com/fatal10110/acis_golang/internal/commons"
)

type SummonItem struct {
	ItemID     int32
	NPCID      int32
	SummonType int
}

func NewSummonItem(set *commons.StatSet) (SummonItem, error) {
	idf := commons.NewFields(set, "item: summon item")
	itemID := idf.Int32("id")
	if err := idf.Err(); err != nil {
		return SummonItem{}, err
	}

	f := commons.NewFields(set, fmt.Sprintf("item: summon item %d", itemID))
	item := SummonItem{
		ItemID:     itemID,
		NPCID:      f.Int32("npcId"),
		SummonType: f.Int("summonType"),
	}
	if err := f.Err(); err != nil {
		return SummonItem{}, err
	}
	return item, nil
}

type SummonItemTable struct {
	items map[int32]SummonItem
}

func NewSummonItemTable(items []SummonItem) (*SummonItemTable, error) {
	itemMap := make(map[int32]SummonItem, len(items))
	for _, entry := range items {
		if _, exists := itemMap[entry.ItemID]; exists {
			return nil, fmt.Errorf("item: duplicate summon item %d", entry.ItemID)
		}
		itemMap[entry.ItemID] = entry
	}
	return &SummonItemTable{items: itemMap}, nil
}

func (t *SummonItemTable) Item(itemID int32) (SummonItem, bool) {
	value, ok := t.items[itemID]
	return value, ok
}

func (t *SummonItemTable) Count() int { return len(t.items) }
