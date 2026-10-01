package augmentation

import "fmt"

// An augmentation id packs two 16-bit option ids: stat12 in the low half
// and stat34 in the high half. An option id from 1 to 14560 is a stat
// option: 4 color blocks of 3640 ids, each 40 levels of 91 ids, where the
// first 13 of a level are the solo stats and the other 78 every pair of
// them. 14561 to 16340 are the skill options, and 16341 to 16344 the base
// stat options (STR, CON, INT, MEN).
const (
	statStart        = 1
	statEnd          = 14560
	statBlockSize    = 3640
	statSubBlockSize = 91
	statNum          = 13
	statColors       = 4

	baseStatSTR = 16341
	baseStatCON = 16342
	baseStatINT = 16343
	baseStatMEN = 16344
)

// stat1Map and stat2Map name, for each offset within a level's 91 ids, the
// two stat tables it combines: equal for a solo stat, distinct for a pair.
var stat1Map, stat2Map [statSubBlockSize]int

func init() {
	idx := 0
	for ; idx < statNum; idx++ {
		stat1Map[idx], stat2Map[idx] = idx, idx
	}
	for i := 0; i < statNum; i++ {
		for j := i + 1; j < statNum; j++ {
			stat1Map[idx], stat2Map[idx] = i, j
			idx++
		}
	}
}

// Bonus is one flat stat bonus an augmentation adds while its item is worn.
type Bonus struct {
	// Stat is the stat's datapack name ("pAtk", "maxHp", "STR").
	Stat  string
	Value float32
}

// Bonuses returns the stat bonuses augmentation id grants, stat12's first.
// A solo stat option gives one bonus at its solo value, a pair two at their
// combined value, and a base stat option +1 to its base stat; a skill
// option gives none.
func (t *Table) Bonuses(id int32) []Bonus {
	var out []Bonus
	for _, option := range [2]int32{id & 0xffff, id >> 16} {
		switch {
		case option >= statStart && option <= statEnd:
			base := int(option - statStart)
			color := base / statBlockSize
			sub := base % statBlockSize
			level := sub / statSubBlockSize
			offset := sub % statSubBlockSize
			stats := t.byColor[color]
			s1, s2 := stat1Map[offset], stat2Map[offset]
			if s1 == s2 {
				out = append(out, Bonus{Stat: stats[s1].Name, Value: valueAt(stats[s1].SoloValues, level)})
				continue
			}
			out = append(out,
				Bonus{Stat: stats[s1].Name, Value: valueAt(stats[s1].CombinedValues, level)},
				Bonus{Stat: stats[s2].Name, Value: valueAt(stats[s2].CombinedValues, level)})
		case option >= baseStatSTR && option <= baseStatMEN:
			out = append(out, Bonus{Stat: [...]string{"STR", "CON", "INT", "MEN"}[option-baseStatSTR], Value: 1})
		}
	}
	return out
}

// valueAt reads a stat table at level, clamping a level past its end to
// the last value.
func valueAt(values []float32, level int) float32 {
	if level < 0 || level >= len(values) {
		return values[len(values)-1]
	}
	return values[level]
}

// BonusStats returns the name of every stat a bonus can target, for a
// caller that has to check them all once.
func (t *Table) BonusStats() []string {
	names := []string{"STR", "CON", "INT", "MEN"}
	for _, stats := range t.byColor {
		for _, s := range stats {
			names = append(names, s.Name)
		}
	}
	return names
}

func (t *Table) validate() error {
	for color, stats := range t.byColor {
		if len(stats) < statNum {
			return fmt.Errorf("augmentation stat group order %d: %d stats, want %d", color, len(stats), statNum)
		}
	}
	for level := range skillLevels {
		for _, bucket := range [...]struct {
			name string
			ids  []int
		}{{"blue", t.Blue[level]}, {"purple", t.Purple[level]}, {"red", t.Red[level]}} {
			if len(bucket.ids) == 0 {
				return fmt.Errorf("augmentation skills: no %s skill at life stone level %d", bucket.name, level)
			}
		}
	}
	return nil
}

// Life stone grades: a life stone's grade raises the chance of a skill and
// a glow.
const (
	GradeNone = iota
	GradeMid
	GradeHigh
	GradeTop
)

// LifeStone is a life stone's grade and level.
type LifeStone struct {
	Grade int
	Level int
}

const (
	firstLifeStoneID = 8723
	lifeStoneGrades  = 4
)

// lifeStonePlayerLevels is the lowest character level that may use a life
// stone of each level.
var lifeStonePlayerLevels = [skillLevels]int{46, 49, 52, 55, 58, 61, 64, 67, 70, 76}

// LifeStoneByItemID resolves the life stone item itemID: ten levels of no
// grade from 8723, then ten each of mid, high and top grade.
func LifeStoneByItemID(itemID int32) (LifeStone, bool) {
	n := int(itemID) - firstLifeStoneID
	if n < 0 || n >= lifeStoneGrades*skillLevels {
		return LifeStone{}, false
	}
	return LifeStone{Grade: n / skillLevels, Level: n % skillLevels}, true
}

// PlayerLevel is the lowest character level that may use ls.
func (ls LifeStone) PlayerLevel() int {
	return lifeStonePlayerLevels[ls.Level]
}

// Chances are the configured percent chances of a generated augmentation
// carrying a skill or a glow, per life stone grade, and of carrying a base
// stat when it has no skill.
type Chances struct {
	Skill    [lifeStoneGrades]int
	Glow     [lifeStoneGrades]int
	BaseStat int
}

// DefaultChances are the chances players.properties ships with.
func DefaultChances() Chances {
	return Chances{
		Skill:    [lifeStoneGrades]int{15, 30, 45, 60},
		Glow:     [lifeStoneGrades]int{0, 40, 70, 100},
		BaseStat: 1,
	}
}

// Rand draws a uniform int from min to max, both inclusive.
type Rand func(min, max int) int

// Generated is a freshly rolled augmentation.
type Generated struct {
	// ID packs stat12 in its low and stat34 in its high 16 bits.
	ID int32
	// Skill is the skill option stat34 names, when it names one.
	Skill *Skill
}

// Generate rolls a new augmentation for a life stone of level and grade,
// drawing every random number from rnd in a fixed order: the skill and
// glow rolls, the base stat rolls when no skill comes, the color, the skill
// pick, then the stat options.
func (t *Table) Generate(level, grade int, chances Chances, rnd Rand) Generated {
	level = min(level, skillLevels-1)
	var stat12, stat34 int
	var generateSkill, generateGlow bool
	if grade >= 0 && grade < lifeStoneGrades {
		generateSkill = rnd(1, 100) <= chances.Skill[grade]
		generateGlow = rnd(1, 100) <= chances.Glow[grade]
	}
	if !generateSkill && rnd(1, 100) <= chances.BaseStat {
		stat34 = rnd(baseStatSTR, baseStatMEN)
	}

	// 0 yellow, 1 blue, 2 purple, 3 red.
	color := rnd(0, 100)
	if stat34 == 0 && !generateSkill {
		if color <= 15*grade+40 {
			color = 1
		} else {
			color = 0
		}
	} else {
		switch {
		case color <= 10*grade+5 || stat34 != 0:
			color = 3
		case color <= 10*grade+10:
			color = 1
		default:
			color = 2
		}
	}

	var out Generated
	if generateSkill {
		var bucket []int
		switch color {
		case 1:
			bucket = t.Blue[level]
		case 2:
			bucket = t.Purple[level]
		case 3:
			bucket = t.Red[level]
		}
		stat34 = bucket[rnd(0, len(bucket)-1)]
		if s, ok := t.bySkillID[stat34]; ok {
			out.Skill = &s
		}
	}

	var offset int
	if stat34 == 0 {
		temp := rnd(2, 3)
		colorOffset := color*(10*statSubBlockSize) + temp*statBlockSize + 1
		offset = level*statSubBlockSize + colorOffset
		stat34 = rnd(offset, offset+statSubBlockSize-1)
		if generateGlow && grade >= 2 {
			offset = level*statSubBlockSize + (temp-2)*statBlockSize + grade*(10*statSubBlockSize) + 1
		} else {
			offset = level*statSubBlockSize + (temp-2)*statBlockSize + rnd(0, 1)*(10*statSubBlockSize) + 1
		}
	} else if !generateGlow {
		offset = level*statSubBlockSize + rnd(0, 1)*statBlockSize + 1
	} else {
		offset = level*statSubBlockSize + rnd(0, 1)*statBlockSize + (grade+color)/2*(10*statSubBlockSize) + 1
	}
	stat12 = rnd(offset, offset+statSubBlockSize-1)
	out.ID = int32(stat34<<16 + stat12)
	return out
}
