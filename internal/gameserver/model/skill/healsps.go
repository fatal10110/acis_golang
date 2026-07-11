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
	f := commons.NewFields(set, "skill: heal sps")
	entry := HealSps{
		Correction: f.Float64("correction"),
		NeededMAtk: f.Int("neededMatk"),
	}
	if err := f.Err(); err != nil {
		return HealSps{}, err
	}

	if f.Has("skillId") {
		sf := commons.NewFields(set, "skill: heal sps skill selector")
		skillID := sf.Int32("skillId")
		if err := sf.Err(); err != nil {
			return HealSps{}, err
		}
		lf := commons.NewFields(set, fmt.Sprintf("skill: heal sps %d", skillID))
		entry.SkillID = ID(skillID)
		entry.SkillLevel = lf.Int("skillLevel")
		if err := lf.Err(); err != nil {
			return HealSps{}, err
		}
	}
	if f.Has("magicLevel") {
		mf := commons.NewFields(set, "skill: heal sps magic selector")
		entry.MagicLevel = mf.Int("magicLevel")
		if err := mf.Err(); err != nil {
			return HealSps{}, err
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
