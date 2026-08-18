package xml

import (
	"fmt"

	"github.com/fatal10110/acis_golang/internal/commons"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/travel"
)

type teleportFile struct {
	Lists []teleportListElement `xml:"telPosList"`
}

type teleportListElement struct {
	NPCID int            `xml:"npcId,attr"`
	Locs  []attrsElement `xml:"loc"`
}

// Instant teleports decode into their own types: a <loc> there is nothing
// but coordinates, while a gatekeeper <loc> above carries a description and
// a price that travel.Teleport reads for itself.
type instantTeleportFile struct {
	Lists []instantTeleportListElement `xml:"telPosList"`
}

type instantTeleportListElement struct {
	NPCID int               `xml:"npcId,attr"`
	Locs  []locationElement `xml:"loc"`
}

// LoadTeleports parses regular gatekeeper teleport destinations.
func LoadTeleports(path string) (travel.TeleportTable, error) {
	var doc teleportFile
	if err := readXML(path, &doc); err != nil {
		return nil, fmt.Errorf("teleports: %w", err)
	}

	table := make(travel.TeleportTable, len(doc.Lists))
	for _, list := range doc.Lists {
		if _, exists := table[list.NPCID]; exists {
			return nil, fmt.Errorf("xml: %s: duplicate teleport list for npc %d", path, list.NPCID)
		}
		teleports := make([]travel.Teleport, 0, len(list.Locs))
		for _, loc := range list.Locs {
			t, err := travel.NewTeleport(commons.StatSetFromXMLAttrs(loc.Attrs))
			if err != nil {
				return nil, fmt.Errorf("xml: %s: npc %d: %w", path, list.NPCID, err)
			}
			teleports = append(teleports, t)
		}
		table[list.NPCID] = teleports
	}
	return table, nil
}

// LoadInstantTeleports parses instant teleport destinations keyed by npc id.
func LoadInstantTeleports(path string) (travel.InstantTable, error) {
	var doc instantTeleportFile
	if err := readXML(path, &doc); err != nil {
		return nil, fmt.Errorf("instant teleports: %w", err)
	}

	table := make(travel.InstantTable, len(doc.Lists))
	for _, list := range doc.Lists {
		if _, exists := table[list.NPCID]; exists {
			return nil, fmt.Errorf("xml: %s: duplicate instant teleport list for npc %d", path, list.NPCID)
		}
		teleports := make([]location.Location, 0, len(list.Locs))
		for _, loc := range list.Locs {
			t, err := loc.loc()
			if err != nil {
				return nil, fmt.Errorf("xml: %s: npc %d: %w", path, list.NPCID, err)
			}
			teleports = append(teleports, t)
		}
		table[list.NPCID] = teleports
	}
	return table, nil
}
