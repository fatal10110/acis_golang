package xml

import (
	"bytes"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"math"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"testing"

	"github.com/fatal10110/acis_golang/internal/commons"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/npc"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/residence"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/residence/castle"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/zone"
	"github.com/rs/zerolog"
)

// literalValues is the accepted-input table for an integer attribute, per
// grammar. "lit" is the reference's Integer.decode result and "dec" its
// Integer.parseInt result for the same input, both taken from a Java probe
// (OpenJDK 21.0.11); "ERR" means the reference threw. The rows are chosen
// so the two grammars disagree in both directions ("010", "08", "0x10"),
// which is what makes each field's case non-vacuous: a field wired to the
// wrong grammar fails at least three rows.
var literalValues = []struct {
	in, lit, dec string
	signed       bool // only run for fields where a negative value is legal
}{
	{in: "12", lit: "12", dec: "12"},
	{in: "+12", lit: "12", dec: "12"},
	{in: "010", lit: "8", dec: "10"},
	{in: "0x10", lit: "16", dec: "ERR"},
	{in: "0X10", lit: "16", dec: "ERR"},
	{in: "#10", lit: "16", dec: "ERR"},
	{in: "-0x10", lit: "-16", dec: "ERR", signed: true},
	{in: "08", lit: "ERR", dec: "8"},
	{in: "0b1", lit: "ERR", dec: "ERR"},
	{in: "0o10", lit: "ERR", dec: "ERR"},
	{in: "1_0", lit: "ERR", dec: "ERR"},
	{in: "0x+1", lit: "ERR", dec: "ERR"},
	{in: "", lit: "ERR", dec: "ERR"},
	{in: " 10 ", lit: "ERR", dec: "ERR"},
	{in: "2147483648", lit: "ERR", dec: "ERR"},
	{in: "0x80000000", lit: "ERR", dec: "ERR"},
}

// probeInt returns the first candidate value for which found reports a hit.
// A loaded table is often keyed by the value under test, so the probe set
// is every value literalValues can produce.
func probeInt(found func(int) bool) (int, error) {
	for _, v := range []int{12, 8, 10, 16, -16} {
		if found(v) {
			return v, nil
		}
	}
	return 0, fmt.Errorf("no entry loaded under any expected value")
}

// noValue marks a field whose loaded value is not cheaply observable; its
// rows still pin acceptance and rejection, which already distinguish the
// two grammars.
const noValue = -1 << 40

type literalField struct {
	name    string
	attr    string
	file    string // fixture file name inside a fresh dir; default fixture.xml
	doc     string // one %s where the attribute under test is written
	decimal bool   // the reference reads this one with Integer.parseInt
	signed  bool
	load    func(dir, path string) (int, error)
}

// TestIntAttrGrammarMatchesReferencePerField drives every integer attribute
// whose reference read is Integer.decode (and, as controls, the ones it
// reads with Integer.parseInt) through its production loader with each
// literalValues row, and requires the loaded value or the rejection the
// reference produces. A rejection must name the attribute and the file.
func TestIntAttrGrammarMatchesReferencePerField(t *testing.T) {
	t.Parallel()

	const square = `<node x="0" y="0"/><node x="100" y="0"/><node x="100" y="100"/><node x="0" y="100"/>`
	const boatNode = `<node x="1" y="2" z="3"/>`
	const doorTail = `<stats hp="1" pDef="1" mDef="1" height="1"/></door></list>`
	const castleHead = `<list><castle id="1" alias="gludio" parentId="0" name="Gludio" circletId="1"><tax taxRate="0" taxSysgetRate="0" tributeRate="0"/>`
	const npcHead = `<list><npc id="1" name="x">` + npcRequiredSets
	const territory = `<territory name="t" minZ="-10" maxZ="10"><node x="0" y="0"/><node x="100" y="0"/><node x="100" y="100"/></territory>`
	const maker = `<npcmaker name="m" territory="t" maximumNpcs="1"><npc id="1" total="1" respawn="1min"/></npcmaker>`
	probeSkills := skillTableWith(
		skill.Ref{ID: 1, Level: 1}, skill.Ref{ID: 1, Level: 8}, skill.Ref{ID: 1, Level: 12}, skill.Ref{ID: 1, Level: 16},
		skill.Ref{ID: 8, Level: 1}, skill.Ref{ID: 12, Level: 1}, skill.Ref{ID: 16, Level: 1},
	)
	probeItems := itemTableWithIDs([]int32{1, 8, 12, 16})

	manorArea := func(dir, path string) (int, error) {
		_, err := LoadManorAreas(path)
		return noValue, err
	}
	loadCastle := func(dir, path string) (*castleView, error) {
		table, err := LoadCastles(path)
		if err != nil {
			return nil, err
		}
		c, _ := table.Get(1)
		return &castleView{c.Artifacts[0].NPCID, c}, nil
	}
	npcByName := func(dir, path string) (*npcView, error) {
		table, err := LoadNPCTemplates(dir, probeItems, probeSkills, zerolog.Nop())
		if err != nil {
			return nil, err
		}
		tpl, ok := table.GetByName("x")
		if !ok {
			return nil, fmt.Errorf("npc x not loaded")
		}
		return &npcView{tpl.ID, tpl.TemplateID, tpl}, nil
	}

	fields := []literalField{
		// manors.xml, manorAreas.xml
		{name: "manor id", attr: "id", doc: `<list><manor %s name="a"/></list>`, load: func(dir, path string) (int, error) {
			table, err := LoadManors(path)
			if err != nil {
				return 0, err
			}
			return table.Manors[0].ID, nil
		}},
		{name: "manor area castleId", attr: "castleId", doc: `<list><area name="a" %s minZ="1" maxZ="2">` + square + `</area></list>`, load: func(dir, path string) (int, error) {
			areas, err := LoadManorAreas(path)
			if err != nil {
				return 0, err
			}
			return areas[0].CastleID, nil
		}},
		{name: "manor area minZ", attr: "minZ", signed: true, doc: `<list><area name="a" castleId="1" %s maxZ="200">` + square + `</area></list>`, load: func(dir, path string) (int, error) {
			areas, err := LoadManorAreas(path)
			if err != nil {
				return 0, err
			}
			return areas[0].MinZ, nil
		}},
		{name: "manor area maxZ", attr: "maxZ", signed: true, doc: `<list><area name="a" castleId="1" minZ="-200" %s>` + square + `</area></list>`, load: func(dir, path string) (int, error) {
			areas, err := LoadManorAreas(path)
			if err != nil {
				return 0, err
			}
			return areas[0].MaxZ, nil
		}},
		{name: "manor area node x", attr: "x", signed: true, doc: `<list><area name="a" castleId="1" minZ="1" maxZ="2"><node %s y="0"/><node x="100" y="0"/><node x="100" y="100"/></area></list>`, load: func(dir, path string) (int, error) {
			areas, err := LoadManorAreas(path)
			if err != nil {
				return 0, err
			}
			return areas[0].Nodes[0].X, nil
		}},
		{name: "manor area node y", attr: "y", signed: true, doc: `<list><area name="a" castleId="1" minZ="1" maxZ="2"><node x="0" %s/><node x="100" y="0"/><node x="100" y="100"/></area></list>`, load: manorArea},

		// restartPointAreas.xml
		{name: "restart area minZ", attr: "minZ", signed: true, doc: `<list><area %s maxZ="100">` + square + `</area></list>`, load: func(dir, path string) (int, error) {
			table, err := LoadRestartPoints(path)
			if err != nil {
				return 0, err
			}
			lo, _ := restartZBand(table.Areas[0])
			return lo, nil
		}},
		{name: "restart area maxZ", attr: "maxZ", signed: true, doc: `<list><area minZ="-100" %s>` + square + `</area></list>`, load: func(dir, path string) (int, error) {
			table, err := LoadRestartPoints(path)
			if err != nil {
				return 0, err
			}
			_, hi := restartZBand(table.Areas[0])
			return hi, nil
		}},
		{name: "restart area node x", attr: "x", signed: true, doc: `<list><area minZ="-100" maxZ="100"><node %s y="0"/><node x="100" y="0"/><node x="100" y="100"/></area></list>`, load: func(dir, path string) (int, error) {
			_, err := LoadRestartPoints(path)
			return noValue, err
		}},

		// boatRoutes.xml
		{name: "boat itinerary item1", attr: "item1", doc: `<list><itinerary dock1="GIRAN" dock2="TALKING_ISLAND" %s item2="2" heading="1"><route>` + boatNode + `</route><route>` + boatNode + `</route></itinerary></list>`, load: func(dir, path string) (int, error) {
			itineraries, err := LoadBoatRoutes(path)
			if err != nil {
				return 0, err
			}
			return itineraries[0].Routes[0].ItemID, nil
		}},
		{name: "boat itinerary item2", attr: "item2", doc: `<list><itinerary dock1="GIRAN" dock2="TALKING_ISLAND" item1="1" %s heading="1"><route>` + boatNode + `</route><route>` + boatNode + `</route></itinerary></list>`, load: func(dir, path string) (int, error) {
			itineraries, err := LoadBoatRoutes(path)
			if err != nil {
				return 0, err
			}
			return itineraries[0].Routes[1].ItemID, nil
		}},
		{name: "boat itinerary heading", attr: "heading", signed: true, doc: `<list><itinerary dock1="GIRAN" %s><route>` + boatNode + `</route></itinerary></list>`, load: func(dir, path string) (int, error) {
			itineraries, err := LoadBoatRoutes(path)
			if err != nil {
				return 0, err
			}
			return itineraries[0].Heading, nil
		}},

		// summonItems.xml, spellbooks.xml, bufferSkills.xml
		{name: "summon item id", attr: "id", doc: `<list><item %s npcId="1" summonType="1"/></list>`, load: func(dir, path string) (int, error) {
			table, err := LoadSummonItems(path)
			if err != nil {
				return 0, err
			}
			return probeInt(func(v int) bool { _, ok := table.Item(int32(v)); return ok })
		}},
		{name: "summon item npcId", attr: "npcId", doc: `<list><item id="1" %s summonType="1"/></list>`, load: func(dir, path string) (int, error) {
			table, err := LoadSummonItems(path)
			if err != nil {
				return 0, err
			}
			it, _ := table.Item(1)
			return int(it.NPCID), nil
		}},
		{name: "summon item summonType", attr: "summonType", doc: `<list><item id="1" npcId="1" %s/></list>`, load: func(dir, path string) (int, error) {
			table, err := LoadSummonItems(path)
			if err != nil {
				return 0, err
			}
			it, _ := table.Item(1)
			return it.SummonType, nil
		}},
		{name: "spellbook skillId", attr: "skillId", doc: `<list><book %s itemId="1"/></list>`, load: func(dir, path string) (int, error) {
			table, err := LoadSpellbooks(path)
			if err != nil {
				return 0, err
			}
			return probeInt(func(v int) bool { return table.BookForSkill(skill.ID(v), 1, true, false) == 1 })
		}},
		{name: "spellbook itemId", attr: "itemId", doc: `<list><book skillId="1" %s/></list>`, load: func(dir, path string) (int, error) {
			table, err := LoadSpellbooks(path)
			if err != nil {
				return 0, err
			}
			return int(table.BookForSkill(1, 1, true, false)), nil
		}},
		{name: "buffer skill id", attr: "id", doc: `<list><category type="a"><buff %s level="1"/></category></list>`, load: func(dir, path string) (int, error) {
			table, err := LoadBufferSkills(path, probeSkills)
			if err != nil {
				return 0, err
			}
			return probeInt(func(v int) bool { _, ok := table.Skill(int32(v)); return ok })
		}},
		{name: "buffer skill level", attr: "level", doc: `<list><category type="a"><buff id="1" %s/></category></list>`, load: func(dir, path string) (int, error) {
			table, err := LoadBufferSkills(path, probeSkills)
			if err != nil {
				return 0, err
			}
			b, _ := table.Skill(1)
			return b.Skill.Level, nil
		}},
		{name: "buffer skill price", attr: "price", doc: `<list><category type="a"><buff id="1" level="1" %s/></category></list>`, load: func(dir, path string) (int, error) {
			table, err := LoadBufferSkills(path, probeSkills)
			if err != nil {
				return 0, err
			}
			b, _ := table.Skill(1)
			return b.Price, nil
		}},

		// augmentation/*.xml: the stat group needs the full augmentation
		// set to validate, so the element is decoded on its own.
		{name: "augmentation set order", attr: "order", doc: `<list><set %s/></list>`, load: func(dir, path string) (int, error) {
			var doc augmentationFile
			if err := readXML(path, &doc); err != nil {
				return 0, err
			}
			if doc.Sets[0].Order == nil {
				return 0, fmt.Errorf("order is required")
			}
			return int(*doc.Sets[0].Order), nil
		}},

		// castles.xml
		{name: "castle artifact id", attr: "id", doc: castleHead + `<artifacts><artifact %s pos="1;2;3;4"/></artifacts></castle></list>`, load: func(dir, path string) (int, error) {
			v, err := loadCastle(dir, path)
			if err != nil {
				return 0, err
			}
			return v.artifactNPCID, nil
		}},
		{name: "castle control tower x", attr: "x", signed: true, doc: castleHead + `<artifacts><artifact id="1" pos="1;2;3;4"/></artifacts><controlTowers><controlTower alias="t" type="LIFE_CONTROL"><position %s y="2" z="3"/><stats hp="1" pDef="1" mDef="1"/></controlTower></controlTowers></castle></list>`, load: func(dir, path string) (int, error) {
			v, err := loadCastle(dir, path)
			if err != nil {
				return 0, err
			}
			return v.castle.ControlTowers[0].Position.X, nil
		}},
		{name: "castle spawn z", attr: "z", signed: true, doc: castleHead + `<artifacts><artifact id="1" pos="1;2;3;4"/></artifacts><spawns><spawn type="OWNER" x="1" y="2" %s/></spawns></castle></list>`, load: func(dir, path string) (int, error) {
			v, err := loadCastle(dir, path)
			if err != nil {
				return 0, err
			}
			return v.castle.Spawns[residence.SpawnOwner][0].Z, nil
		}},
		{name: "clan hall spawn x", attr: "x", signed: true, doc: `<list><clanHall id="21" alias="a" parentId="0" name="a"><agit desc="a" loc="Town"/><tax taxRate="0" taxSysgetRate="0" tributeRate="0"/><spawns><spawn type="OWNER" %s y="2" z="3"/></spawns></clanHall></list>`, load: func(dir, path string) (int, error) {
			table, err := LoadClanHalls(path)
			if err != nil {
				return 0, err
			}
			ch, _ := table.Get(21)
			return ch.Spawns[residence.SpawnOwner][0].X, nil
		}},

		// observerGroups.xml: the spawn reads are literals, the group id a
		// plain decimal (the decimal control).
		{name: "observer spawn id", attr: "id", doc: `<list><spawns><spawn %s x="1" y="2" z="3" groups="1"/></spawns></list>`, load: func(dir, path string) (int, error) {
			table, err := LoadObserverGroups(path)
			if err != nil {
				return 0, err
			}
			return table.Spawns()[0].NPCID, nil
		}},
		{name: "observer spawn y", attr: "y", signed: true, doc: `<list><spawns><spawn id="1" x="1" %s z="3" groups="1"/></spawns></list>`, load: func(dir, path string) (int, error) {
			table, err := LoadObserverGroups(path)
			if err != nil {
				return 0, err
			}
			return table.Spawns()[0].Location.Y, nil
		}},
		{name: "observer group id", attr: "id", decimal: true, signed: true, doc: `<list><groups><group %s><entry locId="1" x="1" y="2" z="3" yaw="0" pitch="0" cost="0" castle="0"/></group></groups></list>`, load: func(dir, path string) (int, error) {
			table, err := LoadObserverGroups(path)
			if err != nil {
				return 0, err
			}
			return probeInt(func(v int) bool { _, ok := table.Group(v); return ok })
		}},

		// teleports.xml, instantTeleports.xml: decimal controls.
		{name: "teleport list npcId", attr: "npcId", decimal: true, doc: `<list><telPosList %s><loc desc="a" priceId="57" priceCount="1" x="1" y="2" z="3"/></telPosList></list>`, load: func(dir, path string) (int, error) {
			table, err := LoadTeleports(path)
			if err != nil {
				return 0, err
			}
			return probeInt(func(v int) bool { _, ok := table[v]; return ok })
		}},
		{name: "instant teleport list npcId", attr: "npcId", decimal: true, doc: `<list><telPosList %s><loc x="1" y="2" z="3"/></telPosList></list>`, load: func(dir, path string) (int, error) {
			table, err := LoadInstantTeleports(path)
			if err != nil {
				return 0, err
			}
			return probeInt(func(v int) bool { _, ok := table[v]; return ok })
		}},

		// announcements.xml: an automatic announcement's schedule.
		{name: "announcement initial_delay", attr: "initial_delay", doc: `<list><announcement message="m" auto="true" %s delay="1" limit="1"/></list>`, load: func(dir, path string) (int, error) {
			a, err := LoadAnnouncements(path)
			if err != nil {
				return 0, err
			}
			return a[0].InitialDelay, nil
		}},
		{name: "announcement delay", attr: "delay", doc: `<list><announcement message="m" auto="true" initial_delay="1" %s limit="1"/></list>`, load: func(dir, path string) (int, error) {
			a, err := LoadAnnouncements(path)
			if err != nil {
				return 0, err
			}
			return a[0].Delay, nil
		}},
		{name: "announcement limit", attr: "limit", doc: `<list><announcement message="m" auto="true" initial_delay="1" delay="1" %s/></list>`, load: func(dir, path string) (int, error) {
			a, err := LoadAnnouncements(path)
			if err != nil {
				return 0, err
			}
			return a[0].Limit, nil
		}},

		// buyLists.xml
		{name: "buylist id", attr: "id", doc: `<list><buyList %s npcId="1"/></list>`, load: func(dir, path string) (int, error) {
			table, err := LoadBuyLists(path, nil)
			if err != nil {
				return 0, err
			}
			return probeInt(func(v int) bool { _, ok := table.Find(v); return ok })
		}},
		{name: "buylist npcId", attr: "npcId", doc: `<list><buyList id="1" %s/></list>`, load: func(dir, path string) (int, error) {
			table, err := LoadBuyLists(path, nil)
			if err != nil {
				return 0, err
			}
			l, _ := table.Find(1)
			return l.NPCID, nil
		}},

		// zones/*.xml: the zone id is the decimal control.
		{name: "zone id", attr: "id", file: "PeaceZone.xml", decimal: true, doc: `<list><zone %s shape="Cuboid" minZ="0" maxZ="10"><node x="0" y="0"/><node x="5" y="5"/></zone></list>`, load: func(dir, path string) (int, error) {
			index, err := LoadZones(dir)
			if err != nil {
				return 0, err
			}
			return probeInt(func(v int) bool { _, ok := index.ByID(v); return ok })
		}},
		{name: "zone minZ", attr: "minZ", file: "PeaceZone.xml", signed: true, doc: `<list><zone shape="Cuboid" %s maxZ="100"><node x="0" y="0"/><node x="5" y="5"/></zone></list>`, load: func(dir, path string) (int, error) {
			_, err := LoadZones(dir)
			return noValue, err
		}},
		{name: "zone maxZ", attr: "maxZ", file: "PeaceZone.xml", signed: true, doc: `<list><zone shape="Cuboid" minZ="-100" %s><node x="0" y="0"/><node x="5" y="5"/></zone></list>`, load: func(dir, path string) (int, error) {
			_, err := LoadZones(dir)
			return noValue, err
		}},
		{name: "zone rad", attr: "rad", file: "PeaceZone.xml", doc: `<list><zone shape="Cylinder" minZ="0" maxZ="10" %s><node x="0" y="0"/></zone></list>`, load: func(dir, path string) (int, error) {
			_, err := LoadZones(dir)
			return noValue, err
		}},
		{name: "zone node x", attr: "x", file: "PeaceZone.xml", signed: true, doc: `<list><zone shape="Cuboid" minZ="0" maxZ="10"><node %s y="0"/><node x="50" y="5"/></zone></list>`, load: func(dir, path string) (int, error) {
			_, err := LoadZones(dir)
			return noValue, err
		}},
		{name: "zone spawn x", attr: "x", file: "OlympiadStadiumZone.xml", signed: true, doc: `<list><zone shape="Cuboid" minZ="0" maxZ="10"><node x="0" y="0"/><node x="5" y="5"/><spawn type="NORMAL" %s y="1" z="1"/></zone></list>`, load: func(dir, path string) (int, error) {
			index, err := LoadZones(dir)
			if err != nil {
				return 0, err
			}
			return zone.OfKind[*zone.Olympiad](index)[0].Spawn(zone.SpawnNormal)[0].X, nil
		}},

		// doors.xml
		{name: "door position x", attr: "x", signed: true, doc: `<list><door id="1" type="DOOR" level="1" name="d"><position %s y="2" z="3"/><coordinates><loc x="1" y="2"/><loc x="1" y="3"/><loc x="2" y="3"/></coordinates>` + doorTail, load: func(dir, path string) (int, error) {
			table, err := LoadDoors(path, zerolog.Nop())
			if err != nil {
				return 0, err
			}
			d, _ := table.Get(1)
			return d.Position.X, nil
		}},
		{name: "door coordinate x", attr: "x", signed: true, doc: `<list><door id="1" type="DOOR" level="1" name="d"><position x="1" y="2" z="3"/><coordinates><loc %s y="2"/><loc x="1" y="3"/><loc x="2" y="3"/></coordinates>` + doorTail, load: func(dir, path string) (int, error) {
			table, err := LoadDoors(path, zerolog.Nop())
			if err != nil {
				return 0, err
			}
			d, _ := table.Get(1)
			return d.Coordinates[0].X, nil
		}},

		// spawnlist/*.xml
		{name: "territory minZ", attr: "minZ", signed: true, doc: `<list><territory name="t" %s maxZ="100"><node x="0" y="0"/><node x="100" y="0"/><node x="100" y="100"/></territory>` + maker + `</list>`, load: func(dir, path string) (int, error) {
			table, err := LoadSpawnlist(dir, zerolog.Nop(), 1)
			if err != nil {
				return 0, err
			}
			terr, _ := table.Territory("t")
			return terr.MinZ, nil
		}},
		{name: "territory node x", attr: "x", signed: true, doc: `<list><territory name="t" minZ="-10" maxZ="10"><node %s y="0"/><node x="100" y="0"/><node x="100" y="100"/></territory>` + maker + `</list>`, load: func(dir, path string) (int, error) {
			table, err := LoadSpawnlist(dir, zerolog.Nop(), 1)
			if err != nil {
				return 0, err
			}
			terr, _ := table.Territory("t")
			return terr.Nodes[0].X, nil
		}},
		{name: "spawn npc id", attr: "id", doc: `<list>` + territory + `<npcmaker name="m" territory="t" maximumNpcs="1"><npc %s total="1" respawn="1min"/></npcmaker></list>`, load: func(dir, path string) (int, error) {
			table, err := LoadSpawnlist(dir, zerolog.Nop(), 1)
			if err != nil {
				return 0, err
			}
			m, _ := table.Maker("m")
			return int(m.Entries[0].NPCID), nil
		}},
		{name: "spawn npc total", attr: "total", doc: `<list>` + territory + `<npcmaker name="m" territory="t" maximumNpcs="1"><npc id="1" %s respawn="1min"/></npcmaker></list>`, load: func(dir, path string) (int, error) {
			table, err := LoadSpawnlist(dir, zerolog.Nop(), 1)
			if err != nil {
				return 0, err
			}
			m, _ := table.Maker("m")
			return m.Entries[0].Total, nil
		}},

		// npcs/*.xml
		{name: "npc id", attr: "id", doc: `<list><npc %s name="x">` + npcRequiredSets + `</npc></list>`, load: func(dir, path string) (int, error) {
			v, err := npcByName(dir, path)
			if err != nil {
				return 0, err
			}
			return v.id, nil
		}},
		{name: "npc idTemplate", attr: "idTemplate", doc: `<list><npc id="1" %s name="x">` + npcRequiredSets + `</npc></list>`, load: func(dir, path string) (int, error) {
			v, err := npcByName(dir, path)
			if err != nil {
				return 0, err
			}
			return v.templateID, nil
		}},
		{name: "npc drop itemid", attr: "itemid", doc: npcHead + `<drops><category type="DROP"><drop %s min="1" max="1" chance="100"/></category></drops></npc></list>`, load: func(dir, path string) (int, error) {
			v, err := npcByName(dir, path)
			if err != nil {
				return 0, err
			}
			return int(v.tpl.Drops[0].Drops[0].ItemID), nil
		}},
		{name: "npc drop min", attr: "min", doc: npcHead + `<drops><category type="DROP"><drop itemid="1" %s max="100" chance="100"/></category></drops></npc></list>`, load: func(dir, path string) (int, error) {
			v, err := npcByName(dir, path)
			if err != nil {
				return 0, err
			}
			return int(v.tpl.Drops[0].Drops[0].Min), nil
		}},
		{name: "npc drop max", attr: "max", doc: npcHead + `<drops><category type="DROP"><drop itemid="1" min="1" %s chance="100"/></category></drops></npc></list>`, load: func(dir, path string) (int, error) {
			v, err := npcByName(dir, path)
			if err != nil {
				return 0, err
			}
			return int(v.tpl.Drops[0].Drops[0].Max), nil
		}},
		{name: "npc petdata food1", attr: "food1", doc: npcHead + `<petdata %s food2="1" autoFeedLimit="1" hungryLimit="1" unsummonLimit="1"/></npc></list>`, load: func(dir, path string) (int, error) {
			v, err := npcByName(dir, path)
			if err != nil {
				return 0, err
			}
			return v.tpl.Pet.Food1, nil
		}},
		{name: "npc petdata food2", attr: "food2", doc: npcHead + `<petdata food1="1" %s autoFeedLimit="1" hungryLimit="1" unsummonLimit="1"/></npc></list>`, load: func(dir, path string) (int, error) {
			v, err := npcByName(dir, path)
			if err != nil {
				return 0, err
			}
			return v.tpl.Pet.Food2, nil
		}},
		{name: "npc skill id", attr: "id", doc: npcHead + `<skills><skill %s level="1"/></skills></npc></list>`, load: func(dir, path string) (int, error) {
			v, err := npcByName(dir, path)
			if err != nil {
				return 0, err
			}
			return probeInt(func(id int) bool { _, ok := v.tpl.Skills[id]; return ok })
		}},
		{name: "npc skill level", attr: "level", doc: npcHead + `<skills><skill id="1" %s/></skills></npc></list>`, load: func(dir, path string) (int, error) {
			v, err := npcByName(dir, path)
			if err != nil {
				return 0, err
			}
			return v.tpl.Skills[1], nil
		}},
	}

	for _, f := range fields {
		for _, v := range literalValues {
			if v.signed && !f.signed {
				continue
			}
			want := v.lit
			if f.decimal {
				want = v.dec
			}
			t.Run(f.name+"/"+strconv.Quote(v.in), func(t *testing.T) {
				t.Parallel()
				dir := t.TempDir()
				file := f.file
				if file == "" {
					file = "fixture.xml"
				}
				path := filepath.Join(dir, file)
				attr := fmt.Sprintf(`%s=%q`, f.attr, v.in)
				writeXMLFixture(t, path, fmt.Sprintf(f.doc, attr))

				got, err := f.load(dir, path)
				if want == "ERR" {
					if err == nil {
						t.Fatalf("load(%s) = %d, want a rejection", attr, got)
					}
					named := regexp.MustCompile(`(^|[^A-Za-z0-9_])` + f.attr + `"?( is required|: )`)
					if msg := err.Error(); !named.MatchString(msg) || !strings.Contains(msg, path) {
						t.Fatalf("load(%s) error %q does not name attribute %q and file %q", attr, msg, f.attr, path)
					}
					return
				}
				if err != nil {
					t.Fatalf("load(%s) error: %v", attr, err)
				}
				if got == noValue {
					return
				}
				if strconv.Itoa(got) != want {
					t.Fatalf("load(%s) = %d, want %s", attr, got, want)
				}
			})
		}
	}
}

type castleView struct {
	artifactNPCID int
	castle        *castle.Castle
}

type npcView struct {
	id, templateID int
	tpl            *npc.Template
}

// TestShippedIntLiteralsReadTheSameInEitherGrammar proves the shipped
// datapack loads unchanged: every value of every attribute whose reading
// this change moved reads the same number now (an integer literal, or the
// nowReading exceptions) as it did before (plain decimal for most loaders,
// strconv base-0 for the skill and item effect and condition attributes,
// which already carry hex such as count="0x7fffffff"). Skill <table> rows
// are checked under both the literal and the decimal reading, since a
// "#name" attribute of either kind reads one of them. Each attribute must have shipped
// values unless listed in optionalInShippedData, so the scan cannot pass
// vacuously.
func TestShippedIntLiteralsReadTheSameInEitherGrammar(t *testing.T) {
	t.Parallel()
	root := datapackPath(t, filepath.Join("data", "xml"))

	// element -> literal attributes, per file set. before is how this
	// loader read the attributes until now: plain decimal, or (skills and
	// items) strconv's base-0 literals with "#" read as hex.
	specs := []struct {
		glob   string
		attrs  map[string][]string
		before func(string) (int64, error)
	}{
		{"manors.xml", map[string][]string{"manor": {"id"}}, beforeDecimal},
		{"manorAreas.xml", map[string][]string{"area": {"castleId", "minZ", "maxZ"}, "node": {"x", "y"}}, beforeDecimal},
		{"restartPointAreas.xml", map[string][]string{"area": {"minZ", "maxZ"}, "node": {"x", "y"}}, beforeDecimal},
		{"boatRoutes.xml", map[string][]string{"itinerary": {"item1", "item2", "heading"}}, beforeDecimal},
		{"summonItems.xml", map[string][]string{"item": {"id", "npcId", "summonType"}}, beforeDecimal},
		{"spellbooks.xml", map[string][]string{"book": {"skillId", "itemId"}}, beforeDecimal},
		{"bufferSkills.xml", map[string][]string{"buff": {"id", "level", "price"}}, beforeDecimal},
		{"augmentation/*.xml", map[string][]string{"set": {"order"}}, beforeDecimal},
		{"castles.xml", map[string][]string{"artifact": {"id"}, "position": {"x", "y", "z"}, "spawn": {"x", "y", "z"}, "zone": {"minZ", "maxZ"}, "node": {"x", "y"}}, beforeDecimal},
		{"clanHalls.xml", map[string][]string{"spawn": {"x", "y", "z"}, "zone": {"minZ", "maxZ"}, "node": {"x", "y"}}, beforeDecimal},
		{"observerGroups.xml", map[string][]string{"spawn": {"id", "x", "y", "z"}}, beforeDecimal},
		{"buyLists.xml", map[string][]string{"buyList": {"id", "npcId"}}, beforeDecimal},
		{"zones/*.xml", map[string][]string{"zone": {"minZ", "maxZ", "rad"}, "node": {"x", "y"}, "spawn": {"x", "y", "z"}}, beforeDecimal},
		{"doors.xml", map[string][]string{"position": {"x", "y", "z"}, "loc": {"x", "y"}}, beforeDecimal},
		{"spawnlist/*.xml", map[string][]string{"territory": {"minZ", "maxZ"}, "node": {"x", "y"}, "npc": {"id", "total"}}, beforeDecimal},
		{"npcs/*.xml", map[string][]string{"npc": {"id", "idTemplate"}, "drop": {"itemid", "min", "max"}, "petdata": {"food1", "food2"}, "skill": {"id", "level"}}, beforeDecimal},
		{"skills/*.xml items/*.xml", map[string][]string{"effect": {"count", "time", "self", "noicon", "triggeredId", "triggeredLevel", "activationChance"}, "cond": {"msgId"}, "player": conditionPlayerAttrs, "target": conditionTargetAttrs, "zone": {"minZ", "maxZ"}, "node": {"x", "y"}}, beforeBase0},
	}

	for _, spec := range specs {
		var paths []string
		for _, glob := range strings.Fields(spec.glob) {
			matched, err := filepath.Glob(filepath.Join(root, glob))
			if err != nil || len(matched) == 0 {
				t.Fatalf("%s: no shipped files (%v)", glob, err)
			}
			paths = append(paths, matched...)
		}
		seen := make(map[string]int)
		for _, path := range paths {
			data, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			for _, el := range scanAttrs(t, data) {
				for _, name := range spec.attrs[el.name] {
					raw, ok := el.attrs[name]
					if !ok {
						continue
					}
					seen[el.name+"@"+name]++
					if strings.HasPrefix(raw, "#") {
						continue // a skill table reference: its rows are checked below
					}
					now := nowReading[el.name+"@"+name]
					if now == nil {
						now = nowLiteral
					}
					for _, tok := range splitConditionList(name, raw) {
						requireUnchanged(t, now, spec.before, path, el.name+"@"+name, tok)
					}
				}
				// Skill table rows feed attributes of every reading by
				// reference, so each row must read the same under all of them.
				if el.name == "table" {
					for _, row := range strings.Fields(el.text) {
						for _, tok := range strings.Split(row, ",") {
							requireUnchanged(t, nowLiteral, spec.before, path, "table", tok)
							requireUnchanged(t, nowDecimal, spec.before, path, "table", tok)
						}
					}
				}
			}
		}
		for el, names := range spec.attrs {
			for _, name := range names {
				if seen[el+"@"+name] == 0 && !optionalInShippedData[spec.glob+" "+el+"@"+name] {
					t.Errorf("%s: no shipped %s@%s value scanned; the check is vacuous", spec.glob, el, name)
				}
			}
		}
	}
}

// optionalInShippedData lists literal attributes no shipped file carries,
// so a zero scan count for them is expected rather than a broken scan.
var optionalInShippedData = map[string]bool{
	"bufferSkills.xml buff@level":                     true,
	"bufferSkills.xml buff@price":                     true,
	"skills/*.xml items/*.xml player@active_skill_id": true,
}

// conditionPlayerAttrs and conditionTargetAttrs are the integer attributes
// of a <player>/<target> use or cast condition.
var (
	conditionPlayerAttrs = []string{"level", "hp", "mp", "pkCount", "battle_force", "spell_force", "Charges", "weight", "invSize", "pledgeClass", "castle", "sex", "clanHall", "active_skill_id", "active_skill_id_lvl", "seed_fire", "seed_water", "seed_wind", "seed_various", "seed_any"}
	conditionTargetAttrs = []string{"hp_min_max", "race_id", "npcId", "active_skill_id"}
)

// splitConditionList splits a condition's comma-separated id list or
// id,level pair into its literals; every other attribute is one literal.
func splitConditionList(name, raw string) []string {
	switch name {
	case "clanHall", "race_id", "npcId", "active_skill_id_lvl", "hp_min_max":
		parts := strings.Split(raw, ",")
		for i := range parts {
			parts[i] = strings.TrimSpace(parts[i])
		}
		return parts
	}
	return []string{raw}
}

type scannedElement struct {
	name  string
	attrs map[string]string
	text  string
}

// scanAttrs lists every element of an XML document with its attributes and
// its direct character data.
func scanAttrs(t *testing.T, data []byte) []scannedElement {
	t.Helper()
	dec := xml.NewDecoder(bytes.NewReader(data))
	var out []scannedElement
	var open []int
	for {
		tok, err := dec.Token()
		if errors.Is(err, io.EOF) {
			return out
		}
		if err != nil {
			t.Fatalf("scan: %v", err)
		}
		switch tok := tok.(type) {
		case xml.StartElement:
			el := scannedElement{name: tok.Name.Local, attrs: make(map[string]string, len(tok.Attr))}
			for _, a := range tok.Attr {
				el.attrs[a.Name.Local] = a.Value
			}
			out = append(out, el)
			open = append(open, len(out)-1)
		case xml.EndElement:
			open = open[:len(open)-1]
		case xml.CharData:
			if len(open) > 0 {
				out[open[len(open)-1]].text += string(tok)
			}
		}
	}
}

// nowReading lists the changed attributes whose reading is not the integer
// literal: effect trigger fields moved from literals to plain decimals, and
// the two forces are literals limited to a signed byte.
var nowReading = map[string]func(string) (int64, error){
	"effect@triggeredId":      nowDecimal,
	"effect@triggeredLevel":   nowDecimal,
	"effect@activationChance": nowDecimal,
	"player@battle_force":     nowByte,
	"player@spell_force":      nowByte,
}

func nowLiteral(raw string) (int64, error) {
	n, err := commons.DecodeInt32(raw)
	return int64(n), err
}

func nowDecimal(raw string) (int64, error) { return strconv.ParseInt(raw, 10, 32) }

func nowByte(raw string) (int64, error) {
	n, err := nowLiteral(raw)
	if err == nil && (n < math.MinInt8 || n > math.MaxInt8) {
		return 0, fmt.Errorf("%q: value out of byte range", raw)
	}
	return n, err
}

// beforeDecimal and beforeBase0 are the two readings the changed
// attributes had until now.
func beforeDecimal(raw string) (int64, error) { return strconv.ParseInt(raw, 10, 32) }

func beforeBase0(raw string) (int64, error) {
	if strings.HasPrefix(raw, "#") {
		raw = "0x" + raw[1:]
	}
	return strconv.ParseInt(raw, 0, 32)
}

// requireUnchanged fails when raw loads differently under now than under
// before. A value neither reading accepts (a float table row, a non-numeric
// token) has no integer reading to compare and is skipped.
func requireUnchanged(t *testing.T, now, before func(string) (int64, error), path, what, raw string) {
	t.Helper()
	cur, curErr := now(raw)
	old, oldErr := before(raw)
	if curErr != nil && oldErr != nil {
		if what != "table" {
			t.Errorf("%s: %s=%q is not an integer in either reading", path, what, raw)
		}
		return
	}
	if curErr != nil || oldErr != nil || cur != old {
		t.Errorf("%s: %s=%q reads differently now (%d, %v) than before (%d, %v)", path, what, raw, cur, curErr, old, oldErr)
	}
}

// TestSkillEffectIntAttrGrammarMatchesReference pins the integer attributes
// of a skill <effect> to the reference's reads: count and time are
// Integer.decode literals, while triggeredId, triggeredLevel and
// activationChance are plain Integer.parseInt decimals. Expected values
// come from literalValues (the Java probe); a malformed value skips the
// skill rather than failing the load, as every other per-skill error does.
func TestSkillEffectIntAttrGrammarMatchesReference(t *testing.T) {
	t.Parallel()
	fields := []struct {
		attr    string
		decimal bool
		get     func(skill.EffectTemplate) int
	}{
		{attr: "count", get: func(e skill.EffectTemplate) int { return e.Count }},
		{attr: "time", get: func(e skill.EffectTemplate) int { return e.Time }},
		{attr: "triggeredId", decimal: true, get: func(e skill.EffectTemplate) int { return e.TriggeredID }},
		{attr: "triggeredLevel", decimal: true, get: func(e skill.EffectTemplate) int { return e.TriggeredLevel }},
		{attr: "activationChance", decimal: true, get: func(e skill.EffectTemplate) int { return e.ActivationChance }},
	}
	for _, f := range fields {
		for _, v := range literalValues {
			want := v.lit
			if f.decimal {
				want = v.dec
			}
			if strings.HasPrefix(v.in, "#") {
				// In a skill file "#name" is a table reference; with no such
				// table it resolves to "", which no reading accepts.
				want = "ERR"
			}
			t.Run(f.attr+"/"+strconv.Quote(v.in), func(t *testing.T) {
				t.Parallel()
				dir := t.TempDir()
				writeXMLFixture(t, filepath.Join(dir, "fixture.xml"), skillFixture(fmt.Sprintf(`<for><effect name="Buff" val="0" %s=%q/></for>`, f.attr, v.in)))
				table, err := LoadSkillDefinitions(dir, zerolog.Nop())
				if err != nil {
					t.Fatalf("LoadSkillDefinitions: %v", err)
				}
				def, ok := table.Get(1, 1)
				if want == "ERR" {
					if ok {
						t.Fatalf("%s=%q loaded as %d, want the skill skipped", f.attr, v.in, f.get(def.Effects[0]))
					}
					return
				}
				if !ok {
					t.Fatalf("%s=%q skipped the skill, want %s", f.attr, v.in, want)
				}
				if got := strconv.Itoa(f.get(def.Effects[0])); got != want {
					t.Fatalf("%s=%q = %s, want %s", f.attr, v.in, got, want)
				}
			})
		}
	}
}
