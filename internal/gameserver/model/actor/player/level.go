package player

import (
	"errors"
	"sort"

	"github.com/fatal10110/acis_golang/internal/commons"
)

// Level holds the experience and death-penalty parameters associated with
// reaching one character level.
type Level struct {
	// RequiredExpToLevelUp is the total experience needed to advance from
	// the previous level into this one.
	RequiredExpToLevelUp int64
	// KarmaModifier scales karma gain/loss calculations at this level.
	KarmaModifier float64
	// ExpLossAtDeath is the percentage of the level's experience span lost
	// on death.
	ExpLossAtDeath float64
}

// NewLevel builds a Level from set. requiredExpToLevelUp is required;
// karmaModifier and expLossAtDeath default to 0 when absent (the sentinel
// entry above the level cap carries neither), but a present value that
// fails to parse is still an error.
func NewLevel(set *commons.StatSet) (Level, error) {
	exp, err := set.GetLong("requiredExpToLevelUp")
	if err != nil {
		return Level{}, err
	}

	l := Level{RequiredExpToLevelUp: exp}
	if l.KarmaModifier, err = set.GetDoubleDefault("karmaModifier", 0); err != nil {
		return Level{}, err
	}
	if l.ExpLossAtDeath, err = set.GetDoubleDefault("expLossAtDeath", 0); err != nil {
		return Level{}, err
	}
	return l, nil
}

// LevelTable is an in-memory lookup of per-level experience/penalty
// parameters keyed by level number, built once at boot and read for the
// remainder of the process lifetime. The zero value is not usable;
// construct with NewLevelTable.
type LevelTable struct {
	levels   map[int]Level
	maxLevel int
}

// NewLevelTable returns a LevelTable backed by levels. An empty map is an
// error: a table with no rows cannot answer the level-cap queries.
func NewLevelTable(levels map[int]Level) (*LevelTable, error) {
	if len(levels) == 0 {
		return nil, errors.New("player: level table has no entries")
	}
	maxLevel := 0
	for level := range levels {
		if level > maxLevel {
			maxLevel = level
		}
	}
	return &LevelTable{levels: levels, maxLevel: maxLevel}, nil
}

// Level returns the experience/penalty parameters for level, and whether an
// entry exists for it.
func (t *LevelTable) Level(level int) (Level, bool) {
	l, ok := t.levels[level]
	return l, ok
}

// Count returns the number of levels loaded.
func (t *LevelTable) Count() int {
	return len(t.levels)
}

// Levels returns every level number loaded, ordered ascending.
func (t *LevelTable) Levels() []int {
	levels := make([]int, 0, len(t.levels))
	for level := range t.levels {
		levels = append(levels, level)
	}
	sort.Ints(levels)
	return levels
}

// MaxLevel returns the first unreachable level: a sentinel entry present
// only to define the experience span of the highest attainable level (e.g.
// 81, when the highest attainable level is 80).
func (t *LevelTable) MaxLevel() int {
	return t.maxLevel
}

// RealMaxLevel returns the highest attainable character level.
func (t *LevelTable) RealMaxLevel() int {
	return t.maxLevel - 1
}

// RequiredExpForHighestLevel returns the experience required to fill the
// experience bar at the level cap (the span between RealMaxLevel and
// MaxLevel). The entry always exists: maxLevel is the largest key present
// and construction rejects an empty table.
func (t *LevelTable) RequiredExpForHighestLevel() int64 {
	return t.levels[t.maxLevel].RequiredExpToLevelUp
}
