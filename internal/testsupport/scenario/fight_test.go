package scenario

import (
	"reflect"
	"slices"
	"strings"
	"testing"

	actorai "github.com/fatal10110/acis_golang/internal/gameserver/model/actor/ai"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/npc"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
)

const fightSample = `character Hunter 20
skill 1068 1
skill 4694 1
door gate 19210001 100 0 0
summon golem 14737
npc knight 35235 SiegeGuard 100 0 -150 walks clan aden_siege_clan,door_clan 300
npc cleric 35237 SiegeGuard 100 150 0 clan aden_siege_clan 300 skill MAGIC_HEAL 4115 1 skill DD_MAGIC 4114 1
npc monster 20999 Monster 180 30 0

hit knight 22
  hate knight 150
  hate knight 0 golem
  desires knight attack(player)=150 flee(player,150)=30
hit gate 100 by golem
  desires cleric -
hit knight 10 by monster
cast monster 4694
  at knight 100 0 -150
  at player 0 0 0
jump knight 800 0 0
away
approach knight 50
approach cleric
`

// TestParseReadsFightLines: the skill, door and summon lines, an npc line's
// clan and skill options, the hit (by another creature, on a door), cast,
// jump, away and approach-to-a-distance steps, and the hate, desires and at
// checks.
func TestParseReadsFightLines(t *testing.T) {
	sc, err := Parse("fight.scenario", []byte(fightSample))
	if err != nil {
		t.Fatal(err)
	}
	if want := []modelskill.Ref{{ID: 1068, Level: 1}, {ID: 4694, Level: 1}}; !slices.Equal(sc.Skills, want) {
		t.Fatalf("skills = %+v", sc.Skills)
	}
	if want := []Door{{Role: "gate", ID: 19210001, DX: 100}}; !slices.Equal(sc.Doors, want) {
		t.Fatalf("doors = %+v", sc.Doors)
	}
	if sc.Summon == nil || *sc.Summon != (Summon{Role: "golem", ID: 14737}) {
		t.Fatalf("summon = %+v", sc.Summon)
	}
	wantNPCs := []NPC{
		{Role: "knight", ID: 35235, Type: "SiegeGuard", DX: 100, DZ: -150, Walks: true, Clans: []string{"aden_siege_clan", "door_clan"}, ClanRange: 300},
		{
			Role: "cleric", ID: 35237, Type: "SiegeGuard", DX: 100, DY: 150, Clans: []string{"aden_siege_clan"}, ClanRange: 300,
			Skills: map[npc.SkillType]modelskill.Ref{npc.SkillTypeMagicHeal: {ID: 4115, Level: 1}, npc.SkillTypeDDMagic: {ID: 4114, Level: 1}},
		},
		{Role: "monster", ID: 20999, Type: "Monster", DX: 180, DY: 30},
	}
	if !reflect.DeepEqual(sc.NPCs, wantNPCs) {
		t.Fatalf("npcs = %+v", sc.NPCs)
	}
	var verbs []string
	for _, st := range sc.Steps {
		verbs = append(verbs, st.String())
	}
	wantVerbs := []string{
		"hit knight 22", "hit gate 100 by golem", "hit knight 10 by monster", "cast monster 4694",
		"jump knight 800 0 0", "away", "approach knight 50", "approach cleric",
	}
	if !slices.Equal(verbs, wantVerbs) {
		t.Fatalf("steps = %q, want %q", verbs, wantVerbs)
	}
	var checks []string
	for _, st := range sc.Steps {
		for _, ck := range st.Checks {
			checks = append(checks, ck.Kind+" "+strings.Join(ck.Args, " "))
		}
	}
	wantChecks := []string{
		"hate knight 150", "hate knight 0 golem", "desires knight attack(player)=150 flee(player,150)=30",
		"desires cleric -", "at knight 100 0 -150", "at player 0 0 0",
	}
	if !slices.Equal(checks, wantChecks) {
		t.Fatalf("checks = %q, want %q", checks, wantChecks)
	}
	if tmpl := template(sc.NPCs[1]); tmpl.ClanRange != 300 || !slices.Equal(tmpl.Clans, []string{"aden_siege_clan"}) ||
		tmpl.SkillsByType[npc.SkillTypeMagicHeal] != (modelskill.Ref{ID: 4115, Level: 1}) || tmpl.Skills[4114] != 1 || tmpl.MPMax != 1000 {
		t.Fatalf("cleric template = %+v", tmpl)
	}
}

func TestParseRejectsMalformedFightLines(t *testing.T) {
	const npcs = "character A 1\nnpc a 1 Monster 0 0 0\nnpc b 2 Monster 0 0 0\n"
	for _, tc := range []struct{ name, text, want string }{
		{"skill usage", "character A 1\nskill 1068\nadvance 1s\n", "skill <skill id> <level>"},
		{"skill level", "character A 1\nskill 1068 0\nadvance 1s\n", "not a number of 1 or more"},
		{"skill twice", "character A 1\nskill 1068 1\nskill 1068 2\nadvance 1s\n", "listed twice"},
		{"door usage", "character A 1\ndoor g 1 0 0\nadvance 1s\n", "door <role>"},
		{"door twice", "character A 1\ndoor g 1 0 0 0\ndoor h 1 0 0 0\nadvance 1s\n", "listed twice"},
		{"door names a role", "character A 1\nnpc g 1 Folk 0 0 0\ndoor g 1 0 0 0\nadvance 1s\n", `role "g" named twice`},
		{"summon twice", "character A 1\nsummon s 1\nsummon t 1\nadvance 1s\n", "one summon line"},
		{"summon names a door", "character A 1\ndoor s 1 0 0 0\nsummon s 1\nadvance 1s\n", `role "s" named twice`},
		{"npc clan usage", "character A 1\nnpc a 1 Monster 0 0 0 clan c\nadvance 1s\n", "clan <name>"},
		{"npc clan range", "character A 1\nnpc a 1 Monster 0 0 0 clan c -1\nadvance 1s\n", "not a number of 0 or more"},
		{"npc clan twice", "character A 1\nnpc a 1 Monster 0 0 0 clan c 1 clan d 1\nadvance 1s\n", "once"},
		{"npc skill type", "character A 1\nnpc a 1 Monster 0 0 0 skill PASSIVE 1 1\nadvance 1s\n", "no type a behavior casts"},
		{"npc skill twice", "character A 1\nnpc a 1 Monster 0 0 0 skill BUFF 1 1 skill BUFF 2 1\nadvance 1s\n", "listed twice"},
		{"hit unknown target", npcs + "hit c 1\n", `unknown role "c"`},
		{"hit by usage", npcs + "hit a 1 from b\n", "hit <role> <damage> [by <role>]"},
		{"hit by itself", npcs + "hit a 1 by a\n", `hit by "a"`},
		{"hit by unknown", npcs + "hit a 1 by c\n", `hit by "c"`},
		{"cast unlisted skill", npcs + "cast a 1068\n", "no skill line lists it"},
		{"cast unknown role", "character A 1\nskill 1068 1\ncast a 1068\n", `unknown role "a"`},
		{"jump unknown role", "character A 1\njump a 0 0 0\n", `unknown role "a"`},
		{"approach distance", npcs + "approach a 0\n", "not a number of 1 or more"},
		{"hate usage", npcs + "advance 1s\n  hate a\n", "hate <role> <weight>"},
		{"hate weight", npcs + "advance 1s\n  hate a x\n", "is not a number"},
		{"hate unknown creature", npcs + "advance 1s\n  hate a 1 c\n", `unknown role "c"`},
		{"desires token", npcs + "advance 1s\n  desires a attack\n", "is not <kind>(<args>)=<weight>"},
		{"desires unknown role", npcs + "advance 1s\n  desires c -\n", `unknown role "c"`},
		{"at usage", npcs + "advance 1s\n  at a 0 0\n", "at <role> <dx> <dy> <dz>"},
		{"at unknown role", npcs + "advance 1s\n  at c 0 0 0\n", `unknown role "c"`},
		{"traced skill line", "trace x.golden\ncharacter A 1\nskill 1068 1\nadvance 1s\n", "takes no skill or summon line"},
		{"traced npc clan", "trace x.golden\ncharacter A 1\nnpc a 1 Monster 0 0 0 clan c 1\nadvance 1s\n", "takes no clan or skill option"},
		{"traced hit by", "trace x.golden\n" + npcs + "hit a 1 by b\n", "takes no hit by another creature"},
		{"summon relog", "character A 1\nsummon s 1\nrelog\n", "takes no relog"},
		{"created door", "character A 1\ndoor g 1 0 0 0\ncreate\nenter\n", "no skill, door or summon line"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, err := Parse("bad.scenario", []byte(tc.text))
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("Parse error = %v, want one containing %q", err, tc.want)
			}
		})
	}
}

// TestDesireRendersEachKindsArguments: a desire renders as its kind, the
// arguments its kind carries and its weight, a creature by its role.
func TestDesireRendersEachKindsArguments(t *testing.T) {
	r := &run{origin: location.Location{X: 10, Y: 20, Z: -100}, roles: map[int32]string{}}
	for _, tc := range []struct {
		d    actorai.Desire
		want string
	}{
		{actorai.Desire{Kind: actorai.IntentionAttack, Weight: 136.36363636363635}, "attack(-)=136.36363636363635"},
		{actorai.Desire{Kind: actorai.IntentionFlee, Distance: 150, Weight: 30}, "flee(-,150)=30"},
		{actorai.Desire{Kind: actorai.IntentionCast, Skill: modelskill.Ref{ID: 4115, Level: 1}, MoveToTarget: true, Weight: 1e6}, "cast(-,4115,move)=1000000"},
		{actorai.Desire{Kind: actorai.IntentionCast, Skill: modelskill.Ref{ID: 4114, Level: 1}, Weight: 1e6}, "cast(-,4114)=1000000"},
		{actorai.Desire{Kind: actorai.IntentionMoveTo, Location: location.Location{X: 710, Y: 20, Z: -100}, Weight: 30}, "move_to(700,0,0)=30"},
		{actorai.Desire{Kind: actorai.IntentionNothing, Timer: 40, Weight: 28.5}, "nothing(40)=28.5"},
		{actorai.Desire{Kind: actorai.IntentionWander, Weight: 1}, "wander()=1"},
	} {
		if got := r.desire(tc.d); got != tc.want || !validDesire(got) {
			t.Errorf("desire %+v = %q (valid %v), want %q", tc.d, got, validDesire(got), tc.want)
		}
	}
}
