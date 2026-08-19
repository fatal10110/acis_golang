package entity

import (
	"fmt"
	"sort"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
)

// CursedWeapon is one cursed weapon definition loaded from XML.
type CursedWeapon struct {
	ItemID          int32
	Skill           skill.Ref
	Name            string
	DropRate        int
	Duration        int
	DurationLost    int
	DisappearChance int
	StageKills      int
}

// NewCursedWeapon builds a CursedWeapon from one XML item's decoded attributes.
func NewCursedWeapon(itemID int32, skillID int32, name string, dropRate, duration, durationLost, disappearChance, stageKills int, skills *skill.Table) (CursedWeapon, error) {
	if skills == nil {
		return CursedWeapon{}, fmt.Errorf("entity: cursed weapon %d: missing skill table", itemID)
	}
	skillLevel := skills.MaxLevel(skill.ID(skillID))
	if skillLevel <= 0 {
		return CursedWeapon{}, fmt.Errorf("entity: cursed weapon %d: skill %d not found", itemID, skillID)
	}

	return CursedWeapon{
		ItemID:          itemID,
		Skill:           skill.Ref{ID: skill.ID(skillID), Level: skillLevel},
		Name:            name,
		DropRate:        dropRate,
		Duration:        duration,
		DurationLost:    durationLost,
		DisappearChance: disappearChance,
		StageKills:      stageKills,
	}, nil
}

// CursedWeaponTable is an in-memory lookup of cursed weapon definitions by item id.
type CursedWeaponTable struct {
	byItemID map[int32]CursedWeapon
}

// NewCursedWeaponTable builds a CursedWeaponTable and rejects duplicate item ids.
func NewCursedWeaponTable(weapons []CursedWeapon) (*CursedWeaponTable, error) {
	byItemID := make(map[int32]CursedWeapon, len(weapons))
	for _, weapon := range weapons {
		if _, exists := byItemID[weapon.ItemID]; exists {
			return nil, fmt.Errorf("entity: duplicate cursed weapon item id %d", weapon.ItemID)
		}
		byItemID[weapon.ItemID] = weapon
	}
	return &CursedWeaponTable{byItemID: byItemID}, nil
}

// Count returns the number of cursed weapon definitions in the table.
func (t *CursedWeaponTable) Count() int {
	return len(t.byItemID)
}

// IDs returns the loaded cursed weapon item ids in deterministic order.
func (t *CursedWeaponTable) IDs() []int32 {
	if t == nil || len(t.byItemID) == 0 {
		return nil
	}
	ids := make([]int32, 0, len(t.byItemID))
	for id := range t.byItemID {
		ids = append(ids, id)
	}
	sort.Slice(ids, func(i, j int) bool { return ids[i] < ids[j] })
	return ids
}

// Weapon returns the cursed weapon for itemID, if present.
func (t *CursedWeaponTable) Weapon(itemID int32) (CursedWeapon, bool) {
	weapon, ok := t.byItemID[itemID]
	return weapon, ok
}
