package scriptcontract

import (
	"bufio"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"slices"
	"strings"
	"testing"

	"github.com/fatal10110/acis_golang/internal/commons/wire"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
)

func mustTables(t *testing.T) []Table {
	t.Helper()
	tables, err := Tables()
	if err != nil {
		t.Fatalf("Tables: %v", err)
	}
	return tables
}

// Every engine contract of the plan has its goldens, and every golden belongs
// to exactly one contract.
func TestContractsCoverEveryGolden(t *testing.T) {
	tables := mustTables(t)
	byName := map[string]Table{}
	for _, tb := range tables {
		if _, dup := byName[tb.Name]; dup {
			t.Errorf("table %s is defined twice", tb.Name)
		}
		byName[tb.Name] = tb
	}

	owner := map[string]string{}
	for _, c := range Contracts {
		if len(c.Slices) == 0 || len(c.Tables) == 0 {
			t.Errorf("contract %q names no slice or no table", c.Name)
		}
		for _, name := range c.Tables {
			if prev, ok := owner[name]; ok {
				t.Errorf("table %s belongs to %q and %q", name, prev, c.Name)
			}
			owner[name] = c.Name
			if tb, ok := byName[name]; !ok {
				t.Errorf("contract %q: no golden %s", c.Name, name)
			} else if len(tb.Rows) == 0 {
				t.Errorf("golden %s has no rows", name)
			}
		}
	}
	for _, tb := range tables {
		if _, ok := owner[tb.Name]; !ok {
			t.Errorf("golden %s (%s) belongs to no contract", tb.Name, tb.File)
		}
	}
}

var probeProvenance = regexp.MustCompile(`^probe java=[0-9a-f]{40} datapack=[0-9a-f]{40}$`)

// Each golden names the reference lines it was derived from and says how it
// was produced: probe goldens carry the reference revision, hand goldens live
// in *_hand.golden files.
func TestGoldensNameSourceAndProvenance(t *testing.T) {
	revisions := map[string]bool{}
	for _, tb := range mustTables(t) {
		if len(tb.Sources) == 0 {
			t.Errorf("%s: no source", tb.Name)
		}
		for _, s := range tb.Sources {
			if !strings.HasPrefix(s.File, "java/") && !strings.HasPrefix(s.File, "../aCis_datapack/") {
				t.Errorf("%s: source %s is not below the reference tree", tb.Name, s.File)
			}
			if s.From < 1 || s.To < s.From || s.Symbol == "" {
				t.Errorf("%s: source %+v has no line range or symbol", tb.Name, s)
			}
		}
		hand := strings.HasSuffix(tb.File, "_hand.golden")
		switch {
		case hand && !tb.Hand():
			t.Errorf("%s: in hand file %s with provenance %q", tb.Name, tb.File, tb.Provenance)
		case !hand && !probeProvenance.MatchString(tb.Provenance):
			t.Errorf("%s: probe file %s with provenance %q", tb.Name, tb.File, tb.Provenance)
		case !hand:
			revisions[tb.Provenance] = true
		}
		ids := map[string]bool{}
		for _, r := range tb.Rows {
			if ids[r.ID] {
				t.Errorf("%s: row %s twice", tb.Name, r.ID)
			}
			ids[r.ID] = true
		}
	}
	if len(revisions) != 1 {
		t.Errorf("probe goldens carry %d reference revisions, want 1: %v", len(revisions), revisions)
	}
}

// Every cited range exists in the reference checkout and holds the symbol it
// names. The reference tree is not in CI; ACIS_REQUIRE_DATAPACK turns its
// absence into a failure, as for the datapack oracle tests.
func TestSourcesMatchReference(t *testing.T) {
	root, ok := referenceRoot()
	if !ok {
		if os.Getenv("ACIS_REQUIRE_DATAPACK") != "" {
			t.Fatal("aCis_gameserver not found above the module root and ACIS_REQUIRE_DATAPACK is set")
		}
		t.Skip("aCis_gameserver not checked out near the module root")
	}
	lines := map[string][]string{}
	for _, tb := range mustTables(t) {
		for _, s := range tb.Sources {
			file, ok := lines[s.File]
			if !ok {
				file = readLines(t, filepath.Join(root, filepath.FromSlash(s.File)))
				lines[s.File] = file
			}
			if s.To > len(file) {
				t.Errorf("%s: %s has %d lines, cited %d-%d", tb.Name, s.File, len(file), s.From, s.To)
				continue
			}
			word := strings.Fields(s.Symbol)[0]
			if !strings.Contains(strings.Join(file[s.From-1:s.To], "\n"), word) {
				t.Errorf("%s: %s %d-%d does not mention %s", tb.Name, s.File, s.From, s.To, word)
			}
		}
	}
}

func referenceRoot() (string, bool) {
	_, here, _, ok := runtime.Caller(0)
	if !ok {
		return "", false
	}
	for dir := filepath.Dir(here); ; {
		for _, c := range []string{filepath.Join(dir, "aCis_gameserver"), filepath.Join(dir, "acis_public", "aCis_gameserver")} {
			if info, err := os.Stat(filepath.Join(c, "java")); err == nil && info.IsDir() {
				return c, true
			}
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return "", false
		}
		dir = parent
	}
}

func readLines(t *testing.T, path string) []string {
	t.Helper()
	f, err := os.Open(path)
	if err != nil {
		t.Fatalf("open reference file: %v", err)
	}
	defer f.Close()
	var out []string
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 1<<20), 1<<20)
	for sc.Scan() {
		out = append(out, sc.Text())
	}
	if err := sc.Err(); err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	return out
}

func payload(t *testing.T, f wire.Frame) []byte {
	t.Helper()
	defer f.Release()
	if err := f.Err(); err != nil {
		t.Fatalf("frame: %v", err)
	}
	return append([]byte(nil), f.Bytes()[2:]...)
}

func goldenLines(t *testing.T, table string) []string {
	t.Helper()
	var out []string
	for _, r := range Lookup(t, table).Rows {
		out = append(out, r.Lines...)
	}
	return out
}

// Packet renders the Go encoders' bytes exactly as the probe rendered the
// reference packets, so a slice compares its captured packets with golden
// lines as strings.
func TestPacketMatchesProbeLines(t *testing.T) {
	const noQuest = "<html><body>You are either not on a quest that involves this NPC, or you don't meet this NPC's minimum quest requirements.</body></html>"
	const roxxy = int32(268500001)
	chooser := "<html><body><a action=\"bypass -h npc_268500001_Quest Q001_LettersOfLove\">[Letters of Love (In Progress)]</a><br>" +
		"<a action=\"bypass -h npc_268500001_Quest Q006_StepIntoTheFuture\">[Step into the Future]</a><br></body></html>"
	roles := map[int32]string{roxxy: "roxxy", 268500002: "baulro"}

	cases := []struct {
		name    string
		payload []byte
		table   string
	}{
		{"quest list", payload(t, serverpackets.FrameQuestList([]serverpackets.QuestListEntry{{QuestID: 1, Flags: -0x7fffffff}})), "journal.write_order"},
		{"empty quest list", payload(t, serverpackets.FrameQuestList(nil)), "questlist.packet"},
		{"quest mark", []byte{0xfe, 0x1a, 0x00, 0x01, 0x00, 0x00, 0x00}, "journal.write_order"},
		{"action failed", payload(t, serverpackets.FrameActionFailed()), "dialog.general_window"},
		{"sound", payload(t, serverpackets.FramePlaySound("ItemSound.quest_itemget")), "drop.divmod"},
		{"item message", payload(t, serverpackets.FrameSystemMessageParams(54, serverpackets.ItemNameParam(1081))), "drop.divmod"},
		{"item count message", payload(t, serverpackets.FrameSystemMessageParams(53, serverpackets.ItemNameParam(1081), serverpackets.ItemNumberParam(4))), "drop.divmod"},
		{"overweight message", payload(t, serverpackets.FrameSystemMessage(1118)), "dialog.general_window"},
		{"no-quest page", payload(t, serverpackets.FrameNpcHtmlMessage(268500002, noQuest, 0)), "dialog.general_window"},
		{"chooser page", payload(t, serverpackets.FrameNpcHtmlMessage(roxxy, chooser, 0)), "dialog.general_window"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, err := Packet(c.payload, roles)
			if err != nil {
				t.Fatalf("Packet: %v", err)
			}
			if !slices.Contains(goldenLines(t, c.table), got) {
				t.Fatalf("Packet = %s\nno such line in %s", got, c.table)
			}
		})
	}

	if _, err := Packet([]byte{0x2e, 0x01}, nil); err == nil {
		t.Error("Packet rendered an undecoded packet")
	}
	if _, err := Packet([]byte{0x0f, 0x01}, nil); err == nil {
		t.Error("Packet rendered a truncated packet")
	}
}

// Every statement line of the goldens is a journal statement, and the exit
// rows pin their order.
func TestParseStatement(t *testing.T) {
	for _, tb := range mustTables(t) {
		for _, r := range tb.Rows {
			for _, l := range r.Lines {
				if !strings.HasPrefix(l, "Q ") {
					continue
				}
				if _, err := ParseStatement(l); err != nil {
					t.Errorf("%s/%s: %v", tb.Name, r.ID, err)
				}
			}
		}
	}

	kinds := map[string][]StatementKind{}
	for _, r := range Lookup(t, "journal.write_order").Rows {
		for _, l := range r.Lines {
			if st, err := ParseStatement(l); err == nil {
				kinds[r.ID] = append(kinds[r.ID], st.Kind)
			}
		}
	}
	want := map[string][]StatementKind{
		"exit-complete":   {Upsert, DeleteExceptState},
		"exit-repeatable": {DeleteQuest},
		"set-cond-skip":   {Upsert, Upsert},
		"unset-var":       {DeleteVar},
	}
	for id, w := range want {
		if !slices.Equal(kinds[id], w) {
			t.Errorf("%s statements = %v, want %v", id, kinds[id], w)
		}
	}
}

func TestParse(t *testing.T) {
	got, err := parse("x.golden", []byte(strings.Join([]string{
		"# comment",
		"table a.b",
		"source java/X.java 3-9 run thing",
		"source java/Y.java 7 FIELD",
		"provenance hand",
		"note first",
		`row r1 n=0x80000001 s="a b\"c\n" l=1,2 e=- f=1.5 ok=true`,
		"  S ActionFailed",
		"  Q x | 1",
		"row r2",
	}, "\n")))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if len(got) != 1 || got[0].Name != "a.b" || !got[0].Hand() || len(got[0].Rows) != 2 {
		t.Fatalf("parse = %+v", got)
	}
	tb := got[0]
	if tb.Sources[0] != (Source{File: "java/X.java", From: 3, To: 9, Symbol: "run thing"}) || tb.Sources[1].From != 7 || tb.Sources[1].To != 7 {
		t.Errorf("sources = %+v", tb.Sources)
	}
	r := tb.Rows[0]
	if r.Int(t, "n") != 0x80000001 || r.Str(t, "s") != "a b\"c\n" || !slices.Equal(r.List(t, "l"), []string{"1", "2"}) ||
		r.List(t, "e") != nil || r.Float(t, "f") != 1.5 || !r.Bool(t, "ok") {
		t.Errorf("row values = %v", r.values)
	}
	if !slices.Equal(r.Keys(), []string{"n", "s", "l", "e", "f", "ok"}) || !slices.Equal(r.Lines, []string{"S ActionFailed", "Q x | 1"}) {
		t.Errorf("row = %+v", r)
	}

	for _, bad := range []string{
		"row early",
		"table t\n  stray",
		"table t\nsource java/X.java x-2 s",
		"table t\nrow r k",
		`table t` + "\n" + `row r k="open`,
		"table t\nrow r k=1 k=2",
		"table t\nwhat is this",
	} {
		if _, err := parse("bad.golden", []byte(bad)); err == nil {
			t.Errorf("parse(%q) succeeded", bad)
		}
	}
}

// The rendering rules for values agree with the probe writer.
func TestQuoteAndValue(t *testing.T) {
	for in, want := range map[string]string{
		"plain":           "plain",
		"":                `""`,
		"two words":       `"two words"`,
		"k=v":             `"k=v"`,
		"tab\there":       `"tab\there"`,
		"bell\a":          `"bell\u0007"`,
		"quote\"back\\sl": `"quote\"back\\sl"`,
		"Ünïcode":         `"Ünïcode"`,
	} {
		if got := Value(in); got != want {
			t.Errorf("Value(%q) = %s, want %s", in, got, want)
		}
	}
}
