package main

import (
	"bufio"
	"fmt"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/script/catalog"
)

// TestScriptClaimsMatchManifest: the claims are exactly, per listed
// behavior with no Go script, its (NPC, event) bindings in the reference
// manifest that a behavior listed before it binds too. The manifest records
// the scripts in list order, and an id range as from-to.
func TestScriptClaimsMatchManifest(t *testing.T) {
	ported, err := scriptCatalog(scriptCatalogs(nil, nil))
	if err != nil {
		t.Fatal(err)
	}
	f, err := os.Open(filepath.Join("..", "..", "internal", "gameserver", "script", "testdata", "oracle", "manifest.golden"))
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	type binding struct {
		event string
		npc   int32
	}
	type listed struct {
		path     string
		behavior bool
		binds    []binding
	}
	var scripts []*listed
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 1<<20), 1<<20)
	for sc.Scan() {
		line := sc.Text()
		fields := strings.Fields(line)
		switch {
		case strings.HasPrefix(line, "script "):
			scripts = append(scripts, &listed{path: fields[1]})
		case len(scripts) == 0:
		case line == "  kind behavior":
			scripts[len(scripts)-1].behavior = true
		case strings.HasPrefix(line, "  bind "):
			cur := scripts[len(scripts)-1]
			for _, s := range strings.Split(fields[2], ",") {
				lo, hi, isRange := strings.Cut(s, "-")
				if !isRange {
					hi = lo
				}
				from, err1 := strconv.ParseInt(lo, 10, 32)
				to, err2 := strconv.ParseInt(hi, 10, 32)
				if err1 != nil || err2 != nil {
					t.Fatalf("bad id %q in %q", s, line)
				}
				for id := int32(from); id <= int32(to); id++ {
					for _, ev := range strings.Split(fields[1], ",") {
						cur.binds = append(cur.binds, binding{ev, id})
					}
				}
			}
		}
	}
	if err := sc.Err(); err != nil {
		t.Fatal(err)
	}
	if len(scripts) == 0 {
		t.Fatal("the manifest lists no script")
	}

	format := func(bs []binding) []string {
		out := make([]string, len(bs))
		for i, b := range bs {
			out[i] = fmt.Sprintf("%s %d", b.event, b.npc)
		}
		slices.Sort(out)
		return out
	}
	want := map[string][]string{}
	earlier := map[binding]bool{}
	for _, s := range scripts {
		if !s.behavior {
			continue
		}
		if _, ok := ported[s.path]; !ok {
			var taken []binding
			for _, b := range s.binds {
				if earlier[b] {
					taken = append(taken, b)
				}
			}
			if len(taken) > 0 {
				want[s.path] = format(taken)
			}
		}
		for _, b := range s.binds {
			earlier[b] = true
		}
	}
	got := map[string][]string{}
	for path, claim := range catalog.Claims() {
		var bs []binding
		for ev, ids := range claim {
			for _, id := range ids {
				bs = append(bs, binding{ev.String(), id})
			}
		}
		got[path] = format(bs)
	}
	for _, path := range slices.Sorted(maps.Keys(want)) {
		if !slices.Equal(got[path], want[path]) {
			t.Errorf("claims of %s = %q, want %q", path, got[path], want[path])
		}
	}
	for path := range got {
		if want[path] == nil {
			t.Errorf("claims of %s = %q, want none: it is ported or takes no binding", path, got[path])
		}
	}
}
