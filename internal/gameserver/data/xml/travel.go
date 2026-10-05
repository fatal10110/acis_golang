package xml

import (
	"fmt"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/travel"
)

type teleportFile struct {
	Lists []teleportListElement `xml:"telPosList"`
}

type teleportListElement struct {
	NPCID *coord32       `xml:"npcId,attr"`
	Locs  []attrsElement `xml:"loc"`
}

// Instant teleports decode into their own types: a <loc> there is nothing
// but coordinates, while a gatekeeper <loc> above carries a description and
// a price that travel.Teleport reads for itself.
type instantTeleportFile struct {
	Lists []instantTeleportListElement `xml:"telPosList"`
}

type instantTeleportListElement struct {
	NPCID *coord32          `xml:"npcId,attr"`
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
		if list.NPCID == nil {
			return nil, fmt.Errorf("xml: %s: telPosList: npcId is required", path)
		}
		npcID := int(*list.NPCID)
		teleports := make([]travel.Teleport, 0, len(list.Locs))
		for _, loc := range list.Locs {
			t, err := buildTeleport(newAttrValues(foldAttrs(loc.Attrs), ""))
			if err != nil {
				return nil, fmt.Errorf("xml: %s: npc %d: %w", path, npcID, err)
			}
			teleports = append(teleports, t)
		}
		table[npcID] = teleports
	}
	return table, nil
}

// buildTeleport builds one gatekeeper <loc>. desc, priceId, priceCount, x,
// y and z are required; type defaults to STANDARD and castleId to 0.
func buildTeleport(a *attrValues) (travel.Teleport, error) {
	a.prefix = "travel: teleport"
	desc := a.str("desc")
	if err := a.Err(); err != nil {
		return travel.Teleport{}, err
	}
	a.prefix = fmt.Sprintf("travel: teleport %q", desc)
	t := travel.Teleport{
		Location:    location.Location{X: a.int("x"), Y: a.int("y"), Z: a.int("z")},
		Description: desc,
		Kind:        attrEnumDefault(a, "type", travel.ParseKind, travel.KindStandard),
		PriceID:     a.int("priceId"),
		PriceCount:  a.int("priceCount"),
		CastleID:    a.intDefault("castleId", 0),
	}
	if err := a.Err(); err != nil {
		return travel.Teleport{}, err
	}
	return t, nil
}

// LoadInstantTeleports parses instant teleport destinations keyed by npc id.
func LoadInstantTeleports(path string) (travel.InstantTable, error) {
	var doc instantTeleportFile
	if err := readXML(path, &doc); err != nil {
		return nil, fmt.Errorf("instant teleports: %w", err)
	}

	table := make(travel.InstantTable, len(doc.Lists))
	for _, list := range doc.Lists {
		if list.NPCID == nil {
			return nil, fmt.Errorf("xml: %s: telPosList: npcId is required", path)
		}
		npcID := int(*list.NPCID)
		teleports := make([]location.Location, 0, len(list.Locs))
		for _, loc := range list.Locs {
			t, err := loc.loc()
			if err != nil {
				return nil, fmt.Errorf("xml: %s: npc %d: %w", path, npcID, err)
			}
			teleports = append(teleports, t)
		}
		table[npcID] = teleports
	}
	return table, nil
}
