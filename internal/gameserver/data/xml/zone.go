package xml

import (
	"encoding/xml"
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/zone"
)

type zoneFile struct {
	Zones []zoneElement `xml:"zone"`
}

type zoneElement struct {
	Attrs  []xml.Attr         `xml:",any,attr"`
	Nodes  []pointElement     `xml:"node"`
	Stats  []zoneStatElement  `xml:"stat"`
	Spawns []zoneSpawnElement `xml:"spawn"`
}

// zoneSpawnElement is one <spawn> under a zone: a kind plus the coordinates
// it applies to.
type zoneSpawnElement struct {
	Type string `xml:"type,attr"`
	literalLocation
}

type zoneStatElement struct {
	Name string `xml:"name,attr"`
	Val  string `xml:"val,attr"`
}

// zoneBuilder builds one zone kind from its id, form and <stat> settings.
// It reads the settings it uses through set and leaves any parse failure in
// set's sticky error; a kind that takes no settings ignores them entirely.
type zoneBuilder func(id int, form zone.Form, set *attrValues) zone.Kind

// zoneBuilders maps a zone data file's base name onto the zone kind its
// entries build. The names are the datapack contract.
var zoneBuilders = map[string]zoneBuilder{
	"ArenaZone":           settingsFree(zone.NewArena),
	"BossZone":            buildBossZone,
	"CastleTeleportZone":  buildCastleTeleportZone,
	"CastleZone":          buildCastleZone,
	"ClanHallZone":        buildClanHallZone,
	"DamageZone":          buildDamageZone,
	"DerbyTrackZone":      settingsFree(zone.NewDerbyTrack),
	"EffectZone":          buildEffectZone,
	"FishingZone":         settingsFree(zone.NewFishing),
	"HqZone":              settingsFree(zone.NewHQ),
	"JailZone":            settingsFree(zone.NewJail),
	"MotherTreeZone":      buildMotherTreeZone,
	"NoLandingZone":       settingsFree(zone.NewNoLanding),
	"NoRestartZone":       settingsFree(zone.NewNoRestart),
	"NoStoreZone":         settingsFree(zone.NewNoStore),
	"NoSummonFriendZone":  settingsFree(zone.NewNoSummonFriend),
	"OlympiadStadiumZone": settingsFree(zone.NewOlympiad),
	"PeaceZone":           settingsFree(zone.NewPeace),
	"PrayerZone":          settingsFree(zone.NewPrayer),
	"ScriptZone":          settingsFree(zone.NewScript),
	"SiegeZone":           buildSiegeZone,
	"SwampZone":           buildSwampZone,
	"TownZone":            buildTownZone,
	"WaterZone":           settingsFree(zone.NewWater),
}

// settingsFree adapts a constructor that ignores zone settings.
func settingsFree[T zone.Kind](ctor func(id int, form zone.Form) T) zoneBuilder {
	return func(id int, form zone.Form, _ *attrValues) zone.Kind { return ctor(id, form) }
}

// Each settings-taking builder starts from the constructor's defaults and
// overrides every value the zone's <stat> children carry.

func buildMotherTreeZone(id int, form zone.Form, set *attrValues) zone.Kind {
	z := zone.NewMotherTree(id, form)
	z.EnterMessage = set.intDefault("enterMsgId", z.EnterMessage)
	z.LeaveMessage = set.intDefault("leaveMsgId", z.LeaveMessage)
	z.MPBonus = set.intDefault("MpRegenBonus", z.MPBonus)
	z.HPBonus = set.intDefault("HpRegenBonus", z.HPBonus)
	z.AffectedRace = set.intDefault("affectedRace", z.AffectedRace)
	return z
}

func buildTownZone(id int, form zone.Form, set *attrValues) zone.Kind {
	z := zone.NewTown(id, form)
	z.TownID = set.intDefault("townId", z.TownID)
	z.CastleID = set.intDefault("castleId", z.CastleID)
	z.Peaceful = set.boolDefault("isPeaceZone", z.Peaceful)
	return z
}

// buildSiegeZone links the battlefield to its castle or, when both are
// given, its clan hall.
func buildSiegeZone(id int, form zone.Form, set *attrValues) zone.Kind {
	z := zone.NewSiege(id, form)
	for _, key := range []string{"castleId", "clanHallId"} {
		if set.has(key) {
			z.ResidenceID = set.int(key)
		}
	}
	return z
}

func buildCastleZone(id int, form zone.Form, set *attrValues) zone.Kind {
	z := zone.NewCastle(id, form)
	z.ResidenceID = set.intDefault("castleId", z.ResidenceID)
	return z
}

func buildClanHallZone(id int, form zone.Form, set *attrValues) zone.Kind {
	z := zone.NewClanHall(id, form)
	z.ResidenceID = set.intDefault("clanHallId", z.ResidenceID)
	return z
}

func buildCastleTeleportZone(id int, form zone.Form, set *attrValues) zone.Kind {
	z := zone.NewCastleTeleport(id, form)
	z.CastleID = set.intDefault("castleId", z.CastleID)
	z.ExitMinX = set.intDefault("spawnMinX", z.ExitMinX)
	z.ExitMaxX = set.intDefault("spawnMaxX", z.ExitMaxX)
	z.ExitMinY = set.intDefault("spawnMinY", z.ExitMinY)
	z.ExitMaxY = set.intDefault("spawnMaxY", z.ExitMaxY)
	z.ExitZ = set.intDefault("spawnZ", z.ExitZ)
	return z
}

// readTrap overrides a trap's castle link and event trigger.
func readTrap(t *zone.Trap, set *attrValues) {
	t.CastleID = set.intDefault("castleId", t.CastleID)
	t.EventID = set.intDefault("eventId", t.EventID)
}

func buildDamageZone(id int, form zone.Form, set *attrValues) zone.Kind {
	z := zone.NewDamage(id, form)
	readTrap(&z.Trap, set)
	z.HPDamage = set.intDefault("hpDamage", z.HPDamage)
	z.InitialDelay = millisDefault(set, "initialDelay", z.InitialDelay)
	z.ReuseDelay = millisDefault(set, "reuseDelay", z.ReuseDelay)
	return z
}

func buildSwampZone(id int, form zone.Form, set *attrValues) zone.Kind {
	z := zone.NewSwamp(id, form)
	readTrap(&z.Trap, set)
	z.MoveBonus = set.intDefault("move_bonus", z.MoveBonus)
	return z
}

// buildEffectZone reads the skill list and target scope; an empty
// targetType keeps the default scope.
func buildEffectZone(id int, form zone.Form, set *attrValues) zone.Kind {
	z := zone.NewEffect(id, form)
	z.Chance = set.intDefault("chance", z.Chance)
	z.InitialDelay = millisDefault(set, "initialDelay", z.InitialDelay)
	z.ReuseDelay = millisDefault(set, "reuseDelay", z.ReuseDelay)
	z.SetEnabled(set.boolDefault("defaultStatus", z.Enabled()))
	skills, err := zone.ParseSkillRefs(set.strDefault("skill", ""))
	if err != nil {
		set.fail(err)
	}
	z.Skills = skills
	if raw := set.strDefault("targetType", ""); raw != "" {
		z.Target = attrEnum(set, "targetType", zone.ParseTargetScope)
	}
	return z
}

func buildBossZone(id int, form zone.Form, set *attrValues) zone.Kind {
	z := zone.NewBoss(id, form)
	z.InvadeWindow = millisDefault(set, "InvadeTime", z.InvadeWindow)
	z.OustLoc.X = set.intDefault("oustX", z.OustLoc.X)
	z.OustLoc.Y = set.intDefault("oustY", z.OustLoc.Y)
	z.OustLoc.Z = set.intDefault("oustZ", z.OustLoc.Z)
	return z
}

// millisDefault reads a whole-millisecond delay at key, or def when absent.
func millisDefault(set *attrValues, key string, def time.Duration) time.Duration {
	return time.Duration(set.intDefault(key, int(def/time.Millisecond))) * time.Millisecond
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

func buildZone(build zoneBuilder, el zoneElement, dynamicID *int) (zone.Kind, error) {
	attrs := newAttrValues(foldAttrs(el.Attrs), "zone")

	var id int
	if attrs.has("id") {
		id = int(attrs.int32("id"))
		if err := attrs.Err(); err != nil {
			return nil, err
		}
	} else {
		id = *dynamicID
		*dynamicID++
	}

	form, err := buildZoneForm(attrs, el.Nodes)
	if err != nil {
		return nil, fmt.Errorf("zone %d: %w", id, err)
	}

	stats := make(map[string]string, len(el.Stats))
	for _, stat := range el.Stats {
		stats[stat.Name] = stat.Val
	}
	set := newAttrValues(stats, "settings")
	k := build(id, form, set)
	if err := set.Err(); err != nil {
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

// buildZoneForm builds the zone's shape. minZ, maxZ and a cylinder's rad
// are integer literals.
func buildZoneForm(attrs *attrValues, nodeEls []pointElement) (zone.Form, error) {
	shape := attrs.str("shape")
	minZ := int(attrs.int32Literal("minZ"))
	maxZ := int(attrs.int32Literal("maxZ"))
	if err := attrs.Err(); err != nil {
		return nil, err
	}

	nodes := make([]location.Point, 0, len(nodeEls))
	for _, el := range nodeEls {
		point, err := el.point()
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
		return zone.NewCuboid(nodes[0].X, nodes[1].X, nodes[0].Y, nodes[1].Y, minZ, maxZ)
	case "NPoly":
		if len(nodes) <= 2 {
			return nil, fmt.Errorf("polygon zone wants more than 2 nodes, has %d", len(nodes))
		}
		return zone.NewPolygon(nodes, minZ, maxZ)
	case "Cylinder":
		if len(nodes) != 1 {
			return nil, fmt.Errorf("cylinder zone wants 1 node, has %d", len(nodes))
		}
		rad := int(attrs.int32Literal("rad"))
		if err := attrs.Err(); err != nil {
			return nil, err
		}
		return zone.NewCylinder(nodes[0].X, nodes[0].Y, minZ, maxZ, rad)
	default:
		return nil, fmt.Errorf("unknown zone shape %q", shape)
	}
}

func addZoneSpawn(site zone.SpawnSite, el zoneSpawnElement) error {
	kind, err := zone.ParseSpawnKind(el.Type)
	if err != nil {
		return err
	}
	loc, err := el.loc()
	if err != nil {
		return err
	}
	site.AddSpawn(kind, loc)
	return nil
}
