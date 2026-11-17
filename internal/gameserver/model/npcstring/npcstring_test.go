package npcstring

import (
	"bufio"
	"encoding/json"
	"os"
	"strconv"
	"strings"
	"testing"
)

// TestTableMatchesReferenceGolden compares every id and text of the table with
// testdata/npcstring.golden, a dump of the reference NpcString annotations
// (one "id<TAB>JSON string" row each, in reference order), and fails on an
// altered id, an altered text, or a changed count.
func TestTableMatchesReferenceGolden(t *testing.T) {
	f, err := os.Open("testdata/npcstring.golden")
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()

	want := make(map[int32]string, len(table))
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 0, 64*1024), 1<<20)
	for line := 1; sc.Scan(); line++ {
		idText, quoted, ok := strings.Cut(sc.Text(), "\t")
		if !ok {
			t.Fatalf("golden line %d: no tab separator", line)
		}
		id, err := strconv.ParseInt(idText, 10, 32)
		if err != nil {
			t.Fatalf("golden line %d: id: %v", line, err)
		}
		var text string
		if err := json.Unmarshal([]byte(quoted), &text); err != nil {
			t.Fatalf("golden line %d: text: %v", line, err)
		}
		if _, dup := want[int32(id)]; dup {
			t.Fatalf("golden line %d: duplicate id %d", line, id)
		}
		want[int32(id)] = text
	}
	if err := sc.Err(); err != nil {
		t.Fatal(err)
	}

	if len(table) != len(want) {
		t.Errorf("table has %d entries, golden has %d", len(table), len(want))
	}
	for id, wantText := range want {
		got, ok := Text(id)
		if !ok {
			t.Errorf("id %d: missing from table, want %q", id, wantText)
			continue
		}
		if got != wantText {
			t.Errorf("id %d: text %q, want %q", id, got, wantText)
		}
	}
	for id := range table {
		if _, ok := want[id]; !ok {
			t.Errorf("id %d: in table but not in golden", id)
		}
	}
}

func TestTextUnknownID(t *testing.T) {
	for _, id := range []int32{0, -1, 11, 99999999} {
		if text, ok := Text(id); ok || text != "" {
			t.Errorf("Text(%d) = %q, %v; want empty, false", id, text, ok)
		}
	}
}
