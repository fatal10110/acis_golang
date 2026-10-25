package agitwarrior

import (
	"bufio"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/script"
	"github.com/fatal10110/acis_golang/internal/gameserver/script/ai/defaultnpc"
	"github.com/rs/zerolog"
)

// chain is the behavior chain down to the behaviors this package
// ports, in scripts.xml order, with the Go constructor of each.
var chain = []struct {
	path string
	ctor func() script.Script
}{
	{"script.ai.individual.DefaultNpc", defaultnpc.New},
	{"script.ai.individual.AgitWarrior.AgitWarrior", Warrior},
	{"script.ai.individual.AgitWarrior.AgitWarriorFlee.AgitThief", Thief},
	{"script.ai.individual.AgitWarrior.AgitWarriorFlee.AgitWarriorFlee", WarriorFlee},
	{"script.ai.individual.AgitWarrior.AgitWarriorFlee.AgitWizardFlee", WizardFlee},
	{"script.ai.individual.AgitWarrior.AgitWizard", Wizard},
}

// manifestView is what the reference manifest records of the chain: per
// script its kind and bind lines, and the folded per-(npc, event) lists
// restricted to the chain's scripts, as "event id paths" triples.
type manifestView struct {
	kinds map[string]string
	binds map[string][]string
	folds []string
}

// expandIDs reads a manifest id list: ids and a-b runs, comma-separated.
func expandIDs(t *testing.T, s string) []int {
	t.Helper()
	var out []int
	for _, part := range strings.Split(s, ",") {
		lo, hi, isRange := strings.Cut(part, "-")
		a, err := strconv.Atoi(lo)
		if err != nil {
			t.Fatalf("bad id %q", part)
		}
		b := a
		if isRange {
			if b, err = strconv.Atoi(hi); err != nil {
				t.Fatalf("bad id %q", part)
			}
		}
		for id := a; id <= b; id++ {
			out = append(out, id)
		}
	}
	return out
}

// readView reads a manifest-layout text (the reference manifest or a
// registry dump) as the chain sees it.
func readView(t *testing.T, path string, text *bufio.Scanner) manifestView {
	t.Helper()
	ours := map[string]bool{}
	for _, c := range chain {
		ours[c.path] = true
	}
	v := manifestView{kinds: map[string]string{}, binds: map[string][]string{}}
	var cur string
	for text.Scan() {
		line := text.Text()
		fields := strings.Fields(line)
		switch {
		case strings.HasPrefix(line, "script "):
			cur = fields[1]
			if ours[cur] && len(fields) > 2 {
				t.Fatalf("%s: %s", path, line)
			}
		case strings.HasPrefix(line, "  kind ") && ours[cur]:
			v.kinds[cur] = fields[1]
		case strings.HasPrefix(line, "  bind ") && ours[cur]:
			v.binds[cur] = append(v.binds[cur], strings.TrimSpace(line))
		case strings.HasPrefix(line, "fold npc "):
			var kept []string
			for _, p := range strings.Split(fields[3], ",") {
				if ours[p] {
					kept = append(kept, p)
				}
			}
			if len(kept) == 0 {
				continue
			}
			for _, ev := range strings.Split(fields[2], ",") {
				for _, id := range expandIDs(t, fields[4]) {
					v.folds = append(v.folds, ev+" "+strconv.Itoa(id)+" "+strings.Join(kept, ","))
				}
			}
		}
	}
	if err := text.Err(); err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	slices.Sort(v.folds)
	return v
}

// TestRegistrationMatchesReferenceManifest registers the chain's Go
// scripts through the engine registry, with the engine's own seam gate and
// each id of the kind the datapack's type gives it: the Oel Mahums
// (Monster, 35428-35432) hostile, the frenzied beasts (Folk, 35618-35622)
// civilians. None is refused, and the registry dump equals the reference
// manifest for the chain: each script's kind and bind lines, and every
// folded per-(npc, event) list restricted to the chain's scripts: the 10
// ids on 4 events each, and the 6 ids of the flee classes on created.
func TestRegistrationMatchesReferenceManifest(t *testing.T) {
	list := make([]script.Listing, len(chain))
	catalog := script.Catalog{}
	for i, c := range chain {
		list[i] = script.Listing{Path: c.path}
		catalog[c.path] = c.ctor
	}
	kindOf := func(id int32) (script.NPCKind, bool) {
		if id >= 35618 && id <= 35622 {
			return script.KindFolk, true
		}
		return script.KindHostile, true
	}
	r := script.Build(list, catalog, script.Config{KindOf: kindOf, Log: zerolog.Nop()})
	var dump strings.Builder
	if err := r.Dump(&dump); err != nil {
		t.Fatal(err)
	}
	got := readView(t, "dump", bufio.NewScanner(strings.NewReader(dump.String())))

	path := filepath.Join("..", "..", "testdata", "oracle", "manifest.golden")
	f, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 1<<20), 1<<20)
	want := readView(t, path, sc)

	for _, c := range chain {
		if want.kinds[c.path] == "" {
			t.Fatalf("%s is not in the reference manifest", c.path)
		}
		if got.kinds[c.path] != want.kinds[c.path] {
			t.Errorf("%s kind = %q, want %q", c.path, got.kinds[c.path], want.kinds[c.path])
		}
		if !slices.Equal(got.binds[c.path], want.binds[c.path]) {
			t.Errorf("%s binds:\n got %q\nwant %q", c.path, got.binds[c.path], want.binds[c.path])
		}
	}
	if wantFolds, got := 10*4+6, len(want.folds); got != wantFolds {
		t.Fatalf("reference folds = %d entries, want %d: the 10 ids on 4 events each, and the flee classes' 6 on created", got, wantFolds)
	}
	if !slices.Equal(got.folds, want.folds) {
		t.Errorf("folded lists differ: %d entries, reference %d", len(got.folds), len(want.folds))
	}
	for _, id := range []int32{35428, 35429, 35430, 35431, 35432, 35618, 35619, 35620, 35621, 35622} {
		if !r.Behaves(id) {
			t.Errorf("Behaves(%d) = false, want a behavior bound", id)
		}
	}
}
