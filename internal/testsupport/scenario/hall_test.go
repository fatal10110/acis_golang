package scenario

import (
	"strings"
	"testing"
)

func TestParseReadsTheHallLines(t *testing.T) {
	sc, err := Parse("hall.scenario", []byte(`character A 20
clan 4 hall 22
hall 35 waiting date 4102444800000
hall 63 registering
mate Chief 19 leader sponsor online hold 1458 922 10 20 30
mate Pal 30 apprentice
npc n 1 Folk 10 0 0
enter Chief
as Chief bypass npc_{n}_Quest
  to player SystemMessage id=1
hallstatus 63 siege
  items Chief 1458=0
`))
	if err != nil {
		t.Fatal(err)
	}
	if sc.Clan != 4 || sc.ClanHall != 22 {
		t.Fatalf("clan = %d hall %d, want 4 hall 22", sc.Clan, sc.ClanHall)
	}
	if len(sc.Halls) != 2 || sc.Halls[0] != (Hall{ID: 35, Status: "waiting", Date: 4102444800000}) || sc.Halls[1] != (Hall{ID: 63, Status: "registering"}) {
		t.Fatalf("halls = %+v", sc.Halls)
	}
	chief := sc.Mates[0]
	if !chief.Leader || chief.Relation != "sponsor" || !chief.Online || chief.DX != 10 || chief.DZ != 30 ||
		len(chief.Holds) != 1 || chief.Holds[0] != (Hold{Item: 1458, Count: 922}) {
		t.Fatalf("mate = %+v", chief)
	}
	if sc.Mates[1].Relation != "apprentice" || sc.Mates[1].Online {
		t.Fatalf("mate = %+v", sc.Mates[1])
	}
	st := sc.Steps[1]
	if st.As != "Chief" || st.Verb != "bypass" || st.Args[0] != "npc_{n}_Quest" || len(st.To["player"]) != 1 {
		t.Fatalf("step = %+v", st)
	}
	if got := sc.Steps[2]; got.Verb != "hallstatus" || got.Args[0] != "63" || got.Args[1] != "siege" {
		t.Fatalf("step = %+v", got)
	}
}

func TestParseRejectsMalformedHallLines(t *testing.T) {
	for _, tc := range []struct{ name, lines, want string }{
		{"status", "hall 35 open", `hall status "open"`},
		{"twice", "hall 35 waiting\nhall 35 siege", "hall 35 listed twice"},
		{"id", "hall x waiting", `hall id "x"`},
		{"date word", "hall 35 waiting at 5", "hall <id>"},
		{"date", "hall 35 waiting date soon", `hall date "soon"`},
		{"clan hall id", "clan 4 hall 0", `clan hall id "0"`},
		{"clan hall needs a clan", "clan 0 hall 22", "a clan of level 1 or more"},
		{"mate without a clan", "mate Chief 19 leader", "needs a clan line"},
		{"two leaders", "clan 4\nmate B 19 leader\nmate C 19 leader", "has one leader"},
		{"two sponsors", "clan 4\nmate B 19 sponsor\nmate C 19 sponsor", "one sponsor"},
		{"both relations", "clan 4\nmate B 19 sponsor apprentice", "one of sponsor and apprentice"},
		{"named twice", "clan 4\nmate B 19\nmate B 20", `mate "B" named twice`},
		{"traced mate that does not lead", "trace x.golden\nclan 4\nmate B 19 sponsor online", "takes only the mate that leads the clan"},
		{"traced hold of a mate that enters later", "trace x.golden\nclan 4\nmate B 19 leader hold 1458 5", "hold line of an online mate only"},
		{"hallstatus without hall", "advance 1s\nhallstatus 35 waiting", "no hall line names it"},
		{"as an unknown mate", "npc n 1 Folk 10 0 0\nadvance 1s\nas Nobody action n", "no mate line names it"},
		{"enter an unknown mate", "advance 1s\nenter Nobody", "no mate line names it"},
		{"to an unknown character", "advance 1s\n  to Nobody ActionFailed", "no mate line names it"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			lines := tc.lines
			if !strings.Contains(lines, "advance 1s") {
				lines += "\nadvance 1s"
			}
			_, err := Parse("bad.scenario", []byte("character A 20\n"+lines+"\n"))
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("err = %v, want %q", err, tc.want)
			}
		})
	}
}
