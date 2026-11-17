package perf

import (
	"fmt"
	"path/filepath"
	"slices"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/geo/engine"
	"github.com/fatal10110/acis_golang/internal/gameserver/geo/reader"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/move"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/npc"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/script"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
	"github.com/fatal10110/acis_golang/internal/testsupport/datapack"
)

// variantEnv names the comma-separated variants of TestLoadBaseline; unset
// runs the baseline.
//
//	spread   clients stand in a grid of regions (ACIS_PERF_REGIONS, default 16)
//	         instead of one spot, so the region fan-out and visibility
//	         bookkeeping see many occupied regions
//	geo      clients and monsters stand on the shipped geodata of a Gludio
//	         tile: movement and attacks go through the geo engine's CanMove
//	         and Height. It skips without geodata (not in the repository;
//	         ACIS_GEODATA_DIR or an aCis_datapack that carries it)
//	scripts  every monster has a script on the production registry path,
//	         subscribed to the attack hook, so the script engine's dispatch
//	         shows in the profile
//	cast     clients cast a skill at their monster and the monsters, wired
//	         with the AI-cast seam, cast one back at whoever struck them:
//	         the cast, effect and NPC cast-desire paths run under load
//	catalog  the monsters are shipped templates, each bound to a different
//	         script of the production content catalog (scripts.xml, the
//	         catalog package, the shipped NPC and skill tables), so the real
//	         behaviors' hooks run on every hit. The monsters have no cast
//	         seam: a shipped skill would one-shot the 36-HP test class (the
//	         cast variant covers casting). It skips without a datapack.
//	         ACIS_PERF_CATALOG_NPCS bounds how many templates. It implies
//	         spread, one region per client unless ACIS_PERF_REGIONS says
//	         otherwise: the shipped attacked reactions (call for help) pulled
//	         monsters onto neighbouring clients when regions were shared
//	fixture  the database holds a production-sized item and character
//	         population (ACIS_PERF_ITEMS per client, ACIS_PERF_BG_CHARS other
//	         characters, ACIS_PERF_BG_ITEMS other item rows), and the run
//	         ends by stopping the server with every client connected,
//	         logging how long the drain and the detach saves took
const variantEnv = "ACIS_PERF_VARIANT"

// variantBase is where a variant that places clients starts: a point inside
// Gludio castle's station, on the tile the geo variant loads.
var variantBase = location.Location{X: -19032, Y: 108072, Z: -2032}

// regionStep is the distance between neighbouring occupied regions of the
// spread variant: wider than a region, so each holds its own clients.
const regionStep = 4096

// variants is what ACIS_PERF_VARIANT selected.
type variants struct {
	spread, geo, scripts, cast, catalog, fixture bool
	regions                                      int
	catalogNPCs                                  int
}

func variantsFromEnv(t *testing.T) variants {
	t.Helper()
	var v variants
	if raw := strings.TrimSpace(envString(variantEnv)); raw != "" {
		for _, name := range strings.Split(raw, ",") {
			switch strings.TrimSpace(name) {
			case "spread":
				v.spread = true
			case "geo":
				v.geo = true
			case "scripts":
				v.scripts = true
			case "cast":
				v.cast = true
			case "catalog":
				v.catalog = true
			case "fixture":
				v.fixture = true
			default:
				t.Fatalf("%s=%q: want a comma-separated list of spread, geo, scripts, cast, catalog, fixture", variantEnv, raw)
			}
		}
	}
	// The shipped attacked reactions call for help: monsters of shared
	// regions were pulled onto clients 0 and 1, so each client of the catalog
	// variant stands in a region of its own.
	if v.catalog {
		v.spread = true
	}
	if v.spread && v.geo {
		t.Fatalf("%s: spread and geo do not combine: the geo variant loads one tile", variantEnv)
	}
	if scripted := countTrue(v.scripts, v.cast, v.catalog); scripted > 1 {
		t.Fatalf("%s: scripts, cast and catalog do not combine: each boots its own script set", variantEnv)
	}
	v.catalogNPCs = envInt(t, "ACIS_PERF_CATALOG_NPCS", 24)
	if v.catalogNPCs < 1 {
		t.Fatalf("ACIS_PERF_CATALOG_NPCS=%d: want at least 1", v.catalogNPCs)
	}
	defaultRegions := 16
	if v.catalog {
		defaultRegions = max(defaultRegions, envInt(t, "ACIS_PERF_CLIENTS", 0))
	}
	v.regions = envInt(t, "ACIS_PERF_REGIONS", defaultRegions)
	if v.regions < 1 {
		t.Fatalf("ACIS_PERF_REGIONS=%d: want at least 1", v.regions)
	}
	return v
}

// placed reports whether the variant puts clients at chosen positions rather
// than the class template's start.
func (v variants) placed() bool { return v.spread || v.geo }

// place is where client i starts: its region's corner in a square grid for
// the spread variant, the base point otherwise.
func (v variants) place(i int) location.Location {
	if !v.spread {
		return variantBase
	}
	cols := 1
	for cols*cols < v.regions {
		cols++
	}
	slot := i % v.regions
	return location.Location{X: variantBase.X + slot%cols*regionStep, Y: variantBase.Y + slot/cols*regionStep, Z: variantBase.Z}
}

func countTrue(flags ...bool) int {
	n := 0
	for _, f := range flags {
		if f {
			n++
		}
	}
	return n
}

func (v variants) String() string {
	var names []string
	for name, on := range map[string]bool{"spread": v.spread, "geo": v.geo, "scripts": v.scripts, "cast": v.cast, "catalog": v.catalog, "fixture": v.fixture} {
		if on {
			names = append(names, name)
		}
	}
	if len(names) == 0 {
		return "baseline"
	}
	slices.Sort(names)
	return strings.Join(names, ",")
}

// loadGeo reads the geodata tile that holds variantBase into a geo engine,
// skipping the test when the data is absent.
func loadGeo(t *testing.T) move.Geo {
	t.Helper()
	tileX := engine.TileXMin + (variantBase.X-engine.WorldXMin)/engine.TileSize
	tileY := engine.TileYMin + (variantBase.Y-engine.WorldYMin)/engine.TileSize
	path := filepath.Join(datapack.Geodata(t), fmt.Sprintf("%d_%d_conv.dat", tileX, tileY))
	region, _, err := reader.ReadL2OFF(path)
	if err != nil {
		t.Skipf("geodata tile %s unreadable: %v", path, err)
	}
	geo := engine.New()
	if err := geo.SetRegion(tileX, tileY, region); err != nil {
		t.Fatalf("SetRegion: %v", err)
	}
	return move.NewGeo(geo, nil)
}

// monsterScript is the scripts variant's script: it subscribes to the attack
// hook of the monster template and counts what it hears.
type monsterScript struct {
	templateID int32
	attacked   atomic.Int64
	// castRef, when set, is the skill the monster answers every hit with: a
	// cast desire at the attacker.
	castRef modelskill.Ref
}

const monsterScriptKey = "perf/monster"

func (m *monsterScript) listing() []script.Listing {
	return []script.Listing{{Path: monsterScriptKey}}
}

func (m *monsterScript) catalog() script.Catalog {
	return script.Catalog{monsterScriptKey: func() script.Script {
		return script.Script{
			Bind: script.Bindings{script.EventAttacked: []int32{m.templateID}},
			Hooks: script.Hooks{
				OnAttacked: func(_ *script.Script, e script.Attacked) {
					m.attacked.Add(1)
					if m.castRef.ID != 0 {
						e.NPC.AddCastDesire(e.Attacker, m.castRef, 1000)
					}
				},
			},
		}
	}}
}

// options are the boot options the variants add for a monster template:
// ms is the counter script of the scripts and cast variants, kit the cast
// variant's skills, cat the catalog variant's data.
func (v variants) options(tmpl *npc.Template, geo move.Geo, ms *monsterScript, kit *castKit, cat *prodCatalog) []gameservertest.Option {
	var opts []gameservertest.Option
	if v.geo {
		opts = append(opts, gameservertest.WithGeo(geo))
	}
	switch {
	case v.scripts:
		opts = append(opts,
			gameservertest.WithNPCs(npc.NewTable([]*npc.Template{tmpl})),
			gameservertest.WithTemplateScripts(ms.listing(), ms.catalog()))
	case v.cast:
		opts = append(opts,
			kit.opt,
			gameservertest.WithNPCs(npc.NewTable([]*npc.Template{tmpl})),
			gameservertest.WithScriptSkills(castDefinitions()...),
			gameservertest.WithTemplateScripts(ms.listing(), ms.catalog()))
	case v.catalog:
		opts = append(opts,
			gameservertest.WithNPCs(cat.table),
			gameservertest.WithScriptSkills(cat.skills.All()...),
			gameservertest.WithTemplateScripts(cat.list, cat.catalog))
	}
	return opts
}

// monsterSpot is where client i's monster stands, out of melee reach and in
// line of sight. Without geodata that is 60 or more to the east in a grid of
// ten columns, as the baseline always has it; on geodata the spot is the
// first one east, north, west or south of the client, at growing distance,
// the client can walk to.
func (v variants) monsterSpot(geo move.Geo, origin location.Location, i int) location.Location {
	if !v.geo {
		return location.Location{X: origin.X + 60 + i%10*20, Y: origin.Y + i/10*20, Z: origin.Z}
	}
	for dist := 60; dist <= 400; dist += 20 {
		for _, d := range [...][2]int{{1, 0}, {0, 1}, {-1, 0}, {0, -1}} {
			x, y := origin.X+d[0]*dist, origin.Y+d[1]*dist
			z := int(geo.Height(x, y, origin.Z))
			if geo.CanMove(origin.X, origin.Y, origin.Z, x, y, z) {
				return location.Location{X: x, Y: y, Z: z}
			}
		}
	}
	return origin
}
