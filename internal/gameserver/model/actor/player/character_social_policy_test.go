package player

import (
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/handler/target"
	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/skill/effect"
	"github.com/fatal10110/acis_golang/internal/gameserver/task"
)

// policyGraph is a fixed SocialGraph: party ids by player, command channel
// ids by party, alliance and leader by clan, and declared wars.
type policyGraph struct {
	party   map[int32]int
	channel map[int]int
	ally    map[int32]int32
	leader  map[int32]int32
	wars    map[[2]int32]bool
}

func (g policyGraph) InParty(id int32) bool { return g.party[id] != 0 }

func (g policyGraph) SameParty(a, b int32) bool {
	return g.party[a] != 0 && g.party[a] == g.party[b]
}

func (g policyGraph) SameChannel(a, b int32) bool {
	ca, cb := g.channel[g.party[a]], g.channel[g.party[b]]
	return ca != 0 && ca == cb
}

func (g policyGraph) AllyID(clanID int32) int32         { return g.ally[clanID] }
func (g policyGraph) ClanLeaderID(clanID int32) int32   { return g.leader[clanID] }
func (g policyGraph) AtWar(clanID, targetID int32) bool { return g.wars[[2]int32{clanID, targetID}] }

// cursedPeer is a player holding a cursed weapon; Character itself never
// does until cursed weapons are modeled (#225).
type cursedPeer struct{ *Character }

func (cursedPeer) CursedWeaponEquipped() bool { return true }

type policyZones struct{ pvp, siege, peace bool }

type policySide struct {
	clan    int32
	level   int
	karma   int
	flag    task.PvPFlagState
	blessed bool
	zones   policyZones
}

// policyPair builds caster (id 1) and target (id 2) over a shared graph.
// Players 1 and 2 sit in parties 1 and 2 unless the graph says otherwise.
func policyPair(t *testing.T, g policyGraph, cs, ts policySide) (*Character, *Character) {
	t.Helper()
	build := func(id int32, s policySide) *Character {
		c := &Character{ID: id}
		attachTestLive(t, c)
		c.social = g
		c.SetClanID(s.clan)
		c.CharLevel = s.level
		if c.CharLevel == 0 {
			c.CharLevel = 40
		}
		c.KarmaPoints = s.karma
		c.pvpFlag = s.flag
		c.SetInPvPZone(s.zones.pvp)
		c.SetInSiegeZone(s.zones.siege)
		c.SetInPeaceZone(s.zones.peace)
		if s.blessed {
			e, err := effect.New(effect.Skill{ID: 1323, Level: 1}, modelskill.EffectTemplate{Name: "ProtectionBlessing", Time: 30})
			if err != nil {
				t.Fatalf("effect.New(ProtectionBlessing): %v", err)
			}
			e.Effector, e.Effected = c, c
			c.EffectList().Add(e)
			if !c.ProtectionBlessing() {
				t.Fatal("Blessing of Protection did not land")
			}
		}
		return c
	}
	return build(1, cs), build(2, ts)
}

var (
	policyDebuff = &modelskill.Definition{SkillType: "DEBUFF", Debuff: true}
	policyDamage = &modelskill.Definition{SkillType: "MDAM"}
)

var (
	arena     = policyZones{pvp: true}
	siegePvP  = policyZones{pvp: true, siege: true}
	peaceZone = policyZones{peace: true}
)

func samePartyGraph() policyGraph {
	return policyGraph{party: map[int32]int{1: 7, 2: 7}}
}

func sameChannelGraph() policyGraph {
	return policyGraph{party: map[int32]int{1: 7, 2: 8}, channel: map[int]int{7: 3, 8: 3}}
}

func warGraph(mutual bool) policyGraph {
	g := policyGraph{wars: map[[2]int32]bool{{10, 20}: true}}
	if mutual {
		g.wars[[2]int32{20, 10}] = true
	}
	return g
}

func TestOffensiveCastPolicy(t *testing.T) {
	clanmates := policyGraph{}
	allies := policyGraph{ally: map[int32]int32{10: 5, 20: 5}}
	cases := []struct {
		name    string
		g       policyGraph
		cs, ts  policySide
		cursed  bool
		skill   *modelskill.Definition
		ctrl    bool
		allowed bool
	}{
		{name: "white stranger refuses a debuff", skill: policyDebuff},
		{name: "white stranger takes a ctrl damage skill", skill: policyDamage, ctrl: true, allowed: true},
		{name: "white stranger refuses a plain damage skill", skill: policyDamage},

		{name: "arena strangers fight freely", cs: policySide{zones: arena}, ts: policySide{zones: arena}, skill: policyDebuff, allowed: true},
		{name: "arena needs both inside", cs: policySide{zones: arena}, skill: policyDebuff},
		{name: "arena party mate refuses a debuff", g: samePartyGraph(), cs: policySide{zones: arena}, ts: policySide{zones: arena}, skill: policyDebuff},
		{name: "arena party mate refuses a plain damage skill", g: samePartyGraph(), cs: policySide{zones: arena}, ts: policySide{zones: arena}, skill: policyDamage},
		{name: "arena party mate takes a ctrl damage skill", g: samePartyGraph(), cs: policySide{zones: arena}, ts: policySide{zones: arena}, skill: policyDamage, ctrl: true, allowed: true},
		{name: "arena channel mate refuses a debuff", g: sameChannelGraph(), cs: policySide{zones: arena}, ts: policySide{zones: arena}, skill: policyDebuff},

		{name: "both in a siege pvp zone fight freely", cs: policySide{zones: siegePvP}, ts: policySide{zones: siegePvP}, skill: policyDebuff, allowed: true},
		{name: "pvp zone needs both inside", cs: policySide{zones: siegePvP}, skill: policyDebuff},
		{name: "pvp zone keeps party mates protected", g: samePartyGraph(), cs: policySide{zones: siegePvP}, ts: policySide{zones: siegePvP}, skill: policyDebuff},

		{name: "channel mate refuses a debuff", g: sameChannelGraph(), skill: policyDebuff, ctrl: true},
		{name: "channel mate takes a ctrl damage skill", g: sameChannelGraph(), skill: policyDamage, ctrl: true, allowed: true},
		{name: "clanmate refuses a flagged debuff", g: clanmates, cs: policySide{clan: 10}, ts: policySide{clan: 10, flag: task.PvPFlagOn}, skill: policyDebuff, ctrl: true},
		{name: "clanmate takes a ctrl damage skill", g: clanmates, cs: policySide{clan: 10}, ts: policySide{clan: 10}, skill: policyDamage, ctrl: true, allowed: true},
		{name: "ally refuses a debuff", g: allies, cs: policySide{clan: 10}, ts: policySide{clan: 20, karma: 100}, skill: policyDebuff, ctrl: true},

		{name: "flagged target takes a debuff", ts: policySide{flag: task.PvPFlagOn}, skill: policyDebuff, allowed: true},
		{name: "karma target takes a debuff", ts: policySide{karma: 100}, skill: policyDebuff, allowed: true},

		{name: "blessed target shields from a PK ten levels up", cs: policySide{level: 50, karma: 100}, ts: policySide{level: 40, blessed: true, flag: task.PvPFlagOn}, skill: policyDebuff},
		{name: "blessed target open to a PK nine levels up", cs: policySide{level: 49, karma: 100}, ts: policySide{level: 40, blessed: true, flag: task.PvPFlagOn}, skill: policyDebuff, allowed: true},
		{name: "blessed target open to a white caster ten levels up", cs: policySide{level: 50}, ts: policySide{level: 40, blessed: true, flag: task.PvPFlagOn}, skill: policyDebuff, allowed: true},
		{name: "blessed target open to a PK ten levels down", cs: policySide{level: 30, karma: 100}, ts: policySide{level: 40, blessed: true, flag: task.PvPFlagOn}, skill: policyDebuff, allowed: true},
		{name: "blessed caster may not hit a PK ten levels up", cs: policySide{level: 40, blessed: true}, ts: policySide{level: 50, karma: 100}, skill: policyDebuff},
		{name: "blessed caster may hit a PK nine levels up", cs: policySide{level: 40, blessed: true}, ts: policySide{level: 49, karma: 100}, skill: policyDebuff, allowed: true},
		{name: "blessed caster may hit a PK ten levels down", cs: policySide{level: 40, blessed: true}, ts: policySide{level: 30, karma: 100}, skill: policyDebuff, allowed: true},

		{name: "cursed target out of reach at level 20", cs: policySide{level: 20}, ts: policySide{karma: 100}, cursed: true, skill: policyDebuff},
		{name: "cursed target in reach at level 21", cs: policySide{level: 21}, ts: policySide{karma: 100}, cursed: true, skill: policyDebuff, allowed: true},

		{name: "mutual war debuff with ctrl", g: warGraph(true), cs: policySide{clan: 10}, ts: policySide{clan: 20}, skill: policyDebuff, ctrl: true, allowed: true},
		{name: "mutual war debuff without ctrl", g: warGraph(true), cs: policySide{clan: 10}, ts: policySide{clan: 20}, skill: policyDebuff},
		{name: "mutual war damage without ctrl", g: warGraph(true), cs: policySide{clan: 10}, ts: policySide{clan: 20}, skill: policyDamage},
		{name: "one-sided war debuff with ctrl", g: warGraph(false), cs: policySide{clan: 10}, ts: policySide{clan: 20}, skill: policyDebuff, ctrl: true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			c, tgt := policyPair(t, tc.g, tc.cs, tc.ts)
			var victim target.Actor = tgt
			if tc.cursed {
				victim = cursedPeer{tgt}
			}
			got := c.CanCastOnPlayable(victim, tc.skill, tc.ctrl, true)
			if got != tc.allowed {
				t.Fatalf("offensive cast allowed = %v, want %v", got, tc.allowed)
			}
		})
	}
}

func TestOffensiveCastRefusesSelf(t *testing.T) {
	c, _ := policyPair(t, policyGraph{}, policySide{flag: task.PvPFlagOn}, policySide{})
	if c.CanCastOnPlayable(c, policyDamage, true, true) {
		t.Fatal("offensive cast on self allowed, want refused")
	}
}

func TestBeneficialCastPolicy(t *testing.T) {
	cases := []struct {
		name    string
		g       policyGraph
		cs, ts  policySide
		ctrl    bool
		allowed bool
	}{
		{name: "white stranger", allowed: true},
		{name: "flagged stranger needs ctrl", ts: policySide{flag: task.PvPFlagOn}},
		{name: "flagged stranger with ctrl", ts: policySide{flag: task.PvPFlagOn}, ctrl: true, allowed: true},
		{name: "karma stranger needs ctrl", ts: policySide{karma: 100}},
		{name: "flagged party mate", g: samePartyGraph(), ts: policySide{flag: task.PvPFlagOn}, allowed: true},
		{name: "flagged channel mate", g: sameChannelGraph(), ts: policySide{flag: task.PvPFlagOn}, allowed: true},
		{name: "flagged clanmate", cs: policySide{clan: 10}, ts: policySide{clan: 10, flag: task.PvPFlagOn}, allowed: true},
		{name: "flagged ally", g: policyGraph{ally: map[int32]int32{10: 5, 20: 5}}, cs: policySide{clan: 10}, ts: policySide{clan: 20, flag: task.PvPFlagOn}, allowed: true},

		{name: "flagged target in pvp zone from pvp zone", cs: policySide{zones: arena}, ts: policySide{zones: arena, flag: task.PvPFlagOn}, allowed: true},
		{name: "target in pvp zone from peace zone needs ctrl", cs: policySide{zones: peaceZone}, ts: policySide{zones: arena}},
		{name: "party mate in pvp zone from peace zone needs ctrl", g: samePartyGraph(), cs: policySide{zones: peaceZone}, ts: policySide{zones: arena}},
		{name: "target in pvp zone from peace zone with ctrl", cs: policySide{zones: peaceZone}, ts: policySide{zones: arena}, ctrl: true, allowed: true},
		{name: "flagged target in pvp zone from outside needs ctrl", ts: policySide{zones: arena, flag: task.PvPFlagOn}},
		{name: "white target in pvp zone from outside", ts: policySide{zones: arena}, allowed: true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			c, tgt := policyPair(t, tc.g, tc.cs, tc.ts)
			if got := c.CanCastOnPlayable(tgt, policyDebuff, tc.ctrl, false); got != tc.allowed {
				t.Fatalf("beneficial cast allowed = %v, want %v", got, tc.allowed)
			}
		})
	}
}

func TestSocialWithoutForcePolicy(t *testing.T) {
	cases := []struct {
		name             string
		g                policyGraph
		self, attacker   policySide
		allowed, decided bool
		withoutForce     bool
	}{
		{name: "strangers fall through"},
		{name: "flagged stranger fall through to the flag", self: policySide{flag: task.PvPFlagOn}, withoutForce: true},
		{name: "arena strangers", self: policySide{zones: arena}, attacker: policySide{zones: arena}, allowed: true, decided: true, withoutForce: true},
		{name: "arena needs both inside", self: policySide{zones: arena}},
		{name: "arena party mates need force", g: samePartyGraph(), self: policySide{zones: arena}, attacker: policySide{zones: arena}, decided: true},
		{name: "arena channel mates need force", g: sameChannelGraph(), self: policySide{zones: arena}, attacker: policySide{zones: arena}, decided: true},
		{name: "party mate needs force", g: samePartyGraph(), decided: true},
		{name: "flagged clanmate needs force", self: policySide{clan: 10, flag: task.PvPFlagOn}, attacker: policySide{clan: 10}, decided: true},
		{name: "karma ally needs force", g: policyGraph{ally: map[int32]int32{10: 5, 20: 5}}, self: policySide{clan: 10, karma: 100}, attacker: policySide{clan: 20}, decided: true},
		{name: "clanmates in a siege pvp zone need force", self: policySide{clan: 10, zones: siegePvP}, attacker: policySide{clan: 10, zones: siegePvP}, decided: true},
		{name: "mutual war still needs force", g: warGraph(true), self: policySide{clan: 20}, attacker: policySide{clan: 10}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			self, attacker := policyPair(t, tc.g, tc.self, tc.attacker)
			allowed, decided := self.SocialWithoutForce(self, attacker)
			if allowed != tc.allowed || decided != tc.decided {
				t.Fatalf("SocialWithoutForce = (%v, %v), want (%v, %v)", allowed, decided, tc.allowed, tc.decided)
			}
			if got := self.AttackableWithoutForceBy(attacker); got != tc.withoutForce {
				t.Fatalf("AttackableWithoutForceBy = %v, want %v", got, tc.withoutForce)
			}
		})
	}
}
