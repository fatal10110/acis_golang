package scenario

import (
	"slices"
	"strings"
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/script"
)

const sample = `# a comment
trace some/trace.golden
character Talker 20
script quest.Q001_LettersOfLove
stub quest.Q006_StepIntoTheFuture
item 687 906
page default/30048.htm
npc darin 30048 Folk 10 0 -5
npc wolf 20120 Monster 40 0 0

action darin
  S ActionFailed
bypass npc_{darin}_Quest  Q001_LettersOfLove
  S NpcHtmlMessage obj=darin item=0 page=default/30048.htm
  Q upsert Q001_LettersOfLove | <cond> | 1
  items 687=1 906=0
  rows Q001_LettersOfLove <cond>=1 <state>=STARTED
  state Q001_LettersOfLove STARTED
kill wolf x3
advance 3500ms
relog
  rows Q001_LettersOfLove -
`

func TestParseReadsHeaderStepsAndExpectations(t *testing.T) {
	sc, err := Parse("sample.scenario", []byte(sample))
	if err != nil {
		t.Fatal(err)
	}
	if sc.Trace != "some/trace.golden" || sc.Character != "Talker" || sc.Level != 20 {
		t.Fatalf("header = %q %q %d", sc.Trace, sc.Character, sc.Level)
	}
	wantScripts := []ScriptLine{{Path: "quest.Q001_LettersOfLove"}, {Path: "quest.Q006_StepIntoTheFuture", Stub: true}}
	if !slices.Equal(sc.Scripts, wantScripts) || !slices.Equal(sc.Items, []int32{687, 906}) || !slices.Equal(sc.Pages, []string{"default/30048.htm"}) {
		t.Fatalf("scripts %v items %v pages %v", sc.Scripts, sc.Items, sc.Pages)
	}
	wantNPCs := []NPC{{Role: "darin", ID: 30048, Type: "Folk", DX: 10, DZ: -5}, {Role: "wolf", ID: 20120, Type: "Monster", DX: 40}}
	if !slices.Equal(sc.NPCs, wantNPCs) {
		t.Fatalf("npcs = %+v", sc.NPCs)
	}
	var verbs []string
	for _, st := range sc.Steps {
		verbs = append(verbs, st.String())
	}
	wantVerbs := []string{"action darin", "bypass npc_{darin}_Quest  Q001_LettersOfLove", "kill wolf x3", "advance 3500ms", "relog"}
	if !slices.Equal(verbs, wantVerbs) {
		t.Fatalf("steps = %q, want %q", verbs, wantVerbs)
	}
	by := sc.Steps[1]
	if by.Line != 13 || !slices.Equal(by.Packets, []string{"NpcHtmlMessage obj=darin item=0 page=default/30048.htm"}) ||
		!slices.Equal(by.Statements, []string{"upsert Q001_LettersOfLove | <cond> | 1"}) || len(by.Checks) != 3 {
		t.Fatalf("bypass step = %+v", by)
	}
	if ck := by.Checks[1]; ck.Kind != "rows" || ck.Line != 17 || !slices.Equal(ck.Args, []string{"Q001_LettersOfLove", "<cond>=1", "<state>=STARTED"}) {
		t.Fatalf("rows check = %+v", ck)
	}
}

func TestParseRejectsMalformedFiles(t *testing.T) {
	for _, tc := range []struct{ name, text, want string }{
		{"no character", "action x\n", "unknown role"},
		{"missing character", "npc a 1 Folk 0 0 0\naction a\n", "no character line"},
		{"no step", "character A 1\n", "no step"},
		{"unknown header", "character A 1\nfoo bar\naction a\n", "unknown header line"},
		{"unknown verb", "character A 1\nnpc a 1 Folk 0 0 0\naction a\ndance a\n", `unknown step "dance"`},
		{"unknown role", "character A 1\naction a\n", `unknown role "a"`},
		{"expectation first", "character A 1\n  S ActionFailed\n", "before any step"},
		{"unknown expectation", "character A 1\nnpc a 1 Folk 0 0 0\naction a\n  X y\n", "unknown expectation"},
		{"bad kill count", "character A 1\nnpc a 1 Monster 0 0 0\nkill a 3\n", "not xN"},
		{"bad duration", "character A 1\nadvance soon\n", "invalid duration"},
		{"bad item check", "character A 1\nrelog\n  items 57\n", "is not <id>=<count>"},
		{"bad row", "character A 1\nrelog\n  rows Q a\n", "<var>=<value>"},
		{"role twice", "character A 1\nnpc a 1 Folk 0 0 0\nnpc a 2 Folk 0 0 0\nrelog\n", "named twice"},
		{"script twice", "character A 1\nscript a\nstub a\nrelog\n", "listed twice"},
		{"relog args", "character A 1\nrelog now\n", "relog takes 0 to 0"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, err := Parse("bad.scenario", []byte(tc.text))
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("Parse error = %v, want one containing %q", err, tc.want)
			}
		})
	}
}

// TestFirstDiffNamesTheFirstDifference: a failing expectation reports the
// first line that differs, is missing or is extra, with its position.
func TestFirstDiffNamesTheFirstDifference(t *testing.T) {
	for _, tc := range []struct {
		got, want []string
		diff      string
	}{
		{[]string{"a", "b"}, []string{"a", "b"}, ""},
		{nil, nil, ""},
		{[]string{"a", "x", "c"}, []string{"a", "b", "d"}, "packet 2 differs:\n  got x\n want b"},
		{[]string{"a"}, []string{"a", "b"}, "packet 2 missing:\n want b"},
		{[]string{"a", "b"}, []string{"a"}, "packet 2 not expected:\n  got b"},
	} {
		if got := FirstDiff("packet", tc.got, tc.want); got != tc.diff {
			t.Errorf("FirstDiff(%q, %q) = %q, want %q", tc.got, tc.want, got, tc.diff)
		}
	}
}

func TestExpandPageFillsTheNPCsObjectID(t *testing.T) {
	pages := map[string]string{"default/1.htm": "<html><body>Hi\n<a action=\"bypass -h npc_%objectId%_Quest\">Q</a></body></html>\n"}
	got, err := expandPage("NpcHtmlMessage obj=darin item=0 page=default/1.htm", pages)
	if err != nil {
		t.Fatal(err)
	}
	want := `NpcHtmlMessage obj=darin item=0 html="<html><body>Hi\n<a action=\"bypass -h npc_{darin}_Quest\">Q</a></body></html>\n"`
	if got != want {
		t.Fatalf("expandPage = %s, want %s", got, want)
	}
	if got, _ := expandPage("SystemMessage id=1", pages); got != "SystemMessage id=1" {
		t.Fatalf("expandPage changed a line without a page: %s", got)
	}
	if _, err := expandPage("NpcHtmlMessage obj=darin item=0 page=default/2.htm", pages); err == nil {
		t.Fatal("expandPage accepted a page the scenario does not serve")
	}
}

// traceText is a two-packet trace: a talk answered with a page, then a
// bypass answered with a quest start, and a kill.
const traceText = `# test trace
# player object 500; format: README.md in this directory.
C Action 30048 (object 600)
S ActionFailed 25
T 500
C RequestBypassToServer npc_600_Quest
S PlaySound 980000000061000000000000000000000000000000000000000000000000000000
Q INSERT INTO character_quests (charId,name,var,value) VALUES (?,?,?,?) ON DUPLICATE KEY UPDATE value=VALUES(value) | 500 | Q001_LettersOfLove | <cond> | 1
# kill 20120 (object 700)
S ActionFailed 25
`

func traceScenario(t *testing.T, steps string) *Scenario {
	t.Helper()
	sc, err := Parse("traced.scenario", []byte("character A 1\nnpc darin 30048 Folk 0 0 0\nnpc wolf 20120 Monster 0 0 0\n"+steps))
	if err != nil {
		t.Fatal(err)
	}
	return sc
}

func TestCheckTraceAcceptsAReplayingScenario(t *testing.T) {
	sc := traceScenario(t, "action darin\n  S ActionFailed\nbypass npc_{darin}_Quest\n  S PlaySound type=0 file=a bind=0 obj=0 loc=0,0,0 delay=0\n  Q upsert Q001_LettersOfLove | <cond> | 1\nkill wolf\n  S ActionFailed\nrelog\n")
	if err := checkTrace(sc, []byte(traceText), nil); err != nil {
		t.Fatal(err)
	}
}

func TestCheckTraceNamesTheFirstDeparture(t *testing.T) {
	for _, tc := range []struct{ name, steps, want string }{
		{"packet", "action darin\nbypass npc_{darin}_Quest\n", "step 1 (line 4) \"action darin\" against trace \"C Action 30048 (object 600)\": scenario packet 1 missing"},
		{"statement", "action darin\n  S ActionFailed\nbypass npc_{darin}_Quest\n  S PlaySound type=0 file=a bind=0 obj=0 loc=0,0,0 delay=0\nkill wolf\n", "step 2 (line 6) \"bypass npc_{darin}_Quest\" against trace \"C RequestBypassToServer npc_600_Quest\": scenario statement 1 missing"},
		{"client", "action darin\n  S ActionFailed\nbypass npc_{darin}_Chat\n", `the step sends "C RequestBypassToServer npc_600_Chat"`},
		{"kill count", "action darin\n  S ActionFailed\nbypass npc_{darin}_Quest\n  S PlaySound type=0 file=a bind=0 obj=0 loc=0,0,0 delay=0\n  Q upsert Q001_LettersOfLove | <cond> | 1\nkill wolf x2\n", "a kill step is not a client packet a trace records"},
		{"short", "action darin\n  S ActionFailed\n", "the trace has 3 client packets, the scenario 1 steps"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			err := checkTrace(traceScenario(t, tc.steps), []byte(traceText), nil)
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("checkTrace error = %v, want one containing %q", err, tc.want)
			}
		})
	}
}

func TestManifestStubRegistersTheReferenceBindings(t *testing.T) {
	stub, err := manifestStub("../../gameserver/script/testdata/oracle/manifest.golden", "quest.Q006_StepIntoTheFuture")
	if err != nil {
		t.Fatal(err)
	}
	if stub.QuestID != 6 || stub.Title != "Step into the Future" || !slices.Equal(stub.Items, []int32{7571}) {
		t.Fatalf("stub = id %d title %q items %v", stub.QuestID, stub.Title, stub.Items)
	}
	if !slices.Equal(stub.Bind[script.EventQuestStart], []int32{30006}) || !slices.Equal(stub.Bind[script.EventTalked], []int32{30006, 30033, 30311}) || len(stub.Bind) != 2 {
		t.Fatalf("stub bindings = %v", stub.Bind)
	}
	if stub.OnTalk != nil || stub.OnEvent != nil {
		t.Fatal("a stub has hooks")
	}
	tele, err := manifestStub("../../gameserver/script/testdata/oracle/manifest.golden", "script.teleport.NoblesseTeleporter")
	if err != nil {
		t.Fatal(err)
	}
	if tele.QuestID != 0 || tele.Title != "" || len(tele.Bind[script.EventTalked]) != 18 {
		t.Fatalf("teleporter stub = id %d title %q talked %v", tele.QuestID, tele.Title, tele.Bind[script.EventTalked])
	}
	if _, err := manifestStub("../../gameserver/script/testdata/oracle/manifest.golden", "quest.Q999_None"); err == nil {
		t.Fatal("a stub of a script the manifest lacks was built")
	}
}

func TestManifestIDsExpandRuns(t *testing.T) {
	got, err := manifestIDs("1,3-5,9")
	if err != nil || !slices.Equal(got, []int32{1, 3, 4, 5, 9}) {
		t.Fatalf("manifestIDs = %v, %v", got, err)
	}
	if _, err := manifestIDs("1,x"); err == nil {
		t.Fatal("manifestIDs accepted a bad id")
	}
}
