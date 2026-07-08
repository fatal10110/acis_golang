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
	itemID, err := set.GetInt32("id")
	if err != nil {
		return SummonItem{}, fmt.Errorf("item: summon item: %w", err)
	}
	wrap := func(err error) error { return fmt.Errorf("item: summon item %d: %w", itemID, err) }

	npcID, err := set.GetInt32("npcId")
	if err != nil {
		return SummonItem{}, wrap(err)
	}
	summonType, err := set.GetInt("summonType")
	if err != nil {
		return SummonItem{}, wrap(err)
	}
	return SummonItem{ItemID: itemID, NPCID: npcID, SummonType: summonType}, nil
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
