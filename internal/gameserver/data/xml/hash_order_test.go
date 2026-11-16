package xml

import (
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"testing"

	"github.com/rs/zerolog"
)

// readProbeLines returns the lines of the reference probe golden name
// (written by internal/commons/hashorder/testdata/oracle/run.sh), each cut
// into its kind and the rest.
func readProbeLines(t *testing.T, name string) [][2]string {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("testdata", name))
	if err != nil {
		t.Fatal(err)
	}
	var out [][2]string
	for _, line := range strings.Split(strings.TrimSuffix(string(data), "\n"), "\n") {
		kind, rest, _ := strings.Cut(line, " ")
		out = append(out, [2]string{kind, rest})
	}
	return out
}

// TestWalkerListingMatchesReferenceProbe: the shipped walker routes, loaded
// and listed, come in the order the reference route table lists them
// (testdata/walker_order.golden: route by route, each with its NPCs).
func TestWalkerListingMatchesReferenceProbe(t *testing.T) {
	t.Parallel()
	path := datapackPath(t, filepath.Join("data", "xml", "walkerRoutes.xml"))
	routes, loaded, err := LoadWalkerRoutes(path)
	if err != nil {
		t.Fatal(err)
	}
	var got []string
	for _, row := range routes.Listing(loaded) {
		if n := len(got); n > 0 && strings.HasPrefix(got[n-1], row.Route+" => ") {
			got[n-1] += "," + row.NPC
			continue
		}
		got = append(got, row.Route+" => "+row.NPC)
	}
	var want []string
	for _, line := range readProbeLines(t, "walker_order.golden") {
		want = append(want, line[1])
	}
	if len(want) == 0 || !slices.Equal(got, want) {
		t.Fatalf("walker listing:\n%s\nwant:\n%s", strings.Join(got, "\n"), strings.Join(want, "\n"))
	}
}

// TestNPCNameOrderMatchesReferenceProbe: for every name the shipped NPC
// templates share (ignoring case), the table lists them in the order the
// reference template table does and finds the first of them by name
// (testdata/npc_name_order.golden).
func TestNPCNameOrderMatchesReferenceProbe(t *testing.T) {
	t.Parallel()
	skills, err := LoadSkillDefinitions(datapackPath(t, filepath.Join("data", "xml", "skills")), zerolog.Nop())
	if err != nil {
		t.Fatal(err)
	}
	table, err := LoadNPCTemplates(datapackPath(t, filepath.Join("data", "xml", "npcs")), itemTableWithIDs(nil), skills, zerolog.Nop())
	if err != nil {
		t.Fatal(err)
	}
	byName := map[string][]int{}
	for _, tpl := range table.Listed() {
		name := strings.ToLower(tpl.Name)
		byName[name] = append(byName[name], tpl.ID)
	}
	groups := 0
	for _, line := range readProbeLines(t, "npc_name_order.golden") {
		if line[0] == "npccount" {
			if got := strconv.Itoa(table.Len()); got != line[1] {
				t.Errorf("templates = %s, want %s", got, line[1])
			}
			continue
		}
		name, ids, _ := strings.Cut(line[1], " => ")
		var want []int
		for _, f := range strings.Split(ids, ",") {
			id, err := strconv.Atoi(f)
			if err != nil {
				t.Fatal(err)
			}
			want = append(want, id)
		}
		if got := byName[name]; !slices.Equal(got, want) {
			t.Errorf("%q listed as %v, want %v", name, got, want)
		}
		if tpl, ok := table.GetByName(name); !ok || tpl.ID != want[0] {
			t.Errorf("GetByName(%q) = %v, want %d", name, tpl, want[0])
		}
		groups++
	}
	if groups == 0 {
		t.Fatal("golden lists no shared name")
	}
}
