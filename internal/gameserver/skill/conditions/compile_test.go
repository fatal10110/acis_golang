package conditions

import (
	"fmt"
	"testing"

	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
)

// creature is a non-player condition view: every Actor method, plus the
// seed and force lookups ElementSeed and ForceBuff read.
type creature struct {
	level          int
	hp, mp         float64
	x, y, z        int
	moving, riding bool
	skills         map[int]int
	effects        map[int]int
	seeds          map[int]int
	forces         map[int]int
}

func (c *creature) Level() int                 { return c.level }
func (c *creature) HPRatio() float64           { return c.hp }
func (c *creature) MPRatio() float64           { return c.mp }
func (c *creature) X() int                     { return c.x }
func (c *creature) Y() int                     { return c.y }
func (c *creature) Z() int                     { return c.z }
func (c *creature) IsMoving() bool             { return c.moving }
func (c *creature) IsRunning() bool            { return false }
func (c *creature) IsRiding() bool             { return c.riding }
func (c *creature) IsFlying() bool             { return false }
func (c *creature) CurrentHeading() int        { return 0 }
func (c *creature) IsBehind(Actor) bool        { return false }
func (c *creature) IsInFrontOf(Actor) bool     { return false }
func (c *creature) IsNight() bool              { return false }
func (c *creature) SeedPower(effectID int) int { return c.seeds[effectID] }

func (c *creature) ActiveSkillLevel(id int) (int, bool) {
	l, ok := c.skills[id]
	return l, ok
}

func (c *creature) ActiveEffectLevel(id int) (int, bool) {
	l, ok := c.effects[id]
	return l, ok
}

func (c *creature) ForceLevel(skillID int) (int, bool) {
	l, ok := c.forces[skillID]
	return l, ok
}

// player adds the PlayerActor surface.
type player struct {
	creature
	olympiad  bool
	clan      bool
	castleID  int
	anyCastle bool
	hallID    int
	anyHall   bool
	weight    int
	invSize   int
	invLimit  int
	charges   int
}

func (p *player) IsSitting() bool          { return false }
func (p *player) IsInOlympiadMode() bool   { return p.olympiad }
func (p *player) IsHero() bool             { return false }
func (p *player) PkKills() int             { return 0 }
func (p *player) PledgeClass() int         { return 0 }
func (p *player) IsClanLeader() bool       { return false }
func (p *player) HasClan() bool            { return p.clan }
func (p *player) ClanCastleID() int        { return p.castleID }
func (p *player) ClanHasAnyCastle() bool   { return p.anyCastle }
func (p *player) ClanHallID() int          { return p.hallID }
func (p *player) ClanHasAnyClanHall() bool { return p.anyHall }
func (p *player) Race() int                { return 0 }
func (p *player) Sex() int                 { return 0 }
func (p *player) WeightPenalty() int       { return p.weight }
func (p *player) InventorySize() int       { return p.invSize }
func (p *player) InventoryLimit() int      { return p.invLimit }
func (p *player) Charges() int             { return p.charges }
func (p *player) IsWearingType(int) bool   { return false }

// npc and door are the two id-bearing target shapes.
type npc struct {
	creature
	id, race int
}

func (n *npc) NpcID() int       { return n.id }
func (n *npc) RaceOrdinal() int { return n.race }

type door struct {
	creature
	id int
}

func (d *door) DoorID() int { return d.id }

func leaf(kind string, attrs map[string]string, children ...modelskill.Condition) modelskill.Condition {
	return modelskill.Condition{Kind: kind, Attrs: attrs, Children: children}
}

func mustCompile(t *testing.T, node modelskill.Condition) Condition {
	t.Helper()
	c, err := Compile(node)
	if err != nil {
		t.Fatalf("Compile(%+v): %v", node, err)
	}
	return c
}

type probe struct {
	name               string
	effector, effected Actor
	want               bool
}

func runProbes(t *testing.T, node modelskill.Condition, probes []probe) {
	t.Helper()
	c := mustCompile(t, node)
	for _, p := range probes {
		if got := c.Test(p.effector, p.effected, nil); got != p.want {
			t.Errorf("%s: Test = %v, want %v", p.name, got, p.want)
		}
	}
}

func TestCompileSeedAttributesFoldIntoOneElementSeedInReferenceOrder(t *testing.T) {
	c := mustCompile(t, leaf("player", map[string]string{
		"seed_fire": "1", "seed_water": "2", "seed_wind": "3", "seed_various": "4", "seed_any": "5",
	}))
	seed, ok := c.(ElementSeed)
	if !ok {
		t.Fatalf("five seed attributes compiled to %T, want one ElementSeed", c)
	}
	if want := [5]int{1, 2, 3, 4, 5}; seed.Required != want {
		t.Fatalf("Required = %v, want fire,water,wind,various,any = %v", seed.Required, want)
	}
}

func TestCompileSeedBoundaries(t *testing.T) {
	const fire, water, wind = 1285, 1286, 1287
	seeds := func(f, wa, wi int) *player {
		return &player{creature: creature{seeds: map[int]int{fire: f, water: wa, wind: wi}}}
	}
	cases := []struct {
		name   string
		attrs  map[string]string
		probes []probe
	}{
		{"seed_fire", map[string]string{"seed_fire": "2"}, []probe{
			{"fire 2", seeds(2, 0, 0), nil, true},
			{"fire 1", seeds(1, 0, 0), nil, false},
			{"water 2 only", seeds(0, 2, 0), nil, false},
		}},
		{"seed_water", map[string]string{"seed_water": "2"}, []probe{
			{"water 2", seeds(0, 2, 0), nil, true},
			{"water 1", seeds(0, 1, 0), nil, false},
			{"fire 2 only", seeds(2, 0, 0), nil, false},
		}},
		{"seed_wind", map[string]string{"seed_wind": "2"}, []probe{
			{"wind 2", seeds(0, 0, 2), nil, true},
			{"wind 1", seeds(0, 0, 1), nil, false},
			{"water 2 only", seeds(0, 2, 0), nil, false},
		}},
		{"two seeds (1292 shape)", map[string]string{"seed_water": "1", "seed_wind": "1"}, []probe{
			{"water 1 wind 1", seeds(0, 1, 1), nil, true},
			{"water 2", seeds(0, 2, 0), nil, false},
		}},
		{"seed_various counts distinct seeds left", map[string]string{"seed_various": "2"}, []probe{
			{"fire 1 wind 1", seeds(1, 0, 1), nil, true},
			{"fire 3", seeds(3, 0, 0), nil, false},
		}},
		{"seed_various after flat cost", map[string]string{"seed_fire": "1", "seed_various": "1"}, []probe{
			{"fire 1 water 1", seeds(1, 1, 0), nil, true},
			{"fire 1 only: its charge is spent", seeds(1, 0, 0), nil, false},
		}},
		{"seed_any sums what is left", map[string]string{"seed_any": "3"}, []probe{
			{"fire 2 wind 1", seeds(2, 0, 1), nil, true},
			{"fire 2", seeds(2, 0, 0), nil, false},
		}},
		{"no seed view", map[string]string{"seed_any": "1"}, []probe{
			{"npc without seeds", &npc{}, nil, false},
		}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) { runProbes(t, leaf("player", tc.attrs), tc.probes) })
	}
}

func TestCompileForceAttributes(t *testing.T) {
	const battle, spell = 5104, 5105
	forces := func(m map[int]int) *player { return &player{creature: creature{forces: m}} }

	c := mustCompile(t, leaf("player", map[string]string{"battle_force": "2", "spell_force": "3"}))
	if fb, ok := c.(ForceBuff); !ok || fb.BattleForce != 2 || fb.SpellForce != 3 {
		t.Fatalf("compiled %#v, want ForceBuff{BattleForce: 2, SpellForce: 3}", c)
	}

	runProbes(t, leaf("player", map[string]string{"battle_force": "2"}), []probe{
		{"battle 2", forces(map[int]int{battle: 2}), nil, true},
		{"battle 1", forces(map[int]int{battle: 1}), nil, false},
		{"spell 3 only", forces(map[int]int{spell: 3}), nil, false},
		{"none", forces(nil), nil, false},
	})
	runProbes(t, leaf("player", map[string]string{"spell_force": "3"}), []probe{
		{"spell 3", forces(map[int]int{spell: 3}), nil, true},
		{"spell 2", forces(map[int]int{spell: 2}), nil, false},
		{"battle 3 only", forces(map[int]int{battle: 3}), nil, false},
	})
}

func TestCompileInsidePoly(t *testing.T) {
	zone := leaf("zone", map[string]string{"shape": "NPoly", "minZ": "-100", "maxZ": "100"},
		leaf("node", map[string]string{"x": "0", "y": "0"}),
		leaf("node", map[string]string{"x": "1000", "y": "0"}),
		leaf("node", map[string]string{"x": "1000", "y": "1000"}),
		leaf("node", map[string]string{"x": "0", "y": "1000"}),
	)
	at := func(x, y, z int) *player { return &player{creature: creature{x: x, y: y, z: z}} }

	runProbes(t, leaf("player", map[string]string{"insidePoly": "true"}, zone), []probe{
		{"centre", at(500, 500, 0), nil, true},
		{"at maxZ", at(500, 500, 100), nil, true},
		{"at minZ", at(500, 500, -100), nil, true},
		{"above maxZ", at(500, 500, 101), nil, false},
		{"below minZ", at(500, 500, -101), nil, false},
		{"outside the ring", at(1500, 500, 0), nil, false},
	})
	runProbes(t, leaf("player", map[string]string{"insidePoly": "false"}, zone), []probe{
		{"centre", at(500, 500, 0), nil, false},
		{"outside the ring", at(1500, 500, 0), nil, true},
		{"above maxZ", at(500, 500, 101), nil, true},
	})

	if _, err := Compile(leaf("player", map[string]string{"insidePoly": "true"})); err == nil {
		t.Error("insidePoly with no <zone> child compiled, want an error")
	}
}

func TestCompileTargetAttributes(t *testing.T) {
	caster := &player{}
	hp := func(r float64) *npc { return &npc{creature: creature{hp: r}} }

	// Binary-exact ratios keep the percentage product free of rounding.
	runProbes(t, leaf("target", map[string]string{"hp_min_max": "25,75"}), []probe{
		{"at min", caster, hp(0.25), true},
		{"at max", caster, hp(0.75), true},
		{"below min", caster, hp(0.2421875), false},
		{"above max", caster, hp(0.7578125), false},
		{"no target", caster, nil, false},
	})
	runProbes(t, leaf("target", map[string]string{"npcId": "22215, 22216"}), []probe{
		{"listed npc", caster, &npc{id: 22216}, true},
		{"unlisted npc", caster, &npc{id: 22217}, false},
		{"door by door id", caster, &door{id: 22215}, true},
		{"unlisted door", caster, &door{id: 1}, false},
		{"player target", caster, &player{}, false},
		{"no target", caster, nil, false},
	})
	runProbes(t, leaf("target", map[string]string{"race_id": "6,8"}), []probe{
		{"race 8", caster, &npc{race: 8}, true},
		{"race 7", caster, &npc{race: 7}, false},
		{"player target", caster, &player{}, false},
		{"no target", caster, nil, false},
	})
	runProbes(t, leaf("target", map[string]string{"active_skill_id": "1013"}), []probe{
		{"target knows it", caster, &npc{creature: creature{skills: map[int]int{1013: 1}}}, true},
		{"target has it only as an effect", caster, &npc{creature: creature{effects: map[int]int{1013: 1}}}, false},
		{"no target", caster, nil, false},
	})
}

func TestCompileClanOwnership(t *testing.T) {
	noClan := &player{}
	clanNoCastle := &player{clan: true}
	clanCastle3 := &player{clan: true, castleID: 3, anyCastle: true}
	clanHall22 := &player{clan: true, hallID: 22, anyHall: true}

	runProbes(t, leaf("player", map[string]string{"castle": "-1"}), []probe{
		{"clan owns a castle", clanCastle3, nil, true},
		{"clan owns none", clanNoCastle, nil, false},
		{"no clan", noClan, nil, false},
		{"non-player", &npc{}, nil, false},
	})
	runProbes(t, leaf("player", map[string]string{"castle": "0"}), []probe{
		{"no clan", noClan, nil, true},
	})
	runProbes(t, leaf("player", map[string]string{"castle": "3"}), []probe{
		{"that castle", clanCastle3, nil, true},
		{"another castle", &player{clan: true, castleID: 4, anyCastle: true}, nil, false},
	})
	runProbes(t, leaf("player", map[string]string{"clanHall": "-1"}), []probe{
		{"clan owns a hall", clanHall22, nil, true},
		{"clan owns none", clanNoCastle, nil, false},
		{"no clan", noClan, nil, false},
	})
	runProbes(t, leaf("player", map[string]string{"clanHall": "0"}), []probe{
		{"no clan", noClan, nil, true},
	})
	runProbes(t, leaf("player", map[string]string{"clanHall": "21, 22"}), []probe{
		{"listed hall", clanHall22, nil, true},
		{"unlisted hall", &player{clan: true, hallID: 23, anyHall: true}, nil, false},
	})
}

func TestCompileScalarPlayerAttributes(t *testing.T) {
	cases := []struct {
		name   string
		attrs  map[string]string
		probes []probe
	}{
		{"level is a minimum", map[string]string{"level": "40"}, []probe{
			{"40", &player{creature: creature{level: 40}}, nil, true},
			{"39", &player{creature: creature{level: 39}}, nil, false},
			{"npc 40", &npc{creature: creature{level: 40}}, nil, true},
		}},
		{"hp is a maximum percent", map[string]string{"hp": "25"}, []probe{
			{"25%", &player{creature: creature{hp: 0.25}}, nil, true},
			{"26%", &player{creature: creature{hp: 0.26}}, nil, false},
		}},
		{"mp is a maximum percent", map[string]string{"mp": "50"}, []probe{
			{"50%", &player{creature: creature{mp: 0.5}}, nil, true},
			{"51%", &player{creature: creature{mp: 0.51}}, nil, false},
		}},
		{"charges is a minimum", map[string]string{"charges": "2"}, []probe{
			{"2", &player{charges: 2}, nil, true},
			{"1", &player{charges: 1}, nil, false},
			{"non-player", &npc{}, nil, false},
		}},
		{"invSize leaves room", map[string]string{"invSize": "10"}, []probe{
			{"70 of 80", &player{invSize: 70, invLimit: 80}, nil, true},
			{"71 of 80", &player{invSize: 71, invLimit: 80}, nil, false},
			{"non-player", &npc{}, nil, true},
		}},
		{"weight is below a tier", map[string]string{"weight": "1"}, []probe{
			{"tier 0", &player{}, nil, true},
			{"tier 1", &player{weight: 1}, nil, false},
		}},
		{"olympiad", map[string]string{"olympiad": "false"}, []probe{
			{"not in olympiad", &player{}, nil, true},
			{"in olympiad", &player{olympiad: true}, nil, false},
			{"non-player", &npc{}, nil, true},
		}},
		{"riding", map[string]string{"riding": "false"}, []probe{
			{"on foot", &player{}, nil, true},
			{"mounted", &player{creature: creature{riding: true}}, nil, false},
		}},
		{"active_skill_id_lvl", map[string]string{"active_skill_id_lvl": "1315,4"}, []probe{
			{"level 4", &player{creature: creature{skills: map[int]int{1315: 4}}}, nil, true},
			{"level 3", &player{creature: creature{skills: map[int]int{1315: 3}}}, nil, false},
			{"effect only", &player{creature: creature{effects: map[int]int{1315: 9}}}, nil, false},
		}},
		{"active_effect_id", map[string]string{"active_effect_id": "4321"}, []probe{
			{"effect present", &player{creature: creature{effects: map[int]int{4321: 1}}}, nil, true},
			{"skill known only", &player{creature: creature{skills: map[int]int{4321: 1}}}, nil, false},
		}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) { runProbes(t, leaf("player", tc.attrs), tc.probes) })
	}
}

func TestCompileMultiAttributeElementIsAnd(t *testing.T) {
	node := leaf("player", map[string]string{"level": "40", "hp": "50", "seed_fire": "1"})
	ok := &player{creature: creature{level: 40, hp: 0.5, seeds: map[int]int{1285: 1}}}
	runProbes(t, node, []probe{
		{"all hold", ok, nil, true},
		{"level short", &player{creature: creature{level: 39, hp: 0.5, seeds: map[int]int{1285: 1}}}, nil, false},
		{"hp too high", &player{creature: creature{level: 40, hp: 0.75, seeds: map[int]int{1285: 1}}}, nil, false},
		{"seed missing", &player{creature: creature{level: 40, hp: 0.5}}, nil, false},
	})

	runProbes(t, leaf("target", map[string]string{"npcId": "29045", "hp_min_max": "0,50"}), []probe{
		{"right npc, low hp", ok, &npc{id: 29045, creature: creature{hp: 0.25}}, true},
		{"right npc, high hp", ok, &npc{id: 29045, creature: creature{hp: 0.75}}, false},
		{"wrong npc, low hp", ok, &npc{id: 29046, creature: creature{hp: 0.25}}, false},
	})
}

func TestCompileLogicElements(t *testing.T) {
	lvl := func(n string) modelskill.Condition { return leaf("player", map[string]string{"level": n}) }
	p40 := &player{creature: creature{level: 40}}

	runProbes(t, leaf("and", nil, lvl("30"), lvl("40")), []probe{{"both", p40, nil, true}})
	runProbes(t, leaf("and", nil, lvl("30"), lvl("50")), []probe{{"one fails", p40, nil, false}})
	runProbes(t, leaf("or", nil, lvl("50"), lvl("40")), []probe{{"one holds", p40, nil, true}})
	runProbes(t, leaf("or", nil, lvl("50"), lvl("60")), []probe{{"none holds", p40, nil, false}})
	runProbes(t, leaf("not", nil, lvl("50")), []probe{{"inverted", p40, nil, true}})
}

func TestCompileNamesMatchCaseInsensitively(t *testing.T) {
	node := leaf("AND", nil,
		leaf("Player", map[string]string{"LEVEL": "40", "InvSize": "10", "SEED_FIRE": "1"}),
		leaf("TARGET", map[string]string{"NPCID": "29045"}),
		leaf("Not", nil, leaf("player", map[string]string{"Olympiad": "true"})),
	)
	p := &player{creature: creature{level: 40, seeds: map[int]int{1285: 1}}, invSize: 70, invLimit: 80}
	runProbes(t, node, []probe{
		{"all hold", p, &npc{id: 29045}, true},
		{"seed missing", &player{creature: creature{level: 40}, invSize: 70, invLimit: 80}, &npc{id: 29045}, false},
		{"wrong target", p, &npc{id: 1}, false},
	})
}

func TestCompileRejectsWhatItCannotEvaluate(t *testing.T) {
	cases := []struct {
		name string
		node modelskill.Condition
	}{
		{"unknown player attribute", leaf("player", map[string]string{"level": "40", "bogus": "1"})},
		{"unknown target attribute", leaf("target", map[string]string{"bogus": "1"})},
		{"unknown game attribute", leaf("game", map[string]string{"bogus": "1"})},
		{"unknown using attribute", leaf("using", map[string]string{"bogus": "1"})},
		{"unknown element", leaf("bogus", map[string]string{"level": "1"})},
		{"empty target", leaf("target", nil)},
		{"player with only zero seeds", leaf("player", map[string]string{"seed_fire": "0"})},
		{"not with two children", leaf("not", nil, leaf("player", map[string]string{"level": "1"}), leaf("player", map[string]string{"level": "2"}))},
		{"bad number", leaf("player", map[string]string{"level": "forty"})},
		{"one-value hp_min_max", leaf("target", map[string]string{"hp_min_max": "25"})},
		{"bad child under and", leaf("and", nil, leaf("player", map[string]string{"level": "1"}), leaf("player", map[string]string{"bogus": "1"}))},
	}
	for _, tc := range cases {
		if c, err := Compile(tc.node); err == nil {
			t.Errorf("%s: compiled to %#v, want an error", tc.name, c)
		}
	}
}

type source struct{ actor Actor }

func (s source) ConditionActor() Actor { return s.actor }

func TestEvaluateSkillReportsFirstFailingClause(t *testing.T) {
	first := modelskill.ConditionClause{Root: leaf("player", map[string]string{"level": "40"}), MessageID: 1}
	second := modelskill.ConditionClause{Root: leaf("target", map[string]string{"npcId": "29045"}), MessageID: 2}
	def := modelskill.Definition{Conditions: []modelskill.ConditionClause{first, second}}

	caster := source{&player{creature: creature{level: 40}}}
	if _, ok := EvaluateSkill(def, caster, source{&npc{id: 29045}}); !ok {
		t.Fatal("both clauses hold, EvaluateSkill refused")
	}
	if failed, ok := EvaluateSkill(def, caster, source{&npc{id: 1}}); ok || failed.MessageID != 2 {
		t.Fatalf("wrong target: got (%+v, %v), want the second clause refused", failed, ok)
	}
	if failed, ok := EvaluateSkill(def, source{&player{}}, nil); ok || failed.MessageID != 1 {
		t.Fatalf("low level: got (%+v, %v), want the first clause refused", failed, ok)
	}

	broken := modelskill.Definition{Conditions: []modelskill.ConditionClause{{Root: leaf("player", map[string]string{"bogus": "1"}), MessageID: 3}}}
	if failed, ok := EvaluateSkill(broken, caster, nil); ok || failed.MessageID != 3 {
		t.Fatalf("uncompilable clause: got (%+v, %v), want it refused", failed, ok)
	}
	if _, ok := EvaluateSkill(def, nil, nil); ok {
		t.Fatal("nil caster passed a skill with conditions, want refused")
	}
	if _, ok := EvaluateSkill(modelskill.Definition{}, nil, nil); !ok {
		t.Fatal("skill with no conditions refused")
	}
}

// TestCompileIntegersFollowReferenceGrammar pins the condition integer
// reads to the reference (Java probe, OpenJDK 21.0.11): Integer.decode
// literals, except the two forces, which are Byte.decode and so stop at the
// signed 8-bit range, and the id lists, which skip empty entries between
// adjacent commas but reject an entry that is blank after trimming.
func TestCompileIntegersFollowReferenceGrammar(t *testing.T) {
	accepted := []struct {
		name string
		node modelskill.Condition
		want Condition
	}{
		{"octal level", leaf("player", map[string]string{"level": "010"}), Level{Level: 8}},
		{"hash hex level", leaf("player", map[string]string{"level": "#10"}), Level{Level: 16}},
		{"negative hex level", leaf("player", map[string]string{"level": "-0x10"}), Level{Level: -16}},
		{"battle force at byte max", leaf("player", map[string]string{"battle_force": "0x7f"}), ForceBuff{BattleForce: 127}},
		{"spell force octal byte max", leaf("player", map[string]string{"spell_force": "0177"}), ForceBuff{SpellForce: 127}},
		{"list skips empty entries", leaf("target", map[string]string{"npcId": "1,,0x10, 010"}), TargetNpcID{IDs: []int{1, 16, 8}}},
	}
	for _, tc := range accepted {
		c, err := Compile(tc.node)
		if err != nil {
			t.Errorf("%s: %v", tc.name, err)
			continue
		}
		if fmt.Sprintf("%#v", c) != fmt.Sprintf("%#v", tc.want) {
			t.Errorf("%s: compiled %#v, want %#v", tc.name, c, tc.want)
		}
	}

	rejected := []struct {
		name string
		node modelskill.Condition
	}{
		{"binary prefix", leaf("player", map[string]string{"level": "0b1"})},
		{"octal prefix", leaf("player", map[string]string{"level": "0o10"})},
		{"digit separator", leaf("player", map[string]string{"level": "1_0"})},
		{"sign after prefix", leaf("player", map[string]string{"level": "0x-1"})},
		{"bad octal digit", leaf("player", map[string]string{"level": "08"})},
		{"battle force past byte max", leaf("player", map[string]string{"battle_force": "128"})},
		{"spell force past byte max", leaf("player", map[string]string{"spell_force": "0x80"})},
		{"blank list entry", leaf("target", map[string]string{"npcId": "1, ,2"})},
	}
	for _, tc := range rejected {
		if c, err := Compile(tc.node); err == nil {
			t.Errorf("%s: compiled to %#v, want an error", tc.name, c)
		}
	}
}

// TestGameConditionReadsOnlyNight: the reference's <game> reader recognizes
// only "night" (DocumentBase.parseGameCondition); a "chance" attribute is
// unrecognized, exactly like any other unknown <game> attribute, so it never
// becomes a random roll. A certain chance of 100 used to compile to a roll
// that always passed.
func TestGameConditionReadsOnlyNight(t *testing.T) {
	for _, attrs := range []map[string]string{
		{"chance": "50"},
		{"chance": "100"},
		{"Chance": "0"},
	} {
		node := leaf("game", attrs)
		if c, err := Compile(node); err == nil {
			t.Errorf("<game %v> compiled to %#v, want the unsupported-attribute error of <game bogus>", attrs, c)
		}
		clause := modelskill.ConditionClause{Root: node, MessageID: 4}
		def := modelskill.Definition{Conditions: []modelskill.ConditionClause{clause}}
		if _, ok := EvaluateSkill(def, source{&player{}}, nil); ok {
			t.Errorf("<game %v>: cast allowed, want it refused like any condition that does not compile", attrs)
		}
	}

	for raw, want := range map[string]bool{"true": true, "TRUE": true, "false": false, "1": false} {
		c, err := Compile(leaf("game", map[string]string{"night": raw}))
		if err != nil {
			t.Fatalf("<game night=%q>: %v", raw, err)
		}
		if c != (GameTime{Night: want}) {
			t.Errorf("<game night=%q> compiled %#v, want GameTime{Night: %v}", raw, c, want)
		}
	}
}
