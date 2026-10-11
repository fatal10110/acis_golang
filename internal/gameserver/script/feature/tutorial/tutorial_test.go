package tutorial

import (
	"bufio"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"testing"

	"github.com/rs/zerolog"

	"github.com/fatal10110/acis_golang/internal/gameserver/script"
	"github.com/fatal10110/acis_golang/internal/testsupport/scriptfp"
	"github.com/fatal10110/acis_golang/internal/testsupport/scriptpages"
)

const path = "script.feature.Tutorial"

// TestMatchesReferenceFingerprints compares the package with the reference
// class: literals, engine calls and hook shapes.
func TestMatchesReferenceFingerprints(t *testing.T) {
	problems, err := scriptfp.Check(".", []string{path})
	if err != nil {
		t.Fatal(err)
	}
	for _, p := range problems {
		t.Error(p)
	}
}

// TestPagesAreInTheDatapack requires every page the tutorial names to be a
// page of its datapack directory, except three the datapack does not ship:
// the tutorial window shows its missing-page notice for those.
func TestPagesAreInTheDatapack(t *testing.T) {
	idx, err := scriptpages.Load()
	if err != nil {
		t.Fatal(err)
	}
	problems, err := scriptpages.CheckPackage(idx, ".", "feature/Tutorial", "tutorial_07.htm", "tutorial_07a.htm", "tutorial_14.htm")
	if err != nil {
		t.Fatal(err)
	}
	for _, p := range problems {
		t.Error(p)
	}
}

// TestRegistryMatchesManifest registers the tutorial from its scripts.xml
// path and requires the registry dump to carry the reference manifest's
// kind and the reference class's hooks, with no binding, and the script
// the reference's quest id and description.
func TestRegistryMatchesManifest(t *testing.T) {
	quest, hooks := manifestBlock(t)
	kindOf := func(int32) (script.NPCKind, bool) { return script.KindOther, false }
	r := script.Build([]script.Listing{{Path: path}}, script.Catalog{path: New}, script.Config{KindOf: kindOf, Log: zerolog.Nop()})
	var dump strings.Builder
	if err := r.Dump(&dump); err != nil {
		t.Fatal(err)
	}
	var got []string
	for line := range strings.Lines(dump.String()) {
		line = strings.TrimRight(line, "\n")
		if line == "" {
			break
		}
		got = append(got, line)
	}
	want := []string{"script " + path, "  kind script", "  hooks " + strings.Join(hooks, ",")}
	if !slices.Equal(got, want) {
		t.Fatalf("dump:\n got %q\nwant %q", got, want)
	}

	q, ok := r.JournalQuest("Tutorial")
	if !ok {
		t.Fatal("tutorial not registered under its name")
	}
	if line := "  quest " + strconv.Itoa(int(q.ID)) + " name " + q.Name + " descr " + New().Dir; line != quest {
		t.Errorf("quest line %q, want %q", line, quest)
	}
}

// manifestBlock reads the tutorial's quest line and class hooks from the
// reference probe's manifest.
func manifestBlock(t *testing.T) (quest string, hooks []string) {
	t.Helper()
	f, err := os.Open(filepath.Join("..", "..", "testdata", "oracle", "manifest.golden"))
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	var in, inClass bool
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 1<<20), 1<<20)
	for sc.Scan() {
		line := sc.Text()
		switch {
		case strings.HasPrefix(line, "script "):
			in = line == "script "+path
		case strings.HasPrefix(line, "class "):
			inClass = strings.HasPrefix(line, "class "+path+" ")
		case in && strings.HasPrefix(line, "  bind "):
			t.Fatalf("manifest binds the tutorial: %s", line)
		case in && strings.HasPrefix(line, "  quest "):
			quest = line
		case inClass && strings.HasPrefix(line, "  hook "):
			name, _, _ := strings.Cut(strings.TrimPrefix(line, "  hook "), "(")
			hooks = append(hooks, name)
		}
	}
	if err := sc.Err(); err != nil {
		t.Fatal(err)
	}
	if quest == "" || len(hooks) == 0 {
		t.Fatalf("manifest has no complete block for %s", path)
	}
	slices.Sort(hooks)
	return quest, hooks
}
