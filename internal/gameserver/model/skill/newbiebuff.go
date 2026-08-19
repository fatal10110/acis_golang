package skill

type NewbieBuff struct {
	Skill        Ref
	LowerLevel   int
	UpperLevel   int
	IsMagicClass bool
}

// NewNewbieBuff builds a NewbieBuff from one <buff> element's decoded
// attributes.
func NewNewbieBuff(skillID int32, skillLevel, lowerLevel, upperLevel int, isMagicClass bool) NewbieBuff {
	return NewbieBuff{
		Skill:        Ref{ID: ID(skillID), Level: skillLevel},
		LowerLevel:   lowerLevel,
		UpperLevel:   upperLevel,
		IsMagicClass: isMagicClass,
	}
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
