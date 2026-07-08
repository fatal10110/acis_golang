package entity

import (
	"fmt"

	"github.com/fatal10110/acis_golang/internal/commons"
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

// NewCursedWeapon builds a CursedWeapon from one XML item attribute set.
func NewCursedWeapon(set *commons.StatSet, skills *skill.Table) (CursedWeapon, error) {
	itemID, err := set.GetInt32("id")
	if err != nil {
		return CursedWeapon{}, fmt.Errorf("entity: cursed weapon: %w", err)
	}
	wrap := func(err error) error { return fmt.Errorf("entity: cursed weapon %d: %w", itemID, err) }

	skillID, err := set.GetInt32("skillId")
	if err != nil {
		return CursedWeapon{}, wrap(err)
	}
	if skills == nil {
		return CursedWeapon{}, wrap(fmt.Errorf("missing skill table"))
	}
	skillLevel := skills.MaxLevel(skill.ID(skillID))
	if skillLevel <= 0 {
		return CursedWeapon{}, wrap(fmt.Errorf("skill %d not found", skillID))
	}

	name, err := set.GetString("name")
	if err != nil {
		return CursedWeapon{}, wrap(err)
	}
	dropRate, err := set.GetInt("dropRate")
	if err != nil {
		return CursedWeapon{}, wrap(err)
	}
	duration, err := set.GetInt("duration")
	if err != nil {
		return CursedWeapon{}, wrap(err)
	}
	durationLost, err := set.GetInt("durationLost")
	if err != nil {
		return CursedWeapon{}, wrap(err)
	}
	disappearChance, err := set.GetInt("dissapearChance")
	if err != nil {
		return CursedWeapon{}, wrap(err)
	}
	stageKills, err := set.GetInt("stageKills")
	if err != nil {
		return CursedWeapon{}, wrap(err)
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

// Weapon returns the cursed weapon for itemID, if present.
func (t *CursedWeaponTable) Weapon(itemID int32) (CursedWeapon, bool) {
	weapon, ok := t.byItemID[itemID]
	return weapon, ok
}
