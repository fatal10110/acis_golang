package skill

import (
	"fmt"

	"github.com/fatal10110/acis_golang/internal/commons"
)

type NewbieBuff struct {
	Skill        Ref
	LowerLevel   int
	UpperLevel   int
	IsMagicClass bool
}

func NewNewbieBuff(set *commons.StatSet) (NewbieBuff, error) {
	idf := commons.NewFields(set, "skill: newbie buff")
	skillID := idf.Int32("skillId")
	if err := idf.Err(); err != nil {
		return NewbieBuff{}, err
	}

	f := commons.NewFields(set, fmt.Sprintf("skill: newbie buff %d", skillID))
	buff := NewbieBuff{
		Skill:        Ref{ID: ID(skillID), Level: f.Int("skillLevel")},
		LowerLevel:   f.Int("lowerLevel"),
		UpperLevel:   f.Int("upperLevel"),
		IsMagicClass: f.BoolDefault("isMagicClass", false),
	}
	if err := f.Err(); err != nil {
		return NewbieBuff{}, err
	}
	return buff, nil
}

type NewbieBuffTable struct {
	buffs            []NewbieBuff
	lowestMagicLevel int
	lowestFightLevel int
}

func NewNewbieBuffTable(buffs []NewbieBuff) *NewbieBuffTable {
	table := &NewbieBuffTable{
		buffs:            append([]NewbieBuff(nil), buffs...),
		lowestMagicLevel: 100,
		lowestFightLevel: 100,
	}
	for _, buff := range buffs {
		if buff.IsMagicClass {
			if buff.LowerLevel < table.lowestMagicLevel {
				table.lowestMagicLevel = buff.LowerLevel
			}
			continue
		}
		if buff.LowerLevel < table.lowestFightLevel {
			table.lowestFightLevel = buff.LowerLevel
		}
	}
	return table
}

func (t *NewbieBuffTable) Count() int { return len(t.buffs) }

func (t *NewbieBuffTable) LowestBuffLevel(isMagicClass bool) int {
	if isMagicClass {
		return t.lowestMagicLevel
	}
	return t.lowestFightLevel
}

func (t *NewbieBuffTable) ValidBuffs(isMagicClass bool, level int) []NewbieBuff {
	var out []NewbieBuff
	for _, buff := range t.buffs {
		if buff.IsMagicClass != isMagicClass {
			continue
		}
		if level < buff.LowerLevel || level > buff.UpperLevel {
			continue
		}
		out = append(out, buff)
	}
	return out
}
