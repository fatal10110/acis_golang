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
const variantEnv = "ACIS_PERF_VARIANT"

// variantBase is where a variant that places clients starts: a point inside
// Gludio castle's station, on the tile the geo variant loads.
var variantBase = location.Location{X: -19032, Y: 108072, Z: -2032}

// regionStep is the distance between neighbouring occupied regions of the
// spread variant: wider than a region, so each holds its own clients.
const regionStep = 4096

// variants is what ACIS_PERF_VARIANT selected.
type variants struct {
	spread, geo, scripts bool
	regions              int
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
			default:
				t.Fatalf("%s=%q: want a comma-separated list of spread, geo, scripts", variantEnv, raw)
			}
		}
	}
	if v.spread && v.geo {
		t.Fatalf("%s: spread and geo do not combine: the geo variant loads one tile", variantEnv)
	}
	v.regions = envInt(t, "ACIS_PERF_REGIONS", 16)
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

func (v variants) String() string {
	var names []string
	for name, on := range map[string]bool{"spread": v.spread, "geo": v.geo, "scripts": v.scripts} {
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
				OnAttacked: func(*script.Script, script.Attacked) { m.attacked.Add(1) },
			},
		}
	}}
}

// options are the boot options the variants add for a monster template.
func (v variants) options(tmpl *npc.Template, geo move.Geo, ms *monsterScript) []gameservertest.Option {
	var opts []gameservertest.Option
	if v.geo {
		opts = append(opts, gameservertest.WithGeo(geo))
	}
	if v.scripts {
		opts = append(opts,
			gameservertest.WithNPCs(npc.NewTable([]*npc.Template{tmpl})),
			gameservertest.WithTemplateScripts(ms.listing(), ms.catalog()))
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
