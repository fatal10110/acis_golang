package perf

import (
	"cmp"
	"path/filepath"
	"slices"
	"sync/atomic"
	"testing"

	gamexml "github.com/fatal10110/acis_golang/internal/gameserver/data/xml"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/npc"
	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/script"
	"github.com/fatal10110/acis_golang/internal/gameserver/script/catalog"
	"github.com/fatal10110/acis_golang/internal/gameserver/script/quest/thirdclass"
	"github.com/fatal10110/acis_golang/internal/testsupport/datapack"
	"github.com/rs/zerolog"
)

// prodCatalog is the catalog variant's data: the shipped scripts.xml, the
// catalog files the production boot joins from internal/gameserver/script/
// catalog, the engine file (quests, features, teleporters, tasks and the
// base behaviors) among them, and the shipped NPC and skill tables.
type prodCatalog struct {
	list    []script.Listing
	catalog script.Catalog
	skills  *modelskill.Table
	// engineListed counts the listed scripts that come from the engine
	// catalog file, enginePicked the monster templates bound to one of them.
	engineListed, enginePicked int
	// templates are the monsters the clients fight: one copy of a shipped
	// template per scripted behavior, in the order clients take them.
	templates []*npc.Template
	// table holds templates, for the boot.
	table *npc.Table
	// attacked counts the attack hooks the picked scripts ran.
	attacked *atomic.Int64
}

// shippedCastSkill is the shipped skill the shipped variant's clients cast:
// Energy Bolt, a single-target magic hit of 13 power at level 1 with a
// two-second hit time and a three-second reuse, which the catalog monsters
// (given 1000 HP by loadProdCatalog) absorb.
const shippedCastSkill = modelskill.ID(1274)

// newShippedKit is the shipped variant's kit over the shipped skill table.
func newShippedKit(t *testing.T, skills *modelskill.Table) *castKit {
	t.Helper()
	if _, ok := skills.Get(shippedCastSkill, 1); !ok {
		t.Fatalf("the shipped skill table has no skill %d level 1", shippedCastSkill)
	}
	// A cast request on every attack step: a swing in flight only queues it
	// behind the swing, so asking again is what lets one start.
	return newKit(t, skills, shippedCastSkill, []int{0, 1, 2, 3, 4, 5}, true)
}

// loadProdCatalog loads the shipped data (skipping without a datapack) and
// picks up to n monster templates, each bound to a different catalog script
// that hooks the attack event, lowest level first. The scripts run against
// the harness environment, not the production one (no castles, sieges or
// clan halls): the engine quests, features and tasks are registered, and only
// the picked behaviors' attack hooks run.
func loadProdCatalog(t *testing.T, n int) *prodCatalog {
	t.Helper()
	root := datapack.Path(t, "data", "xml")
	list, err := script.LoadList(filepath.Join(root, "scripts.xml"), zerolog.Nop())
	if err != nil {
		t.Fatal(err)
	}
	items, err := gamexml.LoadItemTemplates(filepath.Join(root, "items"), zerolog.Nop())
	if err != nil {
		t.Fatal(err)
	}
	skills, err := gamexml.LoadSkillDefinitions(filepath.Join(root, "skills"), zerolog.Nop())
	if err != nil {
		t.Fatal(err)
	}
	npcs, err := gamexml.LoadNPCTemplates(filepath.Join(root, "npcs"), items, skills, zerolog.Nop())
	if err != nil {
		t.Fatal(err)
	}

	crystals, err := gamexml.LoadSoulCrystalData(filepath.Join(root, "soulCrystals.xml"))
	if err != nil {
		t.Fatal(err)
	}
	buffs, err := gamexml.LoadNewbieBuffs(filepath.Join(root, "newbieBuffs.xml"))
	if err != nil {
		t.Fatal(err)
	}

	// The production boot's catalog; the engine file names the scripts
	// ported with the engine.
	joined, err := catalog.ScriptCatalog(crystals, buffs)
	if err != nil {
		t.Fatal(err)
	}
	engineOwned := catalog.Scripts(nil, nil, thirdclass.NewFamily())["engine"]
	var listed []script.Listing
	engineListed := 0
	for _, l := range list {
		if joined[l.Path] != nil {
			listed = append(listed, l)
			if engineOwned[l.Path] != nil {
				engineListed++
			}
		}
	}

	kindOf := script.NPCKindOf(npcs)
	taken := map[int32]bool{}
	pickedBy := map[int]string{} // template id to the script it was picked for
	var picks []*npc.Template
	attacked := new(atomic.Int64)
	for _, l := range listed {
		s := joined[l.Path]()
		if !s.Behavior || s.Hooks.OnAttacked == nil {
			continue
		}
		var candidates []*npc.Template
		for _, id := range s.NPCs {
			tmpl, ok := npcs.Get(int(id))
			if kind, _ := kindOf(id); ok && kind == script.KindHostile && !taken[id] {
				candidates = append(candidates, tmpl)
			}
		}
		if len(candidates) == 0 {
			continue
		}
		best := slices.MinFunc(candidates, func(a, b *npc.Template) int {
			return cmp.Or(cmp.Compare(a.Level, b.Level), cmp.Compare(a.ID, b.ID))
		})
		taken[int32(best.ID)] = true
		pickedBy[best.ID] = l.Path
		// The pick counts its attack hook, then runs the shipped one.
		ctor := joined[l.Path]
		joined[l.Path] = func() script.Script {
			s := ctor()
			shipped := s.Hooks.OnAttacked
			s.Hooks.OnAttacked = func(self *script.Script, e script.Attacked) {
				attacked.Add(1)
				shipped(self, e)
			}
			return s
		}
		// A copy: the run tunes the attack power, never the shipped table.
		tmpl := *best
		tmpl.PAtk = 0.25
		tmpl.HPMax = 1000 // a shipped low-level monster would die to a single hit between two refills
		tmpl.CanMove = true
		// Zeroed so ACIS_PERF_REGIONS below the client count, which puts
		// several clients in one region, does not gather a crowd of monsters
		// on one client.
		tmpl.AggroRange = 0
		tmpl.Clans, tmpl.ClanRange = nil, 0
		picks = append(picks, &tmpl)
	}
	slices.SortFunc(picks, func(a, b *npc.Template) int {
		return cmp.Or(cmp.Compare(a.Level, b.Level), cmp.Compare(a.ID, b.ID))
	})
	if len(picks) > n {
		picks = picks[:n]
	}
	if len(picks) == 0 {
		t.Fatal("no catalog script binds a hostile NPC to the attack hook")
	}
	enginePicked := 0
	for _, p := range picks {
		if engineOwned[pickedBy[p.ID]] != nil {
			enginePicked++
		}
	}
	return &prodCatalog{list: listed, catalog: joined, engineListed: engineListed, enginePicked: enginePicked, skills: skills, templates: picks, table: npc.NewTable(picks), attacked: attacked}
}
