package scenario

import (
	"slices"
	"strings"
	"testing"
)

const createFlow = `character Newbie 1
script script.feature.Tutorial
create
  Q upsert Tutorial | <state> | STARTED
  rows Tutorial <state>=STARTED
enter
mark 1
pickup 6353
restart
  rows Tutorial <state>=STARTED
enter
relog
`

func TestParseReadsACreatedCharactersFlow(t *testing.T) {
	sc, err := Parse("create.scenario", []byte(createFlow))
	if err != nil {
		t.Fatal(err)
	}
	if !sc.Creates() {
		t.Fatal("a scenario starting with create does not create its character")
	}
	var verbs []string
	for _, st := range sc.Steps {
		verbs = append(verbs, st.String())
	}
	if want := []string{"create", "enter", "mark 1", "pickup 6353", "restart", "enter", "relog"}; !slices.Equal(verbs, want) {
		t.Fatalf("steps = %q, want %q", verbs, want)
	}
	if c := sc.Steps[0]; !slices.Equal(c.Statements, []string{"upsert Tutorial | <state> | STARTED"}) || len(c.Checks) != 1 {
		t.Fatalf("create step = %+v", c)
	}
	plain, err := Parse("plain.scenario", []byte("character A 1\nrestart\nenter\n"))
	if err != nil {
		t.Fatal(err)
	}
	if plain.Creates() {
		t.Fatal("a scenario without create creates its character")
	}
}

func TestParseRejectsStepsOutOfTheirPlace(t *testing.T) {
	for _, tc := range []struct{ name, text, want string }{
		{"create later", "character A 1\nrelog\ncreate\n", "3: create is the first step only"},
		{"create above level 1", "character A 5\ncreate\nenter\n", "starts at level 1, not 5"},
		{"create with class", "character A 1\nclass 18\ncreate\nenter\n", "no class, subclass, clan or quest line"},
		{"create with subclass", "character A 1\nsubclass 12 50\ncreate\nenter\n", "no class, subclass, clan or quest line"},
		{"create with clan", "character A 1\nclan 1\ncreate\nenter\n", "no class, subclass, clan or quest line"},
		{"create with quest", "character A 1\nquest Tutorial <state>=STARTED\ncreate\nenter\n", "no class, subclass, clan or quest line"},
		{"step before enter", "character A 1\ncreate\nadvance 1s\n", "3: advance needs the character in the world"},
		{"step after restart", "character A 1\nrestart\nmark 1\n", "3: mark needs the character in the world"},
		{"enter in the world", "character A 1\nenter\n", "2: enter needs the character select"},
		{"restart twice", "character A 1\nrestart\nrestart\n", "restart needs the character in the world"},
		{"items at the select", "character A 1\nrestart\n  items 57=1\n", "3: a items check reads the character in the world"},
		{"state at the select", "character A 1\ncreate\n  state Tutorial STARTED\n", "a state check reads the character in the world"},
		{"bad mark", "character A 1\nmark one\n", `mark number "one"`},
		{"bad pickup", "character A 1\npickup gem\n", `pickup number "gem"`},
		{"create args", "character A 1\ncreate Newbie\n", "create takes 0 to 0"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, err := Parse("bad.scenario", []byte(tc.text))
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("Parse error = %v, want one containing %q", err, tc.want)
			}
		})
	}
}

func TestExpandPageFillsATutorialWindow(t *testing.T) {
	pages := map[string]string{"script/feature/Tutorial/t.htm": "<html><body>%objectId%\n<a action=\"link TE02\">Go</a></body></html>\n"}
	got, err := expandPage("TutorialShowHtml page=script/feature/Tutorial/t.htm", pages)
	if err != nil {
		t.Fatal(err)
	}
	// A tutorial window belongs to no NPC: its text is the page's.
	want := `TutorialShowHtml html="<html><body>%objectId%\n<a action=\"link TE02\">Go</a></body></html>\n"`
	if got != want {
		t.Fatalf("expandPage = %s, want %s", got, want)
	}
	if _, err := expandPage("TutorialShowHtml page=script/feature/Tutorial/none.htm", pages); err == nil {
		t.Fatal("expandPage accepted a page the scenario does not serve")
	}
}
