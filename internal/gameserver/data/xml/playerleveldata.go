package xml

import (
	"encoding/xml"
	"fmt"
	"os"

	"github.com/fatal10110/acis_golang/internal/commons"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/records"
)

// PlayerLevelData holds every level's experience/penalty parameters, keyed
// by level number, as loaded from the player level table
// (PlayerLevelData.java).
type PlayerLevelData struct {
	levels   map[int]records.PlayerLevel
	maxLevel int
}

// playerLevelEntry is the wire shape of a single <playerLevel> element: its
// attributes are folded into a StatSet, which records.NewPlayerLevel
// consumes the same way its Java counterpart consumes parseAttributes'
// result.
type playerLevelEntry struct {
	Attrs []xml.Attr `xml:",any,attr"`
}

// playerLevelDocument is the wire shape of the player level table's root
// element.
type playerLevelDocument struct {
	XMLName xml.Name           `xml:"list"`
	Entries []playerLevelEntry `xml:"playerLevel"`
}

// LoadPlayerLevelData reads and parses the player level/experience table
// from the XML file at path. It returns an error if the file cannot be
// read, is not well-formed XML, contains no entries, or an entry omits or
// mangles a required attribute.
func LoadPlayerLevelData(path string) (*PlayerLevelData, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("load player level table %q: %w", path, err)
	}

	var doc playerLevelDocument
	if err := xml.Unmarshal(data, &doc); err != nil {
		return nil, fmt.Errorf("parse player level table %q: %w", path, err)
	}

	levels := make(map[int]records.PlayerLevel, len(doc.Entries))
	maxLevel := 0
	for _, e := range doc.Entries {
		set := commons.StatSetFromXMLAttrs(e.Attrs)

		level, err := set.GetInt("level")
		if err != nil {
			return nil, fmt.Errorf("parse player level table %q: %w", path, err)
		}
		pl, err := records.NewPlayerLevel(set)
		if err != nil {
			return nil, fmt.Errorf("parse player level table %q: level %d: %w", path, level, err)
		}

		levels[level] = pl
		if level > maxLevel {
			maxLevel = level
		}
	}
	if len(levels) == 0 {
		return nil, fmt.Errorf("parse player level table %q: no playerLevel entries", path)
	}

	return &PlayerLevelData{levels: levels, maxLevel: maxLevel}, nil
}

// Level returns the experience/penalty parameters for level, and whether an
// entry exists for it.
func (t *PlayerLevelData) Level(level int) (records.PlayerLevel, bool) {
	l, ok := t.levels[level]
	return l, ok
}

// Count returns the number of levels loaded.
func (t *PlayerLevelData) Count() int {
	return len(t.levels)
}

// MaxLevel returns the first unreachable level: a sentinel entry present
// only to define the experience span of the highest attainable level (e.g.
// 81, when the highest attainable level is 80).
func (t *PlayerLevelData) MaxLevel() int {
	return t.maxLevel
}

// RealMaxLevel returns the highest attainable character level.
func (t *PlayerLevelData) RealMaxLevel() int {
	return t.maxLevel - 1
}

// RequiredExpForHighestLevel returns the experience required to fill the
// experience bar at the level cap (the span between RealMaxLevel and
// MaxLevel). The entry always exists: maxLevel is the largest loaded key
// and loading rejects an empty table.
func (t *PlayerLevelData) RequiredExpForHighestLevel() int64 {
	return t.levels[t.maxLevel].RequiredExpToLevelUp
}
