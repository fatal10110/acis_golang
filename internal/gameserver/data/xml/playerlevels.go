package xml

import (
	"encoding/xml"
	"fmt"
	"os"
)

// PlayerLevel holds the experience and death-penalty parameters associated
// with reaching a specific character level.
type PlayerLevel struct {
	// RequiredExp is the total experience needed to advance from the
	// previous level into this one.
	RequiredExp int64
	// KarmaModifier scales karma gain/loss calculations at this level.
	KarmaModifier float64
	// ExpLossAtDeath is the percentage of the level's experience span lost
	// on death.
	ExpLossAtDeath float64
}

// PlayerLevelTable holds every level's experience/penalty parameters, keyed
// by level number, as loaded from the player level table.
type PlayerLevelTable struct {
	levels   map[int]PlayerLevel
	maxLevel int
}

// playerLevelEntry is the wire shape of a single <playerLevel> element.
// Pointer fields distinguish "attribute absent" from "attribute present with
// its zero value", so required attributes can be validated and optional ones
// defaulted independently.
type playerLevelEntry struct {
	Level                *int     `xml:"level,attr"`
	RequiredExpToLevelUp *int64   `xml:"requiredExpToLevelUp,attr"`
	KarmaModifier        *float64 `xml:"karmaModifier,attr"`
	ExpLossAtDeath       *float64 `xml:"expLossAtDeath,attr"`
}

// playerLevelDocument is the wire shape of the player level table's root
// element.
type playerLevelDocument struct {
	XMLName xml.Name           `xml:"list"`
	Entries []playerLevelEntry `xml:"playerLevel"`
}

// LoadPlayerLevelTable reads and parses the player level/experience table
// from the XML file at path. It returns an error if the file cannot be read,
// is not well-formed XML, or an entry omits one of the required attributes
// (level, requiredExpToLevelUp).
func LoadPlayerLevelTable(path string) (*PlayerLevelTable, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("load player level table %q: %w", path, err)
	}

	var doc playerLevelDocument
	if err := xml.Unmarshal(data, &doc); err != nil {
		return nil, fmt.Errorf("parse player level table %q: %w", path, err)
	}

	levels := make(map[int]PlayerLevel, len(doc.Entries))
	maxLevel := 0
	for _, e := range doc.Entries {
		if e.Level == nil {
			return nil, fmt.Errorf("parse player level table %q: playerLevel entry missing required %q attribute", path, "level")
		}
		if e.RequiredExpToLevelUp == nil {
			return nil, fmt.Errorf("parse player level table %q: level %d missing required %q attribute", path, *e.Level, "requiredExpToLevelUp")
		}

		level := *e.Level
		karmaModifier := 0.0
		if e.KarmaModifier != nil {
			karmaModifier = *e.KarmaModifier
		}
		expLossAtDeath := 0.0
		if e.ExpLossAtDeath != nil {
			expLossAtDeath = *e.ExpLossAtDeath
		}

		levels[level] = PlayerLevel{
			RequiredExp:    *e.RequiredExpToLevelUp,
			KarmaModifier:  karmaModifier,
			ExpLossAtDeath: expLossAtDeath,
		}
		if level > maxLevel {
			maxLevel = level
		}
	}

	return &PlayerLevelTable{levels: levels, maxLevel: maxLevel}, nil
}

// Level returns the experience/penalty parameters for level, and whether an
// entry exists for it.
func (t *PlayerLevelTable) Level(level int) (PlayerLevel, bool) {
	l, ok := t.levels[level]
	return l, ok
}

// Count returns the number of levels loaded.
func (t *PlayerLevelTable) Count() int {
	return len(t.levels)
}

// MaxLevel returns the first unreachable level: a sentinel entry present
// only to define the experience span of the highest attainable level (e.g.
// 81, when the highest attainable level is 80).
func (t *PlayerLevelTable) MaxLevel() int {
	return t.maxLevel
}

// RealMaxLevel returns the highest attainable character level.
func (t *PlayerLevelTable) RealMaxLevel() int {
	return t.maxLevel - 1
}

// RequiredExpForHighestLevel returns the experience required to fill the
// experience bar at the level cap (the span between RealMaxLevel and
// MaxLevel), and whether that value is available.
func (t *PlayerLevelTable) RequiredExpForHighestLevel() (int64, bool) {
	l, ok := t.levels[t.maxLevel]
	if !ok {
		return 0, false
	}
	return l.RequiredExp, true
}
