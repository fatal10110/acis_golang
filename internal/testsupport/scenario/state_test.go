package scenario

import (
	"strings"
	"testing"
	"time"
)

const stateScenario = `character A 20
npc boss 100 Monster 100 0 0 level 60 aggro 600 mem i_ai0=3,c_ai0=player,value=7 privates 200:2:5000,201
npc guard 200 Monster 150 0 0 master boss mem c_quest1=boss
random 1 2
hit boss 10 by guard skill 4515 3
  random 0
  mem boss i_ai0=3 c_ai0=player value=7
  hp boss 430
  privates boss 200=1 -
  master guard boss
  top boss -
  effect player 4515 held
  dead player no
  stance boss on
hp player 5
die guard
top boss guard
land boss 4515 3 by guard
stance boss off
swing guard
`

func TestParseReadsStateOptionsStepsAndChecks(t *testing.T) {
	sc, err := Parse("state.scenario", []byte(strings.ReplaceAll(stateScenario, "privates boss 200=1 -", "privates boss 200=1")))
	if err != nil {
		t.Fatal(err)
	}
	boss, guard := sc.NPCs[0], sc.NPCs[1]
	if boss.Level != 60 || boss.Aggro != 600 || len(boss.Mem) != 3 || boss.Mem[1] != (MemSet{"c_ai0", "player"}) {
		t.Fatalf("boss = %+v, want level 60, aggro 600 and three memory slots", boss)
	}
	if len(boss.Privates) != 2 || boss.Privates[0].NpcID != 200 || boss.Privates[0].Weight != 2 ||
		boss.Privates[0].RespawnDelay != 5*time.Second || boss.Privates[1].Weight != 1 {
		t.Fatalf("boss privates = %+v, want 200 weight 2 respawn 5s and 201 weight 1", boss.Privates)
	}
	if guard.Master != "boss" {
		t.Fatalf("guard master = %q, want boss", guard.Master)
	}
	hit := sc.Steps[0]
	if h, err := parseHit(hit.Args); err != nil || h.by != "guard" || !h.hasSkill || h.skill.ID != 4515 || h.skill.Level != 3 {
		t.Fatalf("hit = %+v, %v, want by guard with skill 4515 level 3", h, err)
	}
	if hit.Random == nil || hit.Random.Draws[0] != 0 || len(hit.Checks) != 8 {
		t.Fatalf("first step random %+v and %d checks, want its own draw 0 and eight checks", hit.Random, len(hit.Checks))
	}
}

func TestParseRejectsBadStateLines(t *testing.T) {
	const head = "character A 1\nnpc a 1 Monster 0 0 0\n"
	for _, c := range []struct{ name, text, want string }{
		{"level", "character A 1\nnpc a 1 Monster 0 0 0 level 0\nadvance 1s\n", "level <n>"},
		{"aggro twice", "character A 1\nnpc a 1 Monster 0 0 0 aggro 5 aggro 6\nadvance 1s\n", "aggro <range>"},
		{"mem slot", "character A 1\nnpc a 1 Monster 0 0 0 mem i_ai5=1\nadvance 1s\n", "is not <slot>=<value>"},
		{"mem number", "character A 1\nnpc a 1 Monster 0 0 0 mem i_ai0=x\nadvance 1s\n", "is not a number"},
		{"mem creature", "character A 1\nnpc a 1 Monster 0 0 0 mem c_ai0=nobody\nadvance 1s\n", "is not player"},
		{"mem twice", "character A 1\nnpc a 1 Monster 0 0 0 mem i_ai0=1,i_ai0=2\nadvance 1s\n", "listed twice"},
		{"private", "character A 1\nnpc a 1 Monster 0 0 0 privates x\nadvance 1s\n", "is not <npc id>"},
		{"master below", "character A 1\nnpc a 1 Monster 0 0 0 master b\nnpc b 2 Monster 0 0 0\nadvance 1s\n", "no npc line above"},
		{"hit skill", head + "hit a 5 skill 4515\n", "hit <role> <damage>"},
		{"hit by self", head + "hit a 5 by a\n", "no other npc, summon or member line"},
		{"hp role", head + "hp b 5\n", "unknown role"},
		{"hp number", head + "hp a -1\n", "0 or more"},
		{"die by", head + "die b\n", "no npc line names it"},
		{"top creature", head + "top a b\n", "no npc, summon or member line"},
		{"land by", head + "land a 4515 1 with player\n", "land <target>"},
		{"land level", head + "land a 4515 0\n", "skill level"},
		{"stance word", head + "stance a maybe\n", "stance <role|player>"},
		{"swing role", head + "swing b\n", "unknown role"},
		{"mem check", head + "advance 1s\n  mem a i_ai9=1\n", "is not <slot>=<value>"},
		{"privates check", head + "advance 1s\n  privates a 2=0\n", "1 or more"},
		{"privates params", head + "advance 1s\n  privates a 2=1:1,2\n", "<p1>,<p2>,<p3>"},
		{"effect check", head + "advance 1s\n  effect a 4515 maybe\n", "effect <role|player>"},
		{"check role", head + "advance 1s\n  master a b\n", "unknown role"},
		{"step random twice", head + "advance 1s\n  random 1\n  random 2\n", "second random line"},
		{"traced aggro", "trace x.golden\ncharacter A 1\nnpc a 1 Monster 0 0 0 aggro 5\nadvance 1s\n", "traced scenario takes no aggro"},
		{"traced level unfought", "trace x.golden\ncharacter A 1\nnpc a 1 Monster 0 0 0 level 5\nadvance 1s\n", "no kill or hit step names"},
		{"traced hp", "trace x.golden\ncharacter A 1\nnpc a 1 Monster 0 0 0\nhp a 5\n", "traced scenario takes no hp"},
		{"traced skill hit", "trace x.golden\ncharacter A 1\nnpc a 1 Monster 0 0 0\nhit a 5 skill 4515 1\n", "traced scenario takes no hp"},
		{"traced step random", "trace x.golden\ncharacter A 1\nnpc a 1 Monster 0 0 0\nadvance 1s\n  random 1\n", "traced scenario takes no hp"},
	} {
		if _, err := Parse("bad.scenario", []byte(c.text)); err == nil || !strings.Contains(err.Error(), c.want) {
			t.Errorf("%s: err = %v, want one containing %q", c.name, err, c.want)
		}
	}
}

func TestDrawsServeAStepsOwnDrawsThenTheLinesBack(t *testing.T) {
	d := &draws{random: &Random{Draws: []int{1, 2}}}
	if got := d.draw(5); got != 1 {
		t.Fatalf("first draw = %d, want 1", got)
	}
	d.push(&Random{Draws: []int{3, 4}})
	if got := d.draw(5); got != 3 {
		t.Fatalf("step draw = %d, want 3", got)
	}
	if n := d.pop(); n != 1 {
		t.Fatalf("unused step draws = %d, want 1", n)
	}
	if got := d.draw(5); got != 2 || d.unused() != 0 {
		t.Fatalf("line draw after the step = %d (unused %d), want 2 (0)", got, d.unused())
	}
	if d := (&draws{}); d.draw(1) != 0 || d.unused() != 0 {
		t.Fatal("a source with no draws must draw at random and owe none")
	}
}
