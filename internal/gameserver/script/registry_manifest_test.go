package script_test

import (
	"bufio"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/script"
	"github.com/rs/zerolog"
)

// manifestBlock is one script block of the reference manifest, raw.
type manifestBlock struct {
	path    string
	kind    string
	questID int32
	binds   []string // "bind ..." lines, without indentation
}

// readManifestRaw returns the manifest's script blocks, in order, and its
// "fold npc" lines.
func readManifestRaw(t *testing.T) ([]manifestBlock, []string) {
	t.Helper()
	f, err := os.Open(filepath.Join("testdata", "oracle", "manifest.golden"))
	if err != nil {
		t.Fatalf("open manifest: %v", err)
	}
	defer f.Close()
	var blocks []manifestBlock
	var folds []string
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 1<<20), 1<<20)
	for sc.Scan() {
		line := sc.Text()
		fields := strings.Fields(line)
		switch {
		case strings.HasPrefix(line, "script "):
			blocks = append(blocks, manifestBlock{path: fields[1]})
		case strings.HasPrefix(line, "  kind "):
			blocks[len(blocks)-1].kind = fields[1]
		case strings.HasPrefix(line, "  quest "):
			id, err := strconv.Atoi(fields[1])
			if err != nil {
				t.Fatalf("bad quest line %q", line)
			}
			blocks[len(blocks)-1].questID = int32(id)
		case strings.HasPrefix(line, "  bind "):
			blocks[len(blocks)-1].binds = append(blocks[len(blocks)-1].binds, strings.TrimSpace(line))
		case strings.HasPrefix(line, "fold npc "):
			folds = append(folds, line)
		}
	}
	if err := sc.Err(); err != nil {
		t.Fatalf("read manifest: %v", err)
	}
	return blocks, folds
}

func eventByName(t *testing.T) map[string]script.NPCEvent {
	t.Helper()
	out := map[string]script.NPCEvent{}
	for ev := range script.NPCEventCount {
		out[ev.String()] = ev
	}
	return out
}

// TestRegistryReplaysManifest registers every script of the reference
// manifest, with the bindings the reference recorded for it, through the
// engine registry, and requires the registry dump to reproduce the
// manifest: each script's bind lines and every folded per-(NPC, event)
// list, in the manifest's own layout.
func TestRegistryReplaysManifest(t *testing.T) {
	blocks, folds := readManifestRaw(t)
	events := eventByName(t)

	list := make([]script.Listing, len(blocks))
	catalog := script.Catalog{}
	templates := map[int32]bool{}
	for i, b := range blocks {
		list[i] = script.Listing{Path: b.path}
		bind := script.Bindings{}
		for _, line := range b.binds {
			fields := strings.Fields(line)
			for _, name := range strings.Split(fields[1], ",") {
				ev, ok := events[name]
				if !ok {
					t.Fatalf("%s binds unknown event %s", b.path, name)
				}
				for _, id := range expandIDs(t, fields[2]) {
					bind[ev] = append(bind[ev], int32(id))
					templates[int32(id)] = true
				}
			}
		}
		s := script.Script{Behavior: b.kind == "behavior", Bind: bind}
		if b.kind == "quest" {
			s.QuestID = b.questID
		}
		catalog[b.path] = func() script.Script { return s }
	}
	kindOf := func(id int32) (script.NPCKind, bool) { return script.KindHostile, templates[id] }
	r := script.Build(list, catalog, script.RaiseAll(script.Config{KindOf: kindOf, Log: zerolog.Nop()}))

	var dump strings.Builder
	if err := r.Dump(&dump); err != nil {
		t.Fatal(err)
	}
	gotBinds := map[string][]string{}
	var gotFolds []string
	var cur string
	for line := range strings.Lines(dump.String()) {
		line = strings.TrimSuffix(line, "\n")
		switch {
		case strings.HasPrefix(line, "script "):
			cur = strings.Fields(line)[1]
			if strings.HasSuffix(line, " missing") || strings.HasSuffix(line, " refused") {
				t.Errorf("dump: %s", line)
			}
		case strings.HasPrefix(line, "  bind "):
			gotBinds[cur] = append(gotBinds[cur], strings.TrimSpace(line))
		case strings.HasPrefix(line, "fold npc "):
			gotFolds = append(gotFolds, line)
		}
	}
	for _, b := range blocks {
		if !slices.Equal(gotBinds[b.path], b.binds) {
			t.Errorf("%s binds:\n got %q\nwant %q", b.path, gotBinds[b.path], b.binds)
		}
	}
	if !slices.Equal(gotFolds, folds) {
		for i := range max(len(gotFolds), len(folds)) {
			var g, w string
			if i < len(gotFolds) {
				g = gotFolds[i]
			}
			if i < len(folds) {
				w = folds[i]
			}
			if g != w {
				t.Errorf("fold line %d:\n got %q\nwant %q", i, g, w)
				break
			}
		}
		t.Fatalf("folded lists differ: %d lines, reference %d", len(gotFolds), len(folds))
	}
}
