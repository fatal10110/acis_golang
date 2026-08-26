package item

import "fmt"

type SummonItem struct {
	ItemID     int32
	NPCID      int32
	SummonType int
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
