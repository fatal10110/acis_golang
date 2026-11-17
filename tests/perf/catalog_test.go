package perf

import (
	"cmp"
	"maps"
	"path/filepath"
	"slices"
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
// content catalogs the production boot joins from internal/gameserver/script/
// catalog, and the shipped NPC and skill tables. The engine catalogs (quests,
// features, teleporters, tasks and the base behaviors) live in the server's
// main package and are not importable here.
type prodCatalog struct {
	list    []script.Listing
	catalog script.Catalog
	skills  *modelskill.Table
	// templates are the monsters the clients fight: one copy of a shipped
	// template per scripted behavior, in the order clients take them.
	templates []*npc.Template
	// table holds templates, for the boot.
	table *npc.Table
}

// loadProdCatalog loads the shipped data (skipping without a datapack) and
// picks up to n monster templates, each bound to a different catalog script
// that hooks the attack event, lowest level first.
func loadProdCatalog(t *testing.T, n int) *prodCatalog {
	t.Helper()
	root := datapack.Path(t, "data", "xml")
	list, err := gamexml.LoadScriptList(filepath.Join(root, "scripts.xml"), zerolog.Nop())
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

	files := catalog.Scripts(thirdclass.NewFamily())
	joined := script.Catalog{}
	for _, name := range slices.Sorted(maps.Keys(files)) {
		for path, ctor := range files[name] {
			if ctor != nil {
				joined[path] = ctor
			}
		}
	}
	var listed []script.Listing
	for _, l := range list {
		if joined[l.Path] != nil {
			listed = append(listed, l)
		}
	}

	kindOf := script.NPCKindOf(npcs)
	taken := map[int32]bool{}
	var picks []*npc.Template
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
		// A copy: the run tunes the attack power, never the shipped table.
		tmpl := *best
		tmpl.PAtk = 0.25
		tmpl.HPMax = 1000 // a shipped low-level monster would die to a single hit between two refills
		tmpl.CanMove = true
		// 200 clients share one spot: an aggressive monster, or one that calls
		// its clan-mates, would have a crowd of monsters on one client.
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
	return &prodCatalog{list: listed, catalog: joined, skills: skills, templates: picks, table: npc.NewTable(picks)}
}
