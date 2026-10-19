package main

import (
	"bufio"
	"maps"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/script"
	"github.com/fatal10110/acis_golang/internal/gameserver/script/catalog"
	"github.com/fatal10110/acis_golang/internal/gameserver/script/maker"
	"github.com/fatal10110/acis_golang/internal/gameserver/script/quest/thirdclass"
)

// engineOwner names the catalogs of the scripts and makers ported with the
// engine, in the owner reports.
const engineOwner = "engine"

// TestScriptCatalogOwners: every script of the reference manifest (the
// scripts.xml listing) has exactly one owner, the catalogs of scripts.go or
// one content catalog file, and no catalog holds a script the manifest does
// not list, but for the tasks scripts.xml lists only in a comment
// (commentedTasks). It logs the scripts not ported yet, by catalog file.
func TestScriptCatalogOwners(t *testing.T) {
	sagas := thirdclass.NewFamily()
	owned := map[string]script.Catalog{engineOwner: {}}
	for _, c := range engineCatalogs(nil, nil, sagas) {
		maps.Copy(owned[engineOwner], c)
	}
	maps.Copy(owned, catalog.Scripts(sagas))
	want := append(slices.Collect(maps.Keys(readScriptManifest(t))), commentedTasks...)
	checkOwners(t, "script", want, owned)
}

// TestMakerCatalogOwners: every npcmaker type the reference gives a maker of
// its own has exactly one owner, the maker package or one content catalog
// file, and no catalog holds another type. It logs the types not ported
// yet, by catalog file.
func TestMakerCatalogOwners(t *testing.T) {
	owned := map[string]script.MakerCatalog{engineOwner: maker.Catalog()}
	maps.Copy(owned, catalog.Makers())
	checkOwners(t, "maker type", readMakerTypes(t), owned)
}

// checkOwners fails on every owner problem and logs, by owner, the keys
// not ported yet (a nil constructor).
func checkOwners[C ~map[string]func() T, T any](t *testing.T, what string, want []string, owned map[string]C) {
	t.Helper()
	for _, p := range ownerProblems(what, want, owned) {
		t.Error(p)
	}
	for _, name := range slices.Sorted(maps.Keys(owned)) {
		var unported []string
		for key, ctor := range owned[name] {
			if ctor == nil {
				unported = append(unported, key)
			}
		}
		if len(unported) == 0 {
			continue
		}
		slices.Sort(unported)
		t.Logf("%s: %d of %d not ported yet", name, len(unported), len(owned[name]))
		for _, key := range unported {
			t.Logf("  %s", key)
		}
	}
}

// ownerProblems returns a key of want with no owner or with two, and an
// owned key want lacks.
func ownerProblems[C ~map[string]func() T, T any](what string, want []string, owned map[string]C) []string {
	owners := map[string][]string{}
	for _, name := range slices.Sorted(maps.Keys(owned)) {
		for key := range owned[name] {
			owners[key] = append(owners[key], name)
		}
	}
	var out []string
	listed := map[string]bool{}
	for _, key := range want {
		listed[key] = true
		switch o := owners[key]; len(o) {
		case 0:
			out = append(out, what+" "+key+" has no catalog file")
		case 1:
		default:
			out = append(out, what+" "+key+" is in catalogs "+strings.Join(o, ", "))
		}
	}
	for _, key := range slices.Sorted(maps.Keys(owners)) {
		if !listed[key] {
			out = append(out, what+" "+key+" of catalogs "+strings.Join(owners[key], ", ")+" is not a reference "+what)
		}
	}
	return out
}

// TestOwnerProblemsCatchesMistakes checks the owner check on catalogs with
// an unowned key, a key in two files and a key the reference lacks.
func TestOwnerProblemsCatchesMistakes(t *testing.T) {
	ctor := func() script.Maker { return script.Maker{} }
	for _, tc := range []struct {
		owned map[string]script.MakerCatalog
		want  []string
	}{
		{map[string]script.MakerCatalog{"a": {"x": ctor}, "b": {"y": nil}}, nil},
		{map[string]script.MakerCatalog{"a": {"x": ctor}}, []string{"maker type y has no catalog file"}},
		{map[string]script.MakerCatalog{"a": {"x": ctor, "y": nil}, "b": {"y": nil}}, []string{"maker type y is in catalogs a, b"}},
		{map[string]script.MakerCatalog{"a": {"x": ctor, "y": nil, "z": nil}}, []string{"maker type z of catalogs a is not a reference maker type"}},
	} {
		if got := ownerProblems("maker type", []string{"x", "y"}, tc.owned); !slices.Equal(got, tc.want) {
			t.Errorf("owned %v: problems %q, want %q", tc.owned, got, tc.want)
		}
	}
}

// TestScriptCatalogDropsUnported: the join leaves out the entries not
// ported yet, and still rejects one in two catalogs.
func TestScriptCatalogDropsUnported(t *testing.T) {
	ctor := func() script.Script { return script.Script{} }
	got, err := scriptCatalog([]script.Catalog{{"quest.A": ctor}, {"quest.B": nil}})
	if err != nil {
		t.Fatal(err)
	}
	if keys := slices.Sorted(maps.Keys(got)); !slices.Equal(keys, []string{"quest.A"}) {
		t.Fatalf("joined %v, want only the ported quest.A", keys)
	}
	if _, err := scriptCatalog([]script.Catalog{{"quest.A": ctor}, {"quest.A": nil}}); err == nil {
		t.Fatal("a path ported in one catalog and not in another joined")
	}
	if _, err := makerCatalog([]script.MakerCatalog{{"a_maker": nil}, {"a_maker": nil}}); err == nil {
		t.Fatal("a maker type in two catalogs joined")
	}
}

// readMakerTypes reads the reference's maker types from the golden.
func readMakerTypes(t *testing.T) []string {
	t.Helper()
	f, err := os.Open(filepath.Join("testdata", "maker_types.golden"))
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	var out []string
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		if line := strings.TrimSpace(sc.Text()); line != "" && !strings.HasPrefix(line, "#") {
			out = append(out, line)
		}
	}
	if err := sc.Err(); err != nil {
		t.Fatal(err)
	}
	return out
}

// TestMakerTypesMatchReference rebuilds the maker types golden from the
// reference's maker switch. It needs the reference source beside the
// module and skips without it, or fails when ACIS_REQUIRE_DATAPACK is set.
func TestMakerTypesMatchReference(t *testing.T) {
	const npcMaker = "aCis_gameserver/java/net/sf/l2j/gameserver/model/spawn/NpcMaker.java"
	var src []byte
	dir, err := filepath.Abs(".")
	if err != nil {
		t.Fatal(err)
	}
	for ; src == nil; dir = filepath.Dir(dir) {
		for _, candidate := range []string{filepath.Join(dir, npcMaker), filepath.Join(dir, "acis_public", npcMaker)} {
			if b, err := os.ReadFile(candidate); err == nil {
				src = b
				break
			}
		}
		if src == nil && filepath.Dir(dir) == dir {
			if os.Getenv("ACIS_REQUIRE_DATAPACK") != "" {
				t.Fatal("reference source not found above the module root and ACIS_REQUIRE_DATAPACK is set")
			}
			t.Skip("reference source not checked out near the module root (set ACIS_REQUIRE_DATAPACK=1 to fail instead)")
		}
	}
	var want []string
	for _, m := range regexp.MustCompile(`case "([^"]+)":`).FindAllSubmatch(src, -1) {
		want = append(want, string(m[1]))
	}
	if got := readMakerTypes(t); !slices.Equal(got, want) {
		t.Fatalf("maker_types.golden lists %q, the reference switch %q", got, want)
	}
}
