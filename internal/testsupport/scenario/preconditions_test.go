package scenario

import (
	"slices"
	"strings"
	"testing"
)

const preconditions = `character Elf 20
class 18
clan 5
quest Q022_TragedyInVonHellmannForest <state>=COMPLETED
quest Q260_HuntTheOrcs <state>=STARTED <cond>=1
random 0 39 99
action darin
`

func TestParseReadsPreconditionLines(t *testing.T) {
	sc, err := Parse("pre.scenario", []byte("npc darin 30048 Folk 0 0 0\n"+preconditions))
	if err != nil {
		t.Fatal(err)
	}
	if sc.Class != 18 || sc.Clan != 5 {
		t.Fatalf("class %d clan %d", sc.Class, sc.Clan)
	}
	wantQuests := []QuestRows{
		{Quest: "Q022_TragedyInVonHellmannForest", Vars: [][2]string{{"<state>", "COMPLETED"}}},
		{Quest: "Q260_HuntTheOrcs", Vars: [][2]string{{"<state>", "STARTED"}, {"<cond>", "1"}}},
	}
	if !slices.EqualFunc(sc.Quests, wantQuests, func(a, b QuestRows) bool { return a.Quest == b.Quest && slices.Equal(a.Vars, b.Vars) }) {
		t.Fatalf("quests = %+v", sc.Quests)
	}
	if sc.Random == nil || sc.Random.Always || !slices.Equal(sc.Random.Draws, []int{0, 39, 99}) {
		t.Fatalf("random = %+v", sc.Random)
	}
}

func TestParseDefaultsWithoutPreconditionLines(t *testing.T) {
	sc, err := Parse("plain.scenario", []byte("character A 1\nadvance 1s\n"))
	if err != nil {
		t.Fatal(err)
	}
	if sc.Class != 0 || sc.Clan != -1 || sc.Quests != nil || sc.Random != nil {
		t.Fatalf("defaults = class %d clan %d quests %v random %v", sc.Class, sc.Clan, sc.Quests, sc.Random)
	}
}

func TestParseReadsRandomAlways(t *testing.T) {
	sc, err := Parse("always.scenario", []byte("character A 1\nrandom always 7\nadvance 1s\n"))
	if err != nil {
		t.Fatal(err)
	}
	if sc.Random == nil || !sc.Random.Always || !slices.Equal(sc.Random.Draws, []int{7}) {
		t.Fatalf("random = %+v", sc.Random)
	}
}

func TestParseRejectsMalformedPreconditionLines(t *testing.T) {
	for _, tc := range []struct{ name, line, want string }{
		{"class without id", "class", "one class line"},
		{"class twice", "class 18\nclass 18", "one class line"},
		{"class not a number", "class elf", `class "elf"`},
		{"class zero", "class 0", "class 0 is the default"},
		{"unknown class", "class 9999", "class 9999 has no race"},
		{"clan twice", "clan 1\nclan 2", "one clan line"},
		{"clan level too high", "clan 9", "is not 0 to 8"},
		{"clan level negative", "clan -1", "is not 0 to 8"},
		{"quest without rows", "quest Q001_LettersOfLove", "quest <quest> <var>=<value>"},
		{"quest row without value", "quest Q001_LettersOfLove cond", "is not <var>=<value>"},
		{"quest twice", "quest Q1 a=1\nquest Q1 b=2", "listed twice"},
		{"random without draws", "random", "random <draw>"},
		{"random twice", "random 1\nrandom 2", "one random line"},
		{"random negative", "random -1", "is not a number of 0 or more"},
		{"random not a number", "random x", "is not a number of 0 or more"},
		{"random always list", "random always 1 2", "random always <draw>"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, err := Parse("bad.scenario", []byte("character A 1\n"+tc.line+"\nadvance 1s\n"))
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("err = %v, want %q", err, tc.want)
			}
		})
	}
}

func TestDrawsServeTheListInOrder(t *testing.T) {
	d := &draws{random: &Random{Draws: []int{3, 0}}}
	if got := []int{d.draw(10), d.draw(1)}; !slices.Equal(got, []int{3, 0}) {
		t.Fatalf("draws = %v", got)
	}
	if bad := d.take(); bad != nil || d.unused() != 0 {
		t.Fatalf("bad %v unused %d", bad, d.unused())
	}
	d.draw(5)
	if bad := d.take(); len(bad) != 1 || !strings.Contains(bad[0], "lists only 2") {
		t.Fatalf("past the list: %v", bad)
	}
}

func TestDrawsRejectADrawOutsideItsRange(t *testing.T) {
	d := &draws{random: &Random{Draws: []int{40, 1}}}
	if v := d.draw(40); v != 0 {
		t.Fatalf("out of range draw = %d, want 0", v)
	}
	if bad := d.take(); len(bad) != 1 || !strings.Contains(bad[0], "draw 1 is 40, outside [0, 40)") {
		t.Fatalf("bad = %v", bad)
	}
	if d.unused() != 1 {
		t.Fatalf("unused = %d, want 1", d.unused())
	}
}

func TestDrawsAlwaysRepeatTheDraw(t *testing.T) {
	d := &draws{random: &Random{Draws: []int{2}, Always: true}}
	for range 5 {
		if v := d.draw(3); v != 2 {
			t.Fatalf("draw = %d", v)
		}
	}
	d.draw(2)
	if bad := d.take(); len(bad) != 1 || d.unused() != 0 {
		t.Fatalf("bad %v unused %d", bad, d.unused())
	}
}
