package scenario

import (
	"reflect"
	"strings"
	"testing"
)

const seedSample = `character Leader 40
standing 5
hold 7215 1
member Mate 40 standing 2 hold 7212 1 20 0 0
member Third 40 dead standing -3
zone boss 110002 30 -20 70 20 -100 100
zone peace 7 -10 -10 10 10 -5 5
door gate 19160001 100 0 0
npc cube 31842 Folk 50 0 0

bypass Quest GrandBossTeleporter 31540 x49
  items 7267=1
  open gate no
  dungeon yes
  standing Mate 0
  standing player -5
stance player on
  stance player on
  S TeleportToLocation obj=player loc=1,2,3+-100 fast=0
appear
`

func TestParseReadsWorldSeeds(t *testing.T) {
	sc, err := Parse("seeds.scenario", []byte(seedSample))
	if err != nil {
		t.Fatal(err)
	}
	wantMembers := []Member{
		{Name: "Mate", Level: 40, Standing: 2, Holds: []Hold{{Item: 7212, Count: 1}}, DX: 20},
		{Name: "Third", Level: 40, Dead: true, Standing: -3},
	}
	if !reflect.DeepEqual(sc.Members, wantMembers) {
		t.Fatalf("members = %+v", sc.Members)
	}
	wantZones := []ZoneLine{
		{Kind: "boss", ID: 110002, DX1: 30, DY1: -20, DX2: 70, DY2: 20, DZ1: -100, DZ2: 100},
		{Kind: "peace", ID: 7, DX1: -10, DY1: -10, DX2: 10, DY2: 10, DZ1: -5, DZ2: 5},
	}
	if !reflect.DeepEqual(sc.Zones, wantZones) {
		t.Fatalf("zones = %+v", sc.Zones)
	}
	st := sc.Steps[0]
	if st.Verb != "bypass" || st.Args[0] != "Quest GrandBossTeleporter 31540" || st.Repeat != 49 || len(st.Checks) != 5 {
		t.Fatalf("bypass step = %+v", st)
	}
	if sc.Steps[1].Verb != "stance" || sc.Steps[2].Verb != "appear" || !sc.stancePlayer() {
		t.Fatalf("steps = %+v", sc.Steps)
	}
}

func TestParseKeepsABypassWithoutACountWhole(t *testing.T) {
	sc, err := Parse("bypass.scenario", []byte("character A 1\nbypass Quest X x1y\nbypass Quest Y\nbypass Z x2\n"))
	if err != nil {
		t.Fatal(err)
	}
	for i, want := range []struct {
		cmd    string
		repeat int
	}{{"Quest X x1y", 0}, {"Quest Y", 0}, {"Z", 2}} {
		if got := sc.Steps[i]; got.Args[0] != want.cmd || got.Repeat != want.repeat {
			t.Errorf("step %d = %q x%d, want %q x%d", i, got.Args[0], got.Repeat, want.cmd, want.repeat)
		}
	}
}

func TestParseRejectsMalformedWorldSeeds(t *testing.T) {
	for _, tc := range []struct{ name, text, want string }{
		{"zone kind", "zone water 1 0 0 1 1 0 0\nadvance 1s", `zone kind "water" is not one of boss, peace`},
		{"zone arity", "zone boss 1 0 0\nadvance 1s", "zone <boss|peace>"},
		{"zone twice", "zone boss 1 0 0 1 1 0 0\nzone peace 1 0 0 1 1 0 0\nadvance 1s", "listed twice"},
		{"zone no box", "zone boss 1 5 0 1 1 0 0\nadvance 1s", "is no box"},
		{"zone battlefield id", "zone boss 90001 0 0 1 1 0 0\nadvance 1s", "the summon battlefield's"},
		{"member standing zero", "member M 5 standing 0\nadvance 1s", "member standing"},
		{"member standing high", "member M 5 standing 6\nadvance 1s", "member standing"},
		{"member hold count", "member M 5 hold 57 0\nadvance 1s", "member hold"},
		{"bypass count zero", "bypass Quest X x0", "bypass count"},
		{"open unknown door", "advance 1s\n  open gate yes", "no door line names"},
		{"open bad value", "door gate 5 0 0 0\nadvance 1s\n  open gate maybe", "open <door role> <yes|no>"},
		{"dungeon bad value", "advance 1s\n  dungeon maybe", "dungeon <yes|no>"},
		{"standing unknown", "advance 1s\n  standing Nobody 0", "is not player or a member line"},
		{"standing range", "member M 5\nadvance 1s\n  standing M 7", "is not -5 to 5"},
		{"stance unknown", "stance nobody on", "stance <role|player> <on|off>"},
		{"traced repeat", "trace x.golden\nbypass Quest X x2", "a traced scenario takes no appear step, bypass count"},
		{"traced appear", "trace x.golden\nadvance 1s\nappear", "a traced scenario takes no appear step"},
		{"traced member standing", "trace x.golden\nmember M 5\nadvance 1s\n  standing M 0", "a traced scenario takes no member"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, err := Parse("bad.scenario", []byte("character A 40\n"+tc.text+"\n"))
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("Parse error = %v, want it to contain %q", err, tc.want)
			}
		})
	}
}

func TestMatchNearTeleportsAcceptsALandingInTheBox(t *testing.T) {
	want := []string{"ActionFailed", "TeleportToLocation obj=player loc=100,200,-5+-10 fast=0"}
	for _, tc := range []struct {
		got string
		ok  bool
	}{
		{"TeleportToLocation obj=player loc=110,190,-5 fast=0", true},
		{"TeleportToLocation obj=player loc=111,190,-5 fast=0", false},
		{"TeleportToLocation obj=player loc=100,200,-4 fast=0", false},
		{"TeleportToLocation obj=other loc=100,200,-5 fast=0", false},
	} {
		got := matchNearTeleports([]string{"ActionFailed", tc.got}, want)
		if (FirstDiff("packet", got, want) == "") != tc.ok {
			t.Errorf("%s: match = %v, want %v", tc.got, !tc.ok, tc.ok)
		}
	}
}
