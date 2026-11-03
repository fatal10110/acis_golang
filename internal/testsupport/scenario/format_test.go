package scenario

import (
	"strings"
	"testing"

	actorai "github.com/fatal10110/acis_golang/internal/gameserver/model/actor/ai"
	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
)

const formatScenario = `character A 20
noble
teleports
npc boss 100 Monster 100 0 0 mp 250
npc guard 200 Monster 150 0 0
member Pal 20
summon pet 14001 servitor 20

mp boss 40
  mp boss 40
  reach boss no
  shows boss rhand=1 glow=15
want boss pet 100 hold
hit boss 10 by Pal
top boss Pal
  desires boss attack(pet,hold)=100 cast(player,4115:2,move)=1000000
`

func TestParseReadsTheMPWantReachShowsAndServitorLines(t *testing.T) {
	sc, err := Parse("format.scenario", []byte(formatScenario))
	if err != nil {
		t.Fatal(err)
	}
	if !sc.Noble || !sc.Teleports {
		t.Fatalf("noble %v and teleports %v, want both lines read", sc.Noble, sc.Teleports)
	}
	if boss := sc.NPCs[0]; !boss.HasMP || boss.MP != 250 {
		t.Fatalf("boss = %+v, want mp 250", boss)
	}
	if s := sc.Summon; s == nil || !s.Servitor || s.Level != 20 || s.ID != 14001 || s.Role != "pet" {
		t.Fatalf("summon = %+v, want the servitor pet 14001 of level 20", s)
	}
	if got := len(sc.Steps[0].Checks); got != 3 {
		t.Fatalf("first step has %d checks, want mp, reach and shows", got)
	}
}

func TestParseRejectsBadFormatLines(t *testing.T) {
	const head = "character A 1\nnpc a 1 Monster 0 0 0\nnpc b 2 Monster 5 0 0\nmember Pal 5\n"
	for _, c := range []struct{ name, text, want string }{
		{"mp option", "character A 1\nnpc a 1 Monster 0 0 0 mp -1\nadvance 1s\n", "mp <n>"},
		{"mp twice", "character A 1\nnpc a 1 Monster 0 0 0 mp 1 mp 2\nadvance 1s\n", "mp <n>"},
		{"mp step", head + "mp a x\n", "mp \"x\""},
		{"mp step role", head + "mp z 5\n", "unknown role"},
		{"want weight", head + "want a b 0\n", "want <role>"},
		{"want marker", head + "want a b 5 stay\n", "want <role>"},
		{"want itself", head + "want a a 5\n", "itself"},
		{"want creature", head + "want a z 5\n", "z"},
		{"want by member", head + "want a Pal 5 hold\n  desires a attack(Pal,hold)=5\n", ""},
		{"reach answer", head + "advance 1s\n  reach a maybe\n", "reach <role>"},
		{"shows field", head + "advance 1s\n  shows a foot=1\n", "not once each"},
		{"shows twice", head + "advance 1s\n  shows a glow=1 glow=2\n", "not once each"},
		{"shows none", head + "advance 1s\n  shows a\n", "shows <role>"},
		{"shows role", head + "advance 1s\n  shows z glow=1\n", "unknown role"},
		{"servitor level", head + "summon p 9 servitor 0\nadvance 1s\n", "servitor level"},
		{"summon kind", head + "summon p 9 pet 5\nadvance 1s\n", "summon <role>"},
		{"hit by nobody", head + "hit a 5 by z\n", "hit by"},
		{"hit by member", head + "hit a 5 by Pal\n", ""},
		{"noble twice", head + "noble\nnoble\nadvance 1s\n", "one noble line"},
		{"noble argument", head + "noble yes\nadvance 1s\n", "one noble line"},
		{"teleports twice", head + "teleports\nteleports\nadvance 1s\n", "one teleports line"},
		{"spawned arity", head + "advance 1s\n  spawned 5\n", "spawned <npc id>"},
		{"spawned place", head + "advance 1s\n  spawned 5 1,2,3\n", "spawned <npc id>"},
		{"traced mp", "trace x.golden\n" + head[:len("character A 1\nnpc a 1 Monster 0 0 0\n")] + "advance 1s\nmp a 5\n", "traced"},
	} {
		_, err := Parse("bad.scenario", []byte(c.text))
		switch {
		case c.want == "" && err != nil:
			t.Errorf("%s: %v, want it accepted", c.name, err)
		case c.want != "" && (err == nil || !strings.Contains(err.Error(), c.want)):
			t.Errorf("%s: error %v, want it to say %q", c.name, err, c.want)
		}
	}
}

// TestDesireInFullPinsTheLevelAndTheHold: a cast shows its skill level after
// its id, and an attack that holds its ground shows hold, in the full form; the
// short form shows neither.
func TestDesireInFullPinsTheLevelAndTheHold(t *testing.T) {
	r := &run{roles: map[int32]string{}}
	for _, tc := range []struct {
		d           actorai.Desire
		full, short string
	}{
		{actorai.Desire{Kind: actorai.IntentionCast, Skill: modelskill.Ref{ID: 4115, Level: 3}, MoveToTarget: true, Weight: 5}, "cast(-,4115:3,move)=5", "cast(-,4115,move)=5"},
		{actorai.Desire{Kind: actorai.IntentionAttack, Weight: 7}, "attack(-,hold)=7", "attack(-)=7"},
		{actorai.Desire{Kind: actorai.IntentionAttack, MoveToTarget: true, Weight: 7}, "attack(-)=7", "attack(-)=7"},
		{actorai.Desire{Kind: actorai.IntentionFlee, Distance: 9, Weight: 1}, "flee(-,9)=1", "flee(-,9)=1"},
	} {
		if full, short := r.renderDesire(tc.d, true), r.renderDesire(tc.d, false); full != tc.full || short != tc.short || !validDesire(full) {
			t.Errorf("desire %+v = %q in full, %q short (valid %v), want %q and %q", tc.d, full, short, validDesire(full), tc.full, tc.short)
		}
	}
}

// TestParseNormalizesSpawnedLines: the places of a spawned line are sorted, so
// the order a scenario lists them in does not matter.
func TestParseNormalizesSpawnedLines(t *testing.T) {
	sc, err := Parse("spawned.scenario", []byte("character A 1\nadvance 1s\n  spawned 31112 9,2 1,2\n  spawned 31112 -\n"))
	if err != nil {
		t.Fatal(err)
	}
	if got := sc.Steps[0].Spawns; len(got) != 2 || got[0] != "31112 1,2 9,2" || got[1] != "31112 -" {
		t.Fatalf("spawns = %q, want the places sorted and - kept", got)
	}
}
