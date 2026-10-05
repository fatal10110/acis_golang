package xml

import (
	"bufio"
	"bytes"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/script"
	"github.com/rs/zerolog"
)

// TestLoadScriptList pins the datapack's scripts.xml: 857 live entries in
// the order of the reference's registration manifest, and every schedule
// attribute.
func TestLoadScriptList(t *testing.T) {
	t.Parallel()
	list, err := LoadScriptList(datapackPath(t, filepath.Join("data", "xml", "scripts.xml")), zerolog.Nop())
	if err != nil {
		t.Fatal(err)
	}

	f, err := os.Open(filepath.Join("..", "..", "script", "testdata", "oracle", "manifest.golden"))
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	var want []string
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 1<<20), 1<<20)
	for sc.Scan() {
		if path, ok := strings.CutPrefix(sc.Text(), "script "); ok {
			want = append(want, strings.Fields(path)[0])
		}
	}
	if err := sc.Err(); err != nil {
		t.Fatal(err)
	}

	var got []string
	scheduled := map[string]script.Listing{}
	for _, l := range list {
		got = append(got, l.Path)
		if l.Schedule != "" || l.Start != "" || l.End != "" {
			scheduled[l.Path] = l
		}
	}
	if len(got) != 857 {
		t.Fatalf("listed %d scripts, want 857", len(got))
	}
	if !slices.Equal(got, want) {
		t.Fatal("scripts.xml order differs from the reference manifest")
	}
	wantScheduled := map[string]script.Listing{
		"quest.Q620_FourGoblets":    {Path: "quest.Q620_FourGoblets", Schedule: "HOURLY", Start: "00:00", End: "50:00"},
		"task.CastleTaxRefresh":     {Path: "task.CastleTaxRefresh", Schedule: "DAILY", Start: "00:00:00"},
		"task.ClanLadderRefresh":    {Path: "task.ClanLadderRefresh", Schedule: "DAILY", Start: "00:05:00"},
		"task.ClanLeaderTransfer":   {Path: "task.ClanLeaderTransfer", Schedule: "WEEKLY", Start: "TUE 16:55:00"},
		"task.RaidPointReset":       {Path: "task.RaidPointReset", Schedule: "MONTHLY_WEEK", Start: "TUE-1 00:00:00"},
		"task.RecommendationUpdate": {Path: "task.RecommendationUpdate", Schedule: "DAILY", Start: "13:00:00"},
		"task.SevenSignsUpdate":     {Path: "task.SevenSignsUpdate", Schedule: "HOURLY", Start: "00:00"},
	}
	if len(scheduled) != len(wantScheduled) {
		t.Fatalf("entries with schedule attributes = %v, want %v", scheduled, wantScheduled)
	}
	for path, w := range wantScheduled {
		if scheduled[path] != w {
			t.Errorf("%s = %+v, want %+v", path, scheduled[path], w)
		}
	}
}

// TestLoadScriptListSkipsEntryWithoutPath: an entry with no path is
// logged and skipped; an empty path is kept, to be reported missing.
func TestLoadScriptListSkipsEntryWithoutPath(t *testing.T) {
	t.Parallel()
	path := filepath.Join(t.TempDir(), "scripts.xml")
	writeXMLFixture(t, path, `<list><script path="quest.A"/><script schedule="DAILY"/><script path=""/><script path="task.B" schedule="DAILY" start="01:00:00"/></list>`)
	var logs bytes.Buffer
	list, err := LoadScriptList(path, zerolog.New(&logs))
	if err != nil {
		t.Fatal(err)
	}
	want := []script.Listing{{Path: "quest.A"}, {Path: ""}, {Path: "task.B", Schedule: "DAILY", Start: "01:00:00"}}
	if !slices.Equal(list, want) {
		t.Fatalf("list = %+v, want %+v", list, want)
	}
	if !strings.Contains(logs.String(), "an entry has no path") {
		t.Fatalf("missing path not logged: %s", logs.String())
	}

	writeXMLFixture(t, path, `<scripts><script path="quest.A"/></scripts>`)
	if _, err := LoadScriptList(path, zerolog.Nop()); err == nil {
		t.Fatal("a file whose root is not <list> loaded")
	}
}
