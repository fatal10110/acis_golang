package xml

import (
	"encoding/xml"
	"fmt"
	"os"

	"github.com/fatal10110/acis_golang/internal/commons"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/player"
)

// playerLevelEntry is the wire shape of a single <playerLevel> element: its
// attributes are folded into a StatSet for player.NewLevel to consume.
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
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("load player level table %q: %w", path, err)
	}

	var doc playerLevelFile
	if err := xml.Unmarshal(data, &doc); err != nil {
		return nil, fmt.Errorf("parse player level table %q: %w", path, err)
	}

	levels := make(map[int]player.Level, len(doc.Entries))
	for _, e := range doc.Entries {
		set := commons.StatSetFromXMLAttrs(e.Attrs)

		level, err := set.GetInt("level")
		if err != nil {
			return nil, fmt.Errorf("parse player level table %q: %w", path, err)
		}
		l, err := player.NewLevel(set)
		if err != nil {
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
