package fortressofthedead

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

const path = "script.siegablehall.FortressOfTheDead"

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

// manifestLines returns, from a manifest-layout text, the kind and bind
// lines of the script and, per event and NPC id, each fold naming it.
func manifestLines(t *testing.T, sc *bufio.Scanner) []string {
	t.Helper()
	var out []string
	var cur string
	for sc.Scan() {
		line := sc.Text()
		fields := strings.Fields(line)
		switch {
		case strings.HasPrefix(line, "script "):
			cur = fields[1]
		case cur == path && (strings.HasPrefix(line, "  kind ") || strings.HasPrefix(line, "  bind ")):
			out = append(out, strings.TrimSpace(line))
		case strings.HasPrefix(line, "fold npc ") && slices.Contains(strings.Split(fields[3], ","), path):
			for _, ev := range strings.Split(fields[2], ",") {
				for _, id := range expandIDs(t, fields[4]) {
					out = append(out, "fold "+ev+" "+strconv.Itoa(id))
				}
			}
		}
	}
	if err := sc.Err(); err != nil {
		t.Fatal(err)
	}
	slices.Sort(out)
	return out
}

// TestRegistrationMatchesReferenceManifest registers the script through
// the engine registry, Lidia, her kin and her vampires monsters as the
// datapack types them. It is not refused, and the registry dump equals the
// reference manifest for it: its kind, its bind lines (Lidia's attacks,
// the appearance, death and party deaths of Lidia and her kin, and every
// defender's idleness) and the per-(npc, event) lists of it alone.
func TestRegistrationMatchesReferenceManifest(t *testing.T) {
	kindOf := func(int32) (script.NPCKind, bool) { return script.KindHostile, true }
	r := script.Build([]script.Listing{{Path: path}}, script.Catalog{path: New}, script.Config{KindOf: kindOf, Log: zerolog.Nop()})
	var dump strings.Builder
	if err := r.Dump(&dump); err != nil {
		t.Fatal(err)
	}
	got := manifestLines(t, bufio.NewScanner(strings.NewReader(dump.String())))

	f, err := os.Open(filepath.Join("..", "..", "testdata", "oracle", "manifest.golden"))
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 1<<20), 1<<20)
	want := manifestLines(t, sc)
	if len(want) == 0 {
		t.Fatal("the reference manifest lists nothing of " + path)
	}
	if !slices.Equal(got, want) {
		t.Errorf("registration:\n got %q\nwant %q", got, want)
	}
}
