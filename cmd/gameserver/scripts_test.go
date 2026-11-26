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
	"github.com/fatal10110/acis_golang/internal/gameserver/script/catalog"
	scripttask "github.com/fatal10110/acis_golang/internal/gameserver/script/task"
	"github.com/fatal10110/acis_golang/internal/testsupport/datapack"
	"github.com/rs/zerolog"
)

// TestScriptCatalogCompleteness fails on every scripts.xml path that has
// no Go script, and on a catalog entry scripts.xml does not list: it would
// never be built. A task the file lists only in a comment (commentedTasks)
// is exempt.
func TestScriptCatalogCompleteness(t *testing.T) {
	list, err := script.LoadList(datapack.Path(t, "data", "xml", "scripts.xml"), zerolog.Nop())
	if err != nil {
		t.Fatal(err)
	}
	joined, err := catalog.ScriptCatalog(nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	listed := map[string]bool{}
	var missing []string
	for _, l := range list {
		listed[l.Path] = true
		if _, ok := joined[l.Path]; !ok {
			missing = append(missing, l.Path)
		}
	}
	for path := range joined {
		if !listed[path] && !slices.Contains(commentedTasks, path) {
			t.Errorf("catalog entry %s is not listed in scripts.xml", path)
		}
	}
	for _, path := range missing {
		t.Errorf("listed script %s has no Go script", path)
	}
}

// manifestScript is what the reference manifest records for one script:
// its kind, its class's own hooks, its player-level triggers, its bind
// lines and whether its entry schedules it.
type manifestScript struct {
	kind      string
	hooks     []string
	on        []string
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

// approvedBindDivergences: bind lines a script registers differently from
// the reference on an owner decision, keyed by script path and then by the
// reference line, with the line the port registers instead. Each entry
// names the issue that records the decision.
var approvedBindDivergences = map[string]map[string]string{
	// #5161: the soul of the well also frees its cell and its timers when
	// it decays, whoever killed it.
	"quest.Q022_TragedyInVonHellmannForest": {"bind DECAYED 31528": "bind DECAYED 27217,31528"},
	// #5160: a linked script runs only through an NPC bound to it, and
	// every village master's page links the shadow weapon exchange.
	"script.feature.ShadowWeapon": {
		"bind TALKED 30026,30031,30037,30066,30070,30109,30115,30120,30154,30174,30176,30187,30191,30195,30288-30290,30297,30358,30373,30462,30474,30498-30500,30503-30505,30508,30511-30513,30520,30525,30565,30594,30595,30676,30677,30681,30685,30687,30689,30694,30699,30704,30845,30847,30849,30854,30857,30862,30865,30894,30897,30900,30905,30910,30913,31269,31272,31276,31279,31285,31288,31314,31317,31321,31324,31326,31328,31331,31334,31336,31755,31958,31961,31965,31968,31974,31977,31996,32092-32098": "bind TALKED 30026,30031,30037,30066,30070,30109,30115,30120,30154,30174-30176,30187,30191,30195,30288-30290,30297,30358,30373,30462,30474,30498-30500,30503-30505,30508,30511-30513,30520,30525,30565,30594,30595,30676,30677,30681,30685,30687,30689,30694,30699,30704,30845,30847,30849,30854,30857,30862,30865,30894,30897,30900,30905,30910,30913,31269,31272,31276,31279,31285,31288,31314,31317,31321,31324,31326,31328,31331,31334,31336,31755,31958,31961,31965,31968,31974,31977,31996,32092-32098",
	},
}

// approvedAdditions: own hooks and bind lines a script has beyond the
// reference on an owner decision, keyed by script path. Each entry names
// the issue that records the decision.
var approvedAdditions = map[string]struct{ hooks, binds []string }{
	// #5162: the pure unicorn frees its cell when it decays, so a quester
	// can summon it again.
	"quest.Q242_PossessorOfAPreciousSoul": {hooks: []string{"onDecayed"}, binds: []string{"bind DECAYED 31747"}},
	// #5168: the Ghost of Wigoth stops its watch timer when it decays.
	"quest.Q620_FourGoblets": {hooks: []string{"onDecayed"}, binds: []string{"bind DECAYED 31452"}},
}

// applyApprovedDivergences rewrites the reference bind lines and adds the
// hooks and bind lines an owner decision diverges with, and fails when
// such a line is gone or already there, so a stale entry cannot linger.
func applyApprovedDivergences(t *testing.T, scripts map[string]*manifestScript) {
	t.Helper()
	for path, lines := range approvedBindDivergences {
		s, ok := scripts[path]
		if !ok {
			t.Fatalf("approved divergence for %s: script not in the reference manifest", path)
		}
		for ref, port := range lines {
			i := slices.Index(s.binds, ref)
			if i < 0 {
				t.Fatalf("approved divergence for %s: reference line %q not found", path, ref)
			}
			s.binds[i] = port
		}
	}
	for path, add := range approvedAdditions {
		s, ok := scripts[path]
		if !ok {
			t.Fatalf("approved addition for %s: script not in the reference manifest", path)
		}
		for _, h := range add.hooks {
			if slices.Contains(s.hooks, h) {
				t.Fatalf("approved addition for %s: reference already has hook %s", path, h)
			}
			s.hooks = append(s.hooks, h)
		}
		for _, b := range add.binds {
			if slices.Contains(s.binds, b) {
				t.Fatalf("approved addition for %s: reference already has %q", path, b)
			}
			s.binds = append(s.binds, b)
		}
		slices.Sort(s.hooks)
		slices.Sort(s.binds)
	}
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
		case strings.HasPrefix(line, "  on ") && cur != nil:
			cur.on = append(cur.on, fields[1])
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
// manifest: not registered, kind, own hooks, player-level triggers, bind
// lines.
func manifestMismatches(t *testing.T, list []script.Listing, catalog script.Catalog, cfg script.Config) []string {
	t.Helper()
	want := readScriptManifest(t)
	applyApprovedDivergences(t, want)
	var dump strings.Builder
	if err := script.Build(list, catalog, cfg).Dump(&dump); err != nil {
		t.Fatal(err)
	}
	got := map[string]*manifestScript{}
	var out []string
	var cur *manifestScript
	var gotOther []string
	for line := range strings.Lines(dump.String()) {
		line = strings.TrimSuffix(line, "\n")
		fields := strings.Fields(line)
		switch {
		case isOtherFold(line):
			gotOther = append(gotOther, line)
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
		case strings.HasPrefix(line, "  on "):
			cur.on = append(cur.on, fields[1])
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
			if !slices.Equal(g.on, w.on) {
				out = append(out, path+": on "+strings.Join(g.on, ",")+", reference "+strings.Join(w.on, ","))
			}
			if !slices.Equal(g.binds, w.binds) {
				out = append(out, path+": binds "+strings.Join(g.binds, "; ")+", reference "+strings.Join(w.binds, "; "))
			}
			if g.scheduled != w.scheduled {
				out = append(out, fmt.Sprintf("%s: scheduled %v, reference %v", path, g.scheduled, w.scheduled))
			}
		}
	}
	var wantOther []string
	for _, line := range readOtherFolds(t) {
		for _, path := range strings.Split(strings.Fields(line)[4], ",") {
			if _, ported := catalog[path]; ported {
				wantOther = append(wantOther, line)
				break
			}
		}
	}
	slices.Sort(gotOther)
	slices.Sort(wantOther)
	if !slices.Equal(gotOther, wantOther) {
		out = append(out, fmt.Sprintf("item and zone folds %q, reference %q", gotOther, wantOther))
	}
	slices.Sort(out)
	return out
}

// isOtherFold reports a fold line of an item or a zone.
func isOtherFold(line string) bool {
	return strings.HasPrefix(line, "fold item ") || strings.HasPrefix(line, "fold zone ")
}

// readOtherFolds returns the reference manifest's item and zone fold lines.
func readOtherFolds(t *testing.T) []string {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("..", "..", "internal", "gameserver", "script", "testdata", "oracle", "manifest.golden"))
	if err != nil {
		t.Fatal(err)
	}
	var out []string
	for line := range strings.Lines(string(data)) {
		if line = strings.TrimSuffix(line, "\n"); isOtherFold(line) {
			out = append(out, line)
		}
	}
	return out
}

// TestManifestComparisonCatchesDifferences checks the comparison itself on
// stand-ins for the reference's first quest: one the seam gate refuses, as
// it refuses a script whose hook is not raised, one that registers with a
// hook the reference's lacks, and one that registers without the bindings
// the reference records.
func TestManifestComparisonCatchesDifferences(t *testing.T) {
	list := []script.Listing{{Path: "quest.Q001_LettersOfLove"}}
	stand := func(bind script.Bindings) script.Catalog {
		return script.Catalog{"quest.Q001_LettersOfLove": func() script.Script {
			return script.Script{
				QuestID: 1,
				Bind:    bind,
				Hooks:   script.Hooks{OnTalk: func(*script.Script, script.Talk) string { return "" }},
			}
		}}
	}
	q001 := stand(script.Bindings{script.EventQuestStart: {30048}, script.EventTalked: {30006, 30033, 30048}})
	folk := func(int32) (script.NPCKind, bool) { return script.KindFolk, true }
	unraised := stand(script.Bindings{script.EventPartyAttacked: {30048}})
	if got := manifestMismatches(t, list, unraised, script.Config{KindOf: folk, Log: zerolog.Nop()}); !slices.Equal(got, []string{"script quest.Q001_LettersOfLove refused"}) {
		t.Fatalf("a refused script gave %q", got)
	}
	if got := manifestMismatches(t, list, q001, script.Config{KindOf: folk, Log: zerolog.Nop()}); !slices.Equal(got, []string{"quest.Q001_LettersOfLove: own hooks onTalk, reference onAdvEvent,onTalk"}) {
		t.Fatalf("a script missing a hook gave %q, want a hook mismatch", got)
	}

	noTemplates := func(int32) (script.NPCKind, bool) { return script.KindOther, false }
	got := manifestMismatches(t, list, q001, script.Config{KindOf: noTemplates, Log: zerolog.Nop()})
	if len(got) != 2 || !strings.Contains(got[0], ": binds ") || !strings.Contains(got[1], ": own hooks onTalk, reference onAdvEvent,onTalk") {
		t.Fatalf("a script without its bindings and hooks gave %q, want a bind and a hook mismatch", got)
	}

	// A task its entry does not schedule.
	tasks := script.Catalog{"task.CastleTaxRefresh": scripttask.CastleTaxRefresh}
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
	list, err := script.LoadList(filepath.Join(root, "scripts.xml"), zerolog.Nop())
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
	joined, err := catalog.ScriptCatalog(crystals, buffs)
	if err != nil {
		t.Fatal(err)
	}
	if len(joined) == 0 {
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
	cfg := script.Config{KindOf: script.NPCKindOf(npcs), Log: zerolog.Nop(), HandleOps: catalog.HandleOps()}
	for _, m := range manifestMismatches(t, list, joined, cfg) {
		t.Error(m)
	}
}
