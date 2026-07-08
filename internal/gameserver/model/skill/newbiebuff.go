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
	skillID, err := set.GetInt32("skillId")
	if err != nil {
		return NewbieBuff{}, fmt.Errorf("skill: newbie buff: %w", err)
	}
	wrap := func(err error) error { return fmt.Errorf("skill: newbie buff %d: %w", skillID, err) }

	skillLevel, err := set.GetInt("skillLevel")
	if err != nil {
		return NewbieBuff{}, wrap(err)
	}
	lowerLevel, err := set.GetInt("lowerLevel")
	if err != nil {
		return NewbieBuff{}, wrap(err)
	}
	upperLevel, err := set.GetInt("upperLevel")
	if err != nil {
		return NewbieBuff{}, wrap(err)
	}

	return NewbieBuff{
		Skill:        Ref{ID: ID(skillID), Level: skillLevel},
		LowerLevel:   lowerLevel,
		UpperLevel:   upperLevel,
		IsMagicClass: set.GetBoolDefault("isMagicClass", false),
	}, nil
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
