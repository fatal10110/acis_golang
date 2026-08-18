package skill

import (
	"fmt"
)

type HealSps struct {
	SkillID    ID
	SkillLevel int
	MagicLevel int
	Correction float64
	NeededMAtk int
}

// NewHealSps builds a HealSps from one <healSps> element's decoded
// attributes. skillID and skillLevel are set together (both nil, or both
// present); at least one of the skill selector or magicLevel is required.
func NewHealSps(correction float64, neededMAtk int, skillID *int32, skillLevel *int, magicLevel *int) (HealSps, error) {
	entry := HealSps{Correction: correction, NeededMAtk: neededMAtk}

	if skillID != nil {
		if skillLevel == nil {
			return HealSps{}, fmt.Errorf("skill: heal sps %d: skillLevel is required", *skillID)
		}
		entry.SkillID = ID(*skillID)
		entry.SkillLevel = *skillLevel
	}
	if magicLevel != nil {
		entry.MagicLevel = *magicLevel
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
