package xml

import (
	"encoding/xml"
	"fmt"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/player"
)

// playerLevelEntry is the wire shape of a single <playerLevel> element.
type playerLevelEntry struct {
	Attrs []xml.Attr `xml:",any,attr"`
}

// playerLevelFile is the wire shape of the player level table's root
// element.
type playerLevelFile struct {
	XMLName xml.Name           `xml:"list"`
	Entries []playerLevelEntry `xml:"playerLevel"`
}

// LoadPlayerLevels reads and parses the player level/experience table from
// the XML file at path. It returns an error if the file cannot be read, is
// not well-formed XML, contains no entries, or an entry omits or mangles a
// required attribute.
func LoadPlayerLevels(path string) (*player.LevelTable, error) {
	var doc playerLevelFile
	if err := readXML(path, &doc); err != nil {
		return nil, fmt.Errorf("player level table: %w", err)
	}

	levels := make(map[int]player.Level, len(doc.Entries))
	for _, e := range doc.Entries {
		a := newAttrValues(foldAttrs(e.Attrs), "player level")
		level := a.int("level")
		if err := a.Err(); err != nil {
			return nil, fmt.Errorf("parse player level table %q: %w", path, err)
		}
		// requiredExpToLevelUp is required; karmaModifier and
		// expLossAtDeath default to 0 when absent (the sentinel entry above
		// the level cap carries neither).
		l := player.Level{
			RequiredExpToLevelUp: a.int64("requiredExpToLevelUp"),
			KarmaModifier:        a.float64Default("karmaModifier", 0),
			ExpLossAtDeath:       a.float64Default("expLossAtDeath", 0),
		}
		if err := a.Err(); err != nil {
			return nil, fmt.Errorf("parse player level table %q: level %d: %w", path, level, err)
		}
		levels[level] = l
	}

	table, err := player.NewLevelTable(levels)
	if err != nil {
		return nil, fmt.Errorf("parse player level table %q: %w", path, err)
	}
	return table, nil
}
