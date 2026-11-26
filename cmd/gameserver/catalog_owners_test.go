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
	"github.com/fatal10110/acis_golang/internal/gameserver/script/quest/thirdclass"
	"github.com/fatal10110/acis_golang/internal/testsupport/datapack"
)

// TestScriptCatalogOwners: every script of the reference manifest (the
// scripts.xml listing) has exactly one owner, one catalog file, and no
// catalog holds a script the manifest does not list, but for the tasks
// scripts.xml lists only in a comment (commentedTasks).
func TestScriptCatalogOwners(t *testing.T) {
	want := append(slices.Collect(maps.Keys(readScriptManifest(t))), commentedTasks...)
	checkOwners(t, "script", want, catalog.Scripts(nil, nil, thirdclass.NewFamily()))
}

// TestMakerCatalogOwners: every npcmaker type the reference gives a maker of
// its own has exactly one owner, one catalog file, and no catalog holds
// another type.
func TestMakerCatalogOwners(t *testing.T) {
	checkOwners(t, "maker type", readMakerTypes(t), catalog.Makers())
}

// checkOwners fails on every owner problem.
func checkOwners[C ~map[string]func() T, T any](t *testing.T, what string, want []string, owned map[string]C) {
	t.Helper()
	for _, p := range ownerProblems(what, want, owned) {
		t.Error(p)
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
// module and skips without it, or fails as datapack.Reference decides.
func TestMakerTypesMatchReference(t *testing.T) {
	const npcMaker = "java/net/sf/l2j/gameserver/model/spawn/NpcMaker.java"
	src, err := os.ReadFile(filepath.Join(datapack.Reference(t, npcMaker), npcMaker))
	if err != nil {
		t.Fatal(err)
	}
	var want []string
	for _, m := range regexp.MustCompile(`case "([^"]+)":`).FindAllSubmatch(src, -1) {
		want = append(want, string(m[1]))
	}
	if got := readMakerTypes(t); !slices.Equal(got, want) {
		t.Fatalf("maker_types.golden lists %q, the reference switch %q", got, want)
	}
}
