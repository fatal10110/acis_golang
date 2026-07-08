package skill

import (
	"fmt"

	"github.com/fatal10110/acis_golang/internal/commons"
)

type HealSps struct {
	SkillID    ID
	SkillLevel int
	MagicLevel int
	Correction float64
	NeededMAtk int
}

func NewHealSps(set *commons.StatSet) (HealSps, error) {
	correction, err := set.GetDouble("correction")
	if err != nil {
		return HealSps{}, fmt.Errorf("skill: heal sps: %w", err)
	}
	neededMAtk, err := set.GetInt("neededMatk")
	if err != nil {
		return HealSps{}, fmt.Errorf("skill: heal sps: %w", err)
	}

	entry := HealSps{Correction: correction, NeededMAtk: neededMAtk}
	if set.Has("skillId") {
		skillID, err := set.GetInt32("skillId")
		if err != nil {
			return HealSps{}, fmt.Errorf("skill: heal sps skill selector: %w", err)
		}
		level, err := set.GetInt("skillLevel")
		if err != nil {
			return HealSps{}, fmt.Errorf("skill: heal sps %d: %w", skillID, err)
		}
		entry.SkillID = ID(skillID)
		entry.SkillLevel = level
	}
	if set.Has("magicLevel") {
		entry.MagicLevel, err = set.GetInt("magicLevel")
		if err != nil {
			return HealSps{}, fmt.Errorf("skill: heal sps magic selector: %w", err)
		}
	}
	if entry.SkillID == 0 && entry.MagicLevel == 0 {
		return HealSps{}, fmt.Errorf("skill: heal sps: need skillId/skillLevel or magicLevel")
	}
	return entry, nil
}

type HealSpsTable struct {
	entries []HealSps
}

func NewHealSpsTable(entries []HealSps) (*HealSpsTable, error) {
	return &HealSpsTable{entries: append([]HealSps(nil), entries...)}, nil
}

func (t *HealSpsTable) Count() int { return len(t.entries) }

func (t *HealSpsTable) Calculate(skillID ID, skillLevel, magicLevel, mAtk int) float64 {
	var selected *HealSps
	for i := range t.entries {
		entry := &t.entries[i]
		if entry.SkillID == skillID && entry.SkillLevel == skillLevel {
			selected = entry
			break
		}
	}
	if selected == nil && magicLevel > 0 {
		for i := range t.entries {
			entry := &t.entries[i]
			if entry.MagicLevel <= 0 || entry.MagicLevel > magicLevel {
				continue
			}
			if selected == nil || entry.MagicLevel > selected.MagicLevel {
				selected = entry
			}
		}
	}
	if selected == nil {
		return 0
	}

	amount := selected.Correction
	if diff := selected.NeededMAtk - mAtk; diff > 0 {
		amount -= float64(diff) / 2
	}
	return amount
}
