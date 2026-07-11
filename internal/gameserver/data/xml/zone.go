package xml

import (
	"encoding/xml"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/fatal10110/acis_golang/internal/commons"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/zone"
)

type zoneFile struct {
	Zones []zoneElement `xml:"zone"`
}

type zoneElement struct {
	Attrs  []xml.Attr        `xml:",any,attr"`
	Nodes  []attrsElement    `xml:"node"`
	Stats  []zoneStatElement `xml:"stat"`
	Spawns []attrsElement    `xml:"spawn"`
}

type zoneStatElement struct {
	Name string `xml:"name,attr"`
	Val  string `xml:"val,attr"`
}

// zoneBuilders maps a zone data file's base name onto the zone kind its
// entries build. The names are the datapack contract.
var zoneBuilders = map[string]func(id int, form zone.Form, set *commons.StatSet) (zone.Kind, error){
	"ArenaZone":           settingsFree(zone.NewArena),
	"BossZone":            wrap(zone.NewBoss),
	"CastleTeleportZone":  wrap(zone.NewCastleTeleport),
	"CastleZone":          wrap(zone.NewCastle),
	"ClanHallZone":        wrap(zone.NewClanHall),
	"DamageZone":          wrap(zone.NewDamage),
	"DerbyTrackZone":      settingsFree(zone.NewDerbyTrack),
	"EffectZone":          wrap(zone.NewEffect),
	"FishingZone":         settingsFree(zone.NewFishing),
	"HqZone":              settingsFree(zone.NewHQ),
	"JailZone":            settingsFree(zone.NewJail),
	"MotherTreeZone":      wrap(zone.NewMotherTree),
	"NoLandingZone":       settingsFree(zone.NewNoLanding),
	"NoRestartZone":       settingsFree(zone.NewNoRestart),
	"NoStoreZone":         settingsFree(zone.NewNoStore),
	"NoSummonFriendZone":  settingsFree(zone.NewNoSummonFriend),
	"OlympiadStadiumZone": settingsFree(zone.NewOlympiad),
	"PeaceZone":           settingsFree(zone.NewPeace),
	"PrayerZone":          settingsFree(zone.NewPrayer),
	"ScriptZone":          settingsFree(zone.NewScript),
	"SiegeZone":           wrap(zone.NewSiege),
	"SwampZone":           wrap(zone.NewSwamp),
	"TownZone":            wrap(zone.NewTown),
	"WaterZone":           settingsFree(zone.NewWater),
}

// settingsFree adapts a constructor that ignores zone settings.
func settingsFree[T zone.Kind](ctor func(id int, form zone.Form) T) func(int, zone.Form, *commons.StatSet) (zone.Kind, error) {
	return func(id int, form zone.Form, _ *commons.StatSet) (zone.Kind, error) {
		return ctor(id, form), nil
	}
}

// wrap adapts a settings-taking constructor to the builder shape.
func wrap[T zone.Kind](ctor func(id int, form zone.Form, set *commons.StatSet) (T, error)) func(int, zone.Form, *commons.StatSet) (zone.Kind, error) {
	return func(id int, form zone.Form, set *commons.StatSet) (zone.Kind, error) {
		return ctor(id, form, set)
	}
}

// LoadZones parses every zone data file in dir into a region-attached zone
// index. Files are read in sorted name order; zones without an explicit id
// get sequential ids from a per-file counter that jumps to the next
// multiple of 1000 for each file.
func LoadZones(dir string) (*zone.Index, error) {
	docs, err := loadXMLDocuments[zoneFile](dir, "zones")
	if err != nil {
		return nil, err
	}

	index := zone.NewIndex()
	dynamicID := 0
	for _, doc := range docs {
		dynamicID = dynamicID/1000*1000 + 1000

		name := strings.TrimSuffix(filepath.Base(doc.Path), ".xml")
		build, ok := zoneBuilders[name]
		if !ok {
			return nil, fmt.Errorf("xml: %s: unknown zone kind %q", doc.Path, name)
		}

		for i, el := range doc.Data.Zones {
			k, err := buildZone(build, el, &dynamicID)
			if err != nil {
				return nil, fmt.Errorf("xml: %s: zone %d: %w", doc.Path, i, err)
			}
			index.Add(k)
		}
	}
	return index, nil
}

func buildZone(build func(int, zone.Form, *commons.StatSet) (zone.Kind, error), el zoneElement, dynamicID *int) (zone.Kind, error) {
	attrs := commons.StatSetFromXMLAttrs(el.Attrs)

	var id int
	if attrs.Has("id") {
		v, err := attrs.GetInt("id")
		if err != nil {
			return nil, err
		}
		id = v
	} else {
		id = *dynamicID
		*dynamicID++
	}

	form, err := buildZoneForm(attrs, el.Nodes)
	if err != nil {
		return nil, fmt.Errorf("zone %d: %w", id, err)
	}

	set := commons.NewStatSetWithCapacity(len(el.Stats))
	for _, stat := range el.Stats {
		set.Set(stat.Name, stat.Val)
	}

	k, err := build(id, form, set)
	if err != nil {
		return nil, fmt.Errorf("zone %d: %w", id, err)
	}

	// Spawn points only apply to kinds that carry them; other kinds'
	// files may hold stale spawn elements, which the contract ignores.
	if site, ok := k.(zone.SpawnSite); ok {
		for _, sp := range el.Spawns {
			if err := addZoneSpawn(site, sp); err != nil {
				return nil, fmt.Errorf("zone %d: %w", id, err)
			}
		}
	}
	return k, nil
}

func buildZoneForm(attrs *commons.StatSet, nodeEls []attrsElement) (zone.Form, error) {
	f := commons.NewFields(attrs, "zone form")
	shape := f.String("shape")
	minZ := f.Int("minZ")
	maxZ := f.Int("maxZ")
	if err := f.Err(); err != nil {
		return nil, err
	}

	nodes := make([]location.Point, 0, len(nodeEls))
	for _, el := range nodeEls {
		point, err := location.NewPoint(commons.StatSetFromXMLAttrs(el.Attrs))
		if err != nil {
			return nil, err
		}
		nodes = append(nodes, point)
	}
	if len(nodes) == 0 {
		return nil, fmt.Errorf("zone has no nodes")
	}

	switch shape {
	case "Cuboid":
		if len(nodes) != 2 {
			return nil, fmt.Errorf("cuboid zone wants 2 nodes, has %d", len(nodes))
		}
		return zone.NewCuboid(nodes[0].X, nodes[1].X, nodes[0].Y, nodes[1].Y, minZ, maxZ), nil
	case "NPoly":
		if len(nodes) <= 2 {
			return nil, fmt.Errorf("polygon zone wants more than 2 nodes, has %d", len(nodes))
		}
		return zone.NewPolygon(nodes, minZ, maxZ)
	case "Cylinder":
		if len(nodes) != 1 {
			return nil, fmt.Errorf("cylinder zone wants 1 node, has %d", len(nodes))
		}
		rad := f.Int("rad")
		if err := f.Err(); err != nil {
			return nil, err
		}
		return zone.NewCylinder(nodes[0].X, nodes[0].Y, minZ, maxZ, rad)
	default:
		return nil, fmt.Errorf("unknown zone shape %q", shape)
	}
}

func addZoneSpawn(site zone.SpawnSite, el attrsElement) error {
	set := commons.StatSetFromXMLAttrs(el.Attrs)
	kindName, err := set.GetString("type")
	if err != nil {
		return err
	}
	kind, err := zone.ParseSpawnKind(kindName)
	if err != nil {
		return err
	}
	loc, err := location.NewLocation(set)
	if err != nil {
		return err
	}
	site.AddSpawn(kind, loc)
	return nil
}
