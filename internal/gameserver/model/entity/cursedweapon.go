package entity

import (
	"fmt"
	"sort"
	"sync/atomic"

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
// Replace swaps its definitions for every holder at once.
type CursedWeaponTable struct {
	byItemID atomic.Pointer[map[int32]CursedWeapon]
}

// NewCursedWeaponTable builds a CursedWeaponTable; later duplicate item ids replace earlier ones.
func NewCursedWeaponTable(weapons []CursedWeapon) (*CursedWeaponTable, error) {
	byItemID := make(map[int32]CursedWeapon, len(weapons))
	for _, weapon := range weapons {
		byItemID[weapon.ItemID] = weapon
	}
	t := &CursedWeaponTable{}
	t.byItemID.Store(&byItemID)
	return t, nil
}

// Replace swaps t's definitions for from's, at once for every holder of t:
// CursedWeaponManager.reload.
func (t *CursedWeaponTable) Replace(from *CursedWeaponTable) {
	t.byItemID.Store(from.byItemID.Load())
}

// weapons returns t's current definitions; none for a zero table.
func (t *CursedWeaponTable) weapons() map[int32]CursedWeapon {
	if p := t.byItemID.Load(); p != nil {
		return *p
	}
	return nil
}

// Count returns the number of cursed weapon definitions in the table.
func (t *CursedWeaponTable) Count() int {
	return len(t.weapons())
}

// IDs returns the loaded cursed weapon item ids in deterministic order.
func (t *CursedWeaponTable) IDs() []int32 {
	if t == nil {
		return nil
	}
	weapons := t.weapons()
	if len(weapons) == 0 {
		return nil
	}
	ids := make([]int32, 0, len(weapons))
	for id := range weapons {
		ids = append(ids, id)
	}
	sort.Slice(ids, func(i, j int) bool { return ids[i] < ids[j] })
	return ids
}

// Weapon returns the cursed weapon for itemID, if present.
func (t *CursedWeaponTable) Weapon(itemID int32) (CursedWeapon, bool) {
	weapon, ok := t.weapons()[itemID]
	return weapon, ok
}
