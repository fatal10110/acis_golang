package main

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	gamexml "github.com/fatal10110/acis_golang/internal/gameserver/data/xml"
	"github.com/fatal10110/acis_golang/internal/gameserver/script"
	"github.com/fatal10110/acis_golang/internal/testsupport/datapack"
	"github.com/rs/zerolog"
)

func TestScriptCatalogRejectsDuplicates(t *testing.T) {
	ctor := func() script.Script { return script.Script{} }
	if _, err := scriptCatalog([]script.Catalog{{"quest.A": ctor}, {"quest.B": ctor}}); err != nil {
		t.Fatal(err)
	}
	if _, err := scriptCatalog([]script.Catalog{{"quest.A": ctor}, {"quest.A": ctor}}); err == nil {
		t.Fatal("a path in two catalogs joined")
	}
}

// TestScriptCatalogCompleteness reports every scripts.xml path that has no
// Go script yet, and fails on a catalog entry scripts.xml does not list:
// it would never be built.
func TestScriptCatalogCompleteness(t *testing.T) {
	list, err := gamexml.LoadScriptList(datapack.Path(t, "data", "xml", "scripts.xml"), zerolog.Nop())
	if err != nil {
		t.Fatal(err)
	}
	catalog, err := scriptCatalog(scriptCatalogs())
	if err != nil {
		t.Fatal(err)
	}
	listed := map[string]bool{}
	var missing []string
	for _, l := range list {
		listed[l.Path] = true
		if _, ok := catalog[l.Path]; !ok {
			missing = append(missing, l.Path)
		}
	}
	for path := range catalog {
		if !listed[path] {
			t.Errorf("catalog entry %s is not listed in scripts.xml", path)
		}
	}
	t.Logf("%d of %d listed scripts have no Go script yet", len(missing), len(list))
	for _, path := range missing {
		t.Logf("unported: %s", path)
	}
}

// manifestScript is what the reference manifest records for one script:
// its kind, its class's own hooks, its bind lines and whether its entry
// schedules it.
type manifestScript struct {
	kind      string
	hooks     []string
	binds     []string
	scheduled bool
}

// quest hook methods: the reference's on-methods a script overrides to
// react.
var questHookMethods = map[string]bool{
	"onAbnormalStatusChanged": true, "onAdvEvent": true, "onAttackFinished": true, "onAttacked": true,
	"onClanAttacked": true, "onClanDied": true, "onCreated": true, "onDeath": true, "onDecayed": true,
	"onDoorChange": true, "onEnterWorld": true, "onFirstTalk": true, "onGameTime": true, "onItemUse": true,
	"onMakerNpcsKilled": true, "onMoveToFinished": true, "onMyDying": true, "onNoDesire": true,
	"onOutOfTerritory": true, "onPartyAttacked": true, "onPartyDied": true, "onPickedItem": true,
	"onScriptEvent": true, "onSeeCreature": true, "onSeeItem": true, "onSeeSpell": true, "onSpelled": true,
	"onStaticObjectClanAttacked": true, "onTalk": true, "onTimer": true, "onUseSkillFinished": true,
	"onZoneEnter": true, "onZoneExit": true,
	// A scheduled task's start hook. Its end hook is left out: no task
	// reacts to its end, and the engine builds no end hook.
	"onStart": true,
}

func readScriptManifest(t *testing.T) map[string]*manifestScript {
	t.Helper()
	f, err := os.Open(filepath.Join("..", "..", "internal", "gameserver", "script", "testdata", "oracle", "manifest.golden"))
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	scripts := map[string]*manifestScript{}
	classHooks := map[string][]string{}
	var cur *manifestScript
	var class string
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 1<<20), 1<<20)
	for sc.Scan() {
		line := sc.Text()
		fields := strings.Fields(line)
		switch {
		case strings.HasPrefix(line, "script "):
			cur = &manifestScript{}
			scripts[fields[1]] = cur
		case strings.HasPrefix(line, "  kind "):
			cur.kind = fields[1]
		case strings.HasPrefix(line, "  bind "):
			cur.binds = append(cur.binds, strings.TrimSpace(line))
		case line == "  scheduled" && cur != nil:
			cur.scheduled = true
		case strings.HasPrefix(line, "class "):
			class = fields[1]
		case strings.HasPrefix(line, "  hook ") && class != "":
			name, _, _ := strings.Cut(fields[1], "(")
			if questHookMethods[name] {
				classHooks[class] = append(classHooks[class], name)
			}
		}
	}
	if err := sc.Err(); err != nil {
		t.Fatal(err)
	}
	for path, s := range scripts {
		s.hooks = classHooks[path]
		slices.Sort(s.hooks)
	}
	return scripts
}

// manifestMismatches builds the registry from list and catalog with cfg
// and returns every way a catalog script's dump differs from the reference
// manifest: not registered, kind, own hooks, bind lines.
func manifestMismatches(t *testing.T, list []script.Listing, catalog script.Catalog, cfg script.Config) []string {
	t.Helper()
	want := readScriptManifest(t)
	var dump strings.Builder
	if err := script.Build(list, catalog, cfg).Dump(&dump); err != nil {
		t.Fatal(err)
	}
	got := map[string]*manifestScript{}
	var out []string
	var cur *manifestScript
	for line := range strings.Lines(dump.String()) {
		line = strings.TrimSuffix(line, "\n")
		fields := strings.Fields(line)
		switch {
		case strings.HasPrefix(line, "script "):
			cur = nil
			if _, ported := catalog[fields[1]]; !ported {
				continue
			}
			if len(fields) > 2 {
				out = append(out, line)
				continue
			}
			cur = &manifestScript{}
			got[fields[1]] = cur
		case cur == nil:
		case strings.HasPrefix(line, "  kind "):
			cur.kind = fields[1]
		case strings.HasPrefix(line, "  hooks "):
			cur.hooks = strings.Split(fields[1], ",")
		case strings.HasPrefix(line, "  bind "):
			cur.binds = append(cur.binds, strings.TrimSpace(line))
		case line == "  scheduled":
			cur.scheduled = true
		}
	}
	for path, g := range got {
		w, ok := want[path]
		switch {
		case !ok:
			out = append(out, path+": not in the reference manifest")
		default:
			if g.kind != w.kind {
				out = append(out, path+": kind "+g.kind+", reference "+w.kind)
			}
			if !slices.Equal(g.hooks, w.hooks) {
				out = append(out, path+": own hooks "+strings.Join(g.hooks, ",")+", reference "+strings.Join(w.hooks, ","))
			}
			if !slices.Equal(g.binds, w.binds) {
				out = append(out, path+": binds "+strings.Join(g.binds, "; ")+", reference "+strings.Join(w.binds, "; "))
			}
			if g.scheduled != w.scheduled {
				out = append(out, fmt.Sprintf("%s: scheduled %v, reference %v", path, g.scheduled, w.scheduled))
			}
		}
	}
	slices.Sort(out)
	return out
}

// TestManifestComparisonCatchesDifferences checks the comparison itself on
// stand-ins for the reference's first quest: one the seam gate refuses, as
// it refuses every subscribing script until its hooks are raised, and one
// that registers without the bindings the reference records.
func TestManifestComparisonCatchesDifferences(t *testing.T) {
	list := []script.Listing{{Path: "quest.Q001_LettersOfLove"}}
	q001 := script.Catalog{"quest.Q001_LettersOfLove": func() script.Script {
		return script.Script{
			QuestID: 1,
			Bind:    script.Bindings{script.EventQuestStart: {30048}, script.EventTalked: {30006, 30033, 30048}},
			Hooks:   script.Hooks{OnTalk: func(*script.Script, script.Talk) string { return "" }},
		}
	}}
	folk := func(int32) (script.NPCKind, bool) { return script.KindFolk, true }
	got := manifestMismatches(t, list, q001, script.Config{KindOf: folk, Log: zerolog.Nop()})
	if !slices.Equal(got, []string{"script quest.Q001_LettersOfLove refused"}) {
		t.Fatalf("a refused script gave %q", got)
	}

	noTemplates := func(int32) (script.NPCKind, bool) { return script.KindOther, false }
	got = manifestMismatches(t, list, q001, script.Config{KindOf: noTemplates, Log: zerolog.Nop()})
	if len(got) != 2 || !strings.Contains(got[0], ": binds ") || !strings.Contains(got[1], ": own hooks onTalk, reference onAdvEvent,onTalk") {
		t.Fatalf("a script without its bindings and hooks gave %q, want a bind and a hook mismatch", got)
	}

	// A task its entry does not schedule.
	tasks := script.Catalog{"task.CastleTaxRefresh": taskCatalog()["task.CastleTaxRefresh"]}
	got = manifestMismatches(t, []script.Listing{{Path: "task.CastleTaxRefresh"}}, tasks, script.Config{KindOf: noTemplates, Log: zerolog.Nop()})
	if !slices.Equal(got, []string{"task.CastleTaxRefresh: scheduled false, reference true"}) {
		t.Fatalf("an unscheduled task gave %q, want a scheduled mismatch", got)
	}
}

// TestScriptCatalogMatchesManifest: every ported script registers at boot
// against the datapack's NPC templates and the hooks the engine raises, and
// its kind, own hooks and bindings equal the reference manifest's.
func TestScriptCatalogMatchesManifest(t *testing.T) {
	root := datapack.Path(t, "data", "xml")
	list, err := gamexml.LoadScriptList(filepath.Join(root, "scripts.xml"), zerolog.Nop())
	if err != nil {
		t.Fatal(err)
	}
	catalog, err := scriptCatalog(scriptCatalogs())
	if err != nil {
		t.Fatal(err)
	}
	if len(catalog) == 0 {
		t.Log("no script is ported yet; nothing to compare")
		return
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
	cfg := script.Config{KindOf: npcKindOf(npcs), Log: zerolog.Nop()}
	for _, m := range manifestMismatches(t, list, catalog, cfg) {
		t.Error(m)
	}
}
