package main

import (
	"bufio"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

// manifestChains reads the class chain of every script of the reference
// manifest, by path, as simple class names, own class first.
func manifestChains(t *testing.T) map[string][]string {
	t.Helper()
	f, err := os.Open(filepath.Join("..", "..", "internal", "gameserver", "script", "testdata", "oracle", "manifest.golden"))
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	out := map[string][]string{}
	var cur string
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 1<<20), 1<<20)
	for sc.Scan() {
		line := sc.Text()
		switch {
		case strings.HasPrefix(line, "script "):
			cur = strings.Fields(line)[1]
		case strings.HasPrefix(line, "  chain "):
			var names []string
			for _, class := range strings.Split(strings.TrimPrefix(line, "  chain "), " < ") {
				names = append(names, class[strings.LastIndexByte(class, '.')+1:])
			}
			out[cur] = names
		}
	}
	if err := sc.Err(); err != nil {
		t.Fatal(err)
	}
	return out
}

// TestScriptClassesMatchReferenceChains pins the class chain of every
// catalog script the //info AI page can print, to the reference's: a class
// a script derives from must name its hooks. The page prints the first
// script bound to an NPC that is not a real quest; a scheduled task binds
// to no NPC.
func TestScriptClassesMatchReferenceChains(t *testing.T) {
	want := manifestChains(t)
	catalog, err := scriptCatalog(scriptCatalogs(nil, nil))
	if err != nil {
		t.Fatal(err)
	}
	for path, ctor := range catalog {
		if strings.HasPrefix(path, "quest.") {
			continue
		}
		s := ctor()
		if s.QuestID > 0 || s.OnStart != nil {
			continue
		}
		s.Name = path[strings.LastIndexByte(path, '.')+1:]
		if got := s.Classes(); !slices.Equal(got, want[path]) {
			t.Errorf("%s classes = %q, reference %q", path, got, want[path])
		}
	}
}
