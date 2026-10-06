package q001

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

const path = "quest.Q001_LettersOfLove"

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

// TestPagesAreInTheDatapack requires every page the quest names to be a
// page of its datapack directory.
func TestPagesAreInTheDatapack(t *testing.T) {
	idx, err := scriptpages.Load()
	if err != nil {
		t.Fatal(err)
	}
	problems, err := scriptpages.CheckPackage(idx, ".", "quest/Q001_LettersOfLove")
	if err != nil {
		t.Fatal(err)
	}
	for _, p := range problems {
		t.Error(p)
	}
}

// TestRegistryMatchesManifest registers the quest from its scripts.xml
// path, with Darin, Roxxy and Baulro as civilian templates, and requires
// the registry dump to carry the reference manifest's kind, bindings and
// hooks for it, and the script the reference's quest id, title and items.
func TestRegistryMatchesManifest(t *testing.T) {
	want := manifestBlock(t)
	kindOf := func(id int32) (script.NPCKind, bool) {
		return script.KindFolk, id == darin || id == roxxy || id == baulro
	}
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
	wantDump := []string{"script " + path, "  kind quest", "  hooks " + strings.Join(want.hooks, ",")}
	wantDump = append(wantDump, want.binds...)
	if !slices.Equal(got, wantDump) {
		t.Fatalf("dump:\n got %q\nwant %q", got, wantDump)
	}

	q, ok := r.JournalQuest("Q001_LettersOfLove")
	if !ok {
		t.Fatal("quest not registered under its name")
	}
	s := New()
	if line := "  quest 1 name " + q.Name + " descr " + s.Title; line != want.quest {
		t.Errorf("quest line %q, want %q", line, want.quest)
	}
	var items []string
	for _, id := range q.Items {
		items = append(items, strconv.Itoa(int(id)))
	}
	if line := "  items " + strings.Join(items, ","); line != want.items {
		t.Errorf("items line %q, want %q", line, want.items)
	}
}

type block struct {
	quest, items string
	binds        []string
	hooks        []string
}

// manifestBlock reads the quest's script block and class hooks from the
// reference probe's manifest.
func manifestBlock(t *testing.T) block {
	t.Helper()
	f, err := os.Open(filepath.Join("..", "..", "testdata", "oracle", "manifest.golden"))
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	var b block
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
		case in && strings.HasPrefix(line, "  quest "):
			b.quest = line
		case in && strings.HasPrefix(line, "  items "):
			b.items = line
		case in && strings.HasPrefix(line, "  bind "):
			b.binds = append(b.binds, line)
		case inClass && strings.HasPrefix(line, "  hook "):
			name, _, _ := strings.Cut(strings.TrimPrefix(line, "  hook "), "(")
			b.hooks = append(b.hooks, name)
		}
	}
	if err := sc.Err(); err != nil {
		t.Fatal(err)
	}
	if b.quest == "" || len(b.binds) == 0 || len(b.hooks) == 0 {
		t.Fatalf("manifest has no complete block for %s: %+v", path, b)
	}
	slices.Sort(b.hooks)
	return b
}
