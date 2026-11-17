package scenario

import (
	"strings"
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/testsupport"
)

// TestEveryCheckKindHasAnEvaluator: a check the parser accepts must reach an
// evaluator; one that did not would pass silently. An unknown kind fails.
func TestEveryCheckKindHasAnEvaluator(t *testing.T) {
	for kind, family := range checkKinds {
		if checkEvaluators[family] == nil {
			t.Errorf("the %s check has no evaluator", kind)
		}
	}
	for _, kind := range stateChecks {
		if checkKinds[kind] != checkState {
			t.Errorf("the %s state check is not evaluated as a state check", kind)
		}
	}
	var r run
	got := r.check(t, Check{Kind: "bogus"})
	if !strings.Contains(got, "no evaluator for the bogus check") {
		t.Fatalf("an unknown check kind answered %q, want a failure", got)
	}
}

func TestParseTakesTheRejectedMarkerOnABypassOnly(t *testing.T) {
	const head = "character A 1\nnpc a 1 Folk 0 0 0\n"
	sc, err := Parse("ok.scenario", []byte(head+"bypass npc_{a}_Quest\n  rejected\n"))
	if err != nil || !sc.Steps[0].Rejected {
		t.Fatalf("Parse = %+v, %v; want a rejected bypass step", sc, err)
	}
	for _, tc := range []struct{ name, text, want string }{
		{"on an action", "action a\n  rejected\n", "rejected is one bare line under a bypass step"},
		{"with an argument", "bypass npc_{a}_Quest\n  rejected now\n", "rejected is one bare line"},
		{"twice", "bypass npc_{a}_Quest\n  rejected\n  rejected\n", "rejected is one bare line"},
		{"with a packet", "bypass npc_{a}_Quest\n  rejected\n  S ActionFailed\n", "a rejected bypass is sent once and answers nothing"},
		{"with a statement", "bypass npc_{a}_Quest\n  rejected\n  Q upsert Q | a | b\n", "answers nothing"},
		{"with xN", "bypass npc_{a}_Quest x3\n  rejected\n", "answers nothing"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, err := Parse("bad.scenario", []byte(head+tc.text))
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("Parse error = %v, want one containing %q", err, tc.want)
			}
		})
	}
}

func npcPage(obj int32, html string) []byte {
	// The frame is its payload behind a two byte length.
	return serverpackets.FrameNpcHtmlMessage(obj, html, 0).Bytes()[2:]
}

// TestLinksFollowTheLastPageTheClientWasSent: the links are those of the
// last dialog page, read as the server reads them.
func TestLinksFollowTheLastPageTheClientWasSent(t *testing.T) {
	var l links
	l.observe(npcPage(7, `<a action="bypass -h npc_7_Chat 1">a</a><a action="bypass Quest Q1 go.htm ">b</a>`+
		`<a action="bypass -h npc_7_Quest $name">c</a><a action="bypass -h $x">d</a>`))
	for _, tc := range []struct {
		command string
		want    bool
	}{
		{"npc_7_Chat 1", true},
		{"npc_7_Chat 2", false},
		{"Quest Q1 go.htm", true}, // the link's blanks are trimmed
		{"Quest Q1 other.htm", false},
		{"npc_7_Quest anything", true}, // a '$' link is a prefix
		{"npc_7_Que", false},
		{"npc_8_Chat 1", false},
	} {
		if got := l.offers(tc.command); got != tc.want {
			t.Errorf("offers(%q) = %t, want %t", tc.command, got, tc.want)
		}
	}

	l.observe(npcPage(7, `<a action="bypass -h npc_7_Chat 2">a</a>`))
	if l.offers("npc_7_Chat 1") || !l.offers("npc_7_Chat 2") {
		t.Fatal("a page did not replace the links of the last")
	}
	// A page for no NPC is one the player asked for: its links are not kept.
	l.observe(npcPage(0, `<a action="bypass -h npc_7_Chat 3">a</a>`))
	if l.offers("npc_7_Chat 3") || !l.offers("npc_7_Chat 2") {
		t.Fatal("a page with no NPC changed the links")
	}
	l.observe([]byte{serverpackets.OpcodeCharSelected})
	if l.offers("npc_7_Chat 2") {
		t.Fatal("entering the world kept the links")
	}
}

// TestOfferedDiff: a validated command the last page did not offer fails an
// unmarked step, and a marked step fails when the page does offer it.
func TestOfferedDiff(t *testing.T) {
	c := new(testsupport.ScriptedClient)
	r := &run{}
	l := &links{}
	l.record(`<a action="bypass -h npc_7_Chat 1">a</a>`)
	r.links = map[*testsupport.ScriptedClient]*links{c: l}
	for _, tc := range []struct {
		name     string
		command  string
		rejected bool
		want     string
	}{
		{"offered", "npc_7_Chat 1", false, ""},
		{"unoffered", "npc_7_Chat 2", false, "does not offer npc_7_Chat 2"},
		{"unoffered quest", "Quest Q1 x.htm", false, "does not offer Quest Q1 x.htm"},
		{"unoffered and marked", "npc_7_Chat 2", true, ""},
		{"offered and marked", "npc_7_Chat 1", true, "marked rejected, but the last page the client received offers npc_7_Chat 1"},
		{"not validated", "bbs_home", false, ""},
		{"not validated and marked", "bbs_home", true, "does not check bbs_home"},
	} {
		if got := r.offeredDiff(c, tc.command, tc.rejected); !strings.Contains(got, tc.want) || (tc.want == "") != (got == "") {
			t.Errorf("%s: offeredDiff = %q, want one containing %q", tc.name, got, tc.want)
		}
	}
}
