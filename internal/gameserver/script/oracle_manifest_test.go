package script_test

import (
	"bufio"
	"encoding/xml"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"testing"

	"github.com/fatal10110/acis_golang/internal/testsupport/datapack"
)

// The reference probe's registration manifest (testdata/oracle/README.md). These tests
// check that it is complete and self-consistent; they do not run the probe.

type manifestScript struct {
	path     string
	kind     string
	missing  bool
	bindings map[npcEvent]bool
}

type npcEvent struct {
	npc   int
	event string
}

type manifest struct {
	header  map[string]string
	scripts []manifestScript
	folded  map[npcEvent][]string
}

func readManifest(t *testing.T) manifest {
	t.Helper()
	f, err := os.Open(filepath.Join("testdata", "oracle", "manifest.golden"))
	if err != nil {
		t.Fatalf("open manifest: %v", err)
	}
	defer f.Close()

	m := manifest{header: map[string]string{}, folded: map[npcEvent][]string{}}
	var cur *manifestScript
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 1<<20), 1<<20)
	for sc.Scan() {
		line := sc.Text()
		fields := strings.Fields(line)
		switch {
		case len(fields) == 0 || strings.HasPrefix(line, "#"):
		case fields[0] == "listed":
			for i := 0; i+1 < len(fields); i += 2 {
				m.header[fields[i]] = fields[i+1]
			}
		case fields[0] == "paths" || fields[0] == "kinds":
			for i := 1; i+1 < len(fields); i += 2 {
				m.header[fields[0]+" "+fields[i]] = fields[i+1]
			}
		case fields[0] == "script":
			m.scripts = append(m.scripts, manifestScript{
				path: fields[1], missing: len(fields) > 2 && fields[2] == "missing", bindings: map[npcEvent]bool{},
			})
			cur = &m.scripts[len(m.scripts)-1]
		case fields[0] == "kind":
			cur.kind = fields[1]
		case fields[0] == "bind":
			for _, ev := range strings.Split(fields[1], ",") {
				for _, id := range expandIDs(t, fields[2]) {
					cur.bindings[npcEvent{id, ev}] = true
				}
			}
		case fields[0] == "fold" && fields[1] == "npc":
			scripts := strings.Split(fields[3], ",")
			for _, ev := range strings.Split(fields[2], ",") {
				for _, id := range expandIDs(t, fields[4]) {
					k := npcEvent{id, ev}
					if _, dup := m.folded[k]; dup {
						t.Fatalf("folded list for npc %d %s written twice", id, ev)
					}
					m.folded[k] = scripts
				}
			}
		}
	}
	if err := sc.Err(); err != nil {
		t.Fatalf("read manifest: %v", err)
	}
	return m
}

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
				t.Fatalf("bad id range %q", part)
			}
		}
		for id := a; id <= b; id++ {
			out = append(out, id)
		}
	}
	return out
}

// TestManifestListsEveryScript checks the manifest against scripts.xml: the same paths in
// the same order, every one built, and the live set the engine plan counts.
func TestManifestListsEveryScript(t *testing.T) {
	m := readManifest(t)

	raw, err := os.ReadFile(datapack.Path(t, "data", "xml", "scripts.xml"))
	if err != nil {
		t.Fatalf("read scripts.xml: %v", err)
	}
	var doc struct {
		Scripts []struct {
			Path string `xml:"path,attr"`
		} `xml:"script"`
	}
	if err := xml.Unmarshal(raw, &doc); err != nil {
		t.Fatalf("parse scripts.xml: %v", err)
	}
	var listed, got []string
	for _, s := range doc.Scripts {
		listed = append(listed, s.Path)
	}
	for _, s := range m.scripts {
		got = append(got, s.path)
		if s.missing {
			t.Errorf("script %s missing from the probe run", s.path)
		}
	}
	if !slices.Equal(got, listed) {
		t.Fatalf("manifest scripts differ from scripts.xml: %d vs %d entries", len(got), len(listed))
	}

	for key, want := range map[string]string{
		"listed": "857", "loaded": "857",
		"paths quest.*": "342", "paths script.*": "509", "paths task.*": "6",
	} {
		if got := m.header[key]; got != want {
			t.Errorf("header %q = %q, want %q", key, got, want)
		}
	}
}

// TestManifestFoldFollowsRegistrationRules replays every script's bindings, in
// scripts.xml order, through the registry rules of the engine plan (section 2) and
// requires the result to equal the folded lists the reference produced:
//   - a behavior registering on (npc, event) first removes any behavior there, so one
//     behavior per (npc, event) remains and the last listed wins;
//   - other scripts accumulate in list order;
//   - FIRST_TALK keeps one script: an occupied slot refuses a newcomer unless both are
//     behaviors.
func TestManifestFoldFollowsRegistrationRules(t *testing.T) {
	m := readManifest(t)

	kind := map[string]string{}
	derived := map[npcEvent][]string{}
	for _, s := range m.scripts {
		if s.missing {
			continue
		}
		kind[s.path] = s.kind
		keys := make([]npcEvent, 0, len(s.bindings))
		for k := range s.bindings {
			keys = append(keys, k)
		}
		for _, k := range keys {
			list := derived[k]
			if s.kind == "behavior" {
				if i := slices.IndexFunc(list, func(p string) bool { return kind[p] == "behavior" }); i >= 0 {
					list = slices.Delete(slices.Clone(list), i, i+1)
				}
			} else if i := slices.Index(list, s.path); i >= 0 {
				list = slices.Delete(slices.Clone(list), i, i+1)
			}
			if k.event != "FIRST_TALK" || len(list) == 0 {
				list = append(slices.Clone(list), s.path)
			}
			derived[k] = list
		}
	}

	if len(derived) != len(m.folded) {
		t.Errorf("derived %d (npc, event) lists, reference folded %d", len(derived), len(m.folded))
	}
	for k, want := range m.folded {
		if got := derived[k]; !slices.Equal(got, want) {
			t.Errorf("npc %d %s: derived %v, reference %v", k.npc, k.event, got, want)
		}
	}
	for k, got := range derived {
		if _, ok := m.folded[k]; !ok {
			t.Errorf("npc %d %s: derived %v, reference has no list", k.npc, k.event, got)
		}
	}
}
