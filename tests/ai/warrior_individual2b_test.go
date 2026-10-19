package ai

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/rs/zerolog"

	gamesql "github.com/fatal10110/acis_golang/internal/gameserver/data/sql"
	"github.com/fatal10110/acis_golang/internal/gameserver/data/sql/sqltest"
	gamexml "github.com/fatal10110/acis_golang/internal/gameserver/data/xml"
	actorai "github.com/fatal10110/acis_golang/internal/gameserver/model/actor/ai"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/npc"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/script"
	"github.com/fatal10110/acis_golang/internal/gameserver/script/ai/warrior"
	skillstate "github.com/fatal10110/acis_golang/internal/gameserver/skill"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
)

// The NPC ids the scenarios of the second individual behavior batch's
// second PR spawn (#187 PR 2), each a fixture monster of level 1 unless a
// scenario says otherwise.
const (
	ind2bGuardID       = int32(35412) // Agit02DoomGuardAgit
	ind2bGuardTwoID    = int32(35633) // Agit02DoomGuardAgit
	ind2bPartyKnightID = int32(29995) // Agit02DoomKnightAgitParty, bound by the test
	ind2bVampireID     = int32(35647) // Agit02VampireAgitParty
	ind2bHereticID     = int32(22147) // WarriorHereticSelfRangePhysical
	ind2bHereticAggrID = int32(22141) // WarriorHereticSelfRangePhysicalAggressive
	ind2bKnightID      = int32(35416) // Agit02DoomKnightAgit
	ind2bKnightTwoID   = int32(35637) // Agit02DoomKnightAgit
	ind2bDicorID       = int32(29009) // CorePrivateDicor
	ind2bCurseAggroID  = int32(20087) // WarriorCastingCurseAggressive
	ind2bSelfRangeSkil = modelskill.ID(9504)
	ind2bPartyKnightP  = "test.Agit02DoomKnightAgitParty"
	// ind2bAggro is the aggro points of the character's self buff in the
	// spell scenarios.
	ind2bAggro = 50
)

// ind2bBatch are the classes the PR ports, in scripts.xml order.
var ind2bBatch = []struct {
	path string
	ctor func() script.Script
}{
	{"script.ai.individual.Monster.WarriorBase.Warrior.WarriorCastingCurse.Agit02DoomGuardAgit", warrior.Agit02DoomGuardAgit},
	// Agit02DoomKnightAgitParty binds no NPC of its own; the scenarios
	// bind its hooks to a fixture id to drive them.
	{ind2bPartyKnightP, func() script.Script {
		s := warrior.Agit02DoomKnightAgitParty()
		s.NPCs = []int32{ind2bPartyKnightID}
		return s
	}},
	{"script.ai.individual.Monster.WarriorBase.Warrior.WarriorCastingCurse.Agit02VampireAgitParty", warrior.Agit02VampireAgitParty},
	{"script.ai.individual.Monster.WarriorBase.Warrior.WarriorCastingCurse.WarriorHereticSelfRangePhysical", warrior.HereticSelfRangePhysical},
	{"script.ai.individual.Monster.WarriorBase.Warrior.WarriorCastingCurse.WarriorHereticSelfRangePhysicalAggressive", warrior.HereticSelfRangePhysicalAggressive},
	{"script.ai.individual.Monster.WarriorBase.Warrior.WarriorCastingCurse.WarriorCastingCurseAggressive.Agit02DoomKnightAgit", warrior.Agit02DoomKnightAgit},
	{"script.ai.individual.Monster.WarriorBase.Warrior.WarriorCastingCurse.WarriorCastingCurseAggressive.CorePrivateDicor", warrior.CorePrivateDicor},
	{"script.ai.individual.Monster.WarriorBase.Warrior.WarriorCastingCurse.WarriorCastingCurseAggressive.WarriorCastingCurseAggressive", warrior.CastingCurseAggressive},
}

// bootIndividual2b boots the PR's classes after the party Warriors beside
// templates, with extra's options added last and its step run before the
// character enters the world, every draw 0 until a scenario queues others,
// with the director script sendEvent sends through.
func bootIndividual2b(t *testing.T, extra raidBoot, templates ...*npc.Template) *ind1World {
	t.Helper()
	list, catalog := partyScripts()
	for _, c := range ind2bBatch {
		list = append(list, script.Listing{Path: c.path})
		catalog[c.path] = c.ctor
	}
	const director = "test.Ind1Director"
	catalog[director] = func() script.Script { return script.Script{} }
	list = append(list, script.Listing{Path: director})
	kinds := map[int32]script.NPCKind{}
	for _, tmpl := range templates {
		kinds[int32(tmpl.ID)] = script.KindHostile
	}
	draws := &ind1Draws{}
	opts := append([]gameservertest.Option{
		gameservertest.WithScriptRand(draws.rand),
		gameservertest.WithNPCScripts(kinds, list, catalog),
	}, extra.opts...)
	w := bootRaid(t, raidBoot{opts: opts, ready: extra.ready}, templates...)
	return &ind1World{warriorWorld: w, draws: draws, heard: &ind1Heard{}}
}

// ind2bTemplate is ind2Template that also lists the self range physical
// skill by type.
func ind2bTemplate(id int32) *npc.Template {
	tmpl := ind2Template(id)
	tmpl.SkillsByType[npc.SkillTypeSelfRangePhysicalSpecial] = modelskill.Ref{ID: ind2bSelfRangeSkil, Level: 1}
	return tmpl
}

// ind2bSkills are ind2Skills with the self range physical skill, a
// one-target strike reaching 2000.
func ind2bSkills() *modelskill.Table {
	return modelskill.NewTable([]modelskill.Definition{
		ind1Skill(ind2DDMagicSkill, 2000, false), ind1Skill(ind2DebuffSkill, 2000, false), ind2SelfBuff(),
		ind1Skill(ind2bSelfRangeSkil, 2000, false),
	})
}

// ind2bSpellBoot is raidSpellBoot with the character's self buff carrying
// ind2bAggro aggro points.
func ind2bSpellBoot(t *testing.T) raidBoot {
	t.Helper()
	db := sqltest.SharedDB(t)
	defs := []modelskill.Definition{
		{
			ID: raidSelfBuffID, Level: 1, Activation: modelskill.ActivationActive, Target: modelskill.TargetSelf,
			SkillType: "BUFF", HitTime: 500, StaticHitTime: true, StaticReuse: true, AggroPoints: ind2bAggro,
		},
		raidCurseDefinition(),
	}
	table := modelskill.NewTable(append([]modelskill.Definition{{ID: 248, Level: 3}, {ID: 294, Level: 1}}, defs...))
	return raidBoot{
		opts: []gameservertest.Option{
			gameservertest.WithSkills(skillstate.NewPersistence(gamesql.NewSkillSaveStore(db), table, gamesql.NewCharacterSkillStore(db))),
			gameservertest.WithScriptSkills(defs...),
		},
		ready: func(srv *gameservertest.Server, objID int32) {
			if err := srv.KnownSkills.SetKnownSkill(context.Background(), objID, 0, int(raidSelfBuffID), 1); err != nil {
				t.Fatalf("seed the self buff: %v", err)
			}
		},
	}
}

// ind2bDamageHate is the hate a clan hall siege NPC of maxHP adds for a
// hit of damage: 100 for each twentieth of its maximum hit points.
func ind2bDamageHate(damage int, maxHP float64) float64 {
	return float64(damage) / maxHP / 0.05 * 100
}

// ind2bWantHold fails unless h's queued desire of kind is on the creature
// of the object id target with weight, held in place.
func ind2bWantHold(t *testing.T, when string, h *npc.Hostile, kind actorai.Intention, target int32, weight float64) {
	t.Helper()
	d, ok := desire(h, kind)
	if !ok || d.FinalTarget == nil || d.FinalTarget.ObjectID() != target || d.Weight != weight || d.MoveToTarget {
		t.Fatalf("%s: %v desire = %+v (%v), want weight %v on %d, held in place", when, kind, d, ok, weight, target)
	}
}

// ind2bAttackMoves reports whether h's attack desire, queued or taken up,
// closes in on its target, and whether h has one at all.
func ind2bAttackMoves(h *npc.Hostile) (moves, ok bool) {
	if d, queued := desire(h, actorai.IntentionAttack); queued {
		return d.MoveToTarget, true
	}
	if h.AI().CurrentIntention() == actorai.IntentionAttack {
		return h.AI().CurrentIntentionMovesToTarget(), true
	}
	return false, false
}

// ind2bWantAttack fails unless h wants to attack, closing in when moves is
// set and held in place otherwise.
func ind2bWantAttack(t *testing.T, when string, h *npc.Hostile, moves bool) {
	t.Helper()
	got, ok := ind2bAttackMoves(h)
	if !ok || got != moves {
		t.Fatalf("%s: attack desire closes in = %v (%v), want %v", when, got, ok, moves)
	}
}

// ind2bHeldDebuff is the fixture debuff, reaching 2000 and holding an
// effect of its own stack for a minute.
func ind2bHeldDebuff() modelskill.Definition {
	d := ind1Skill(ind2DebuffSkill, 2000, false)
	d.Effects = []modelskill.EffectTemplate{{Name: "Buff", Time: 60, Icon: true, StackType: "ind2b_debuff", StackOrder: 1}}
	return d
}

// ind2bLandDebuff lands the fixture debuff's effect, cast by caster, on
// the character when on is nil and on the NPC on otherwise, and waits for
// it.
func ind2bLandDebuff(t *testing.T, w *ind1World, caster, on *npc.Hostile) {
	t.Helper()
	ref := modelskill.Ref{ID: ind2DebuffSkill, Level: 1}
	me := w.player.ObjectID()
	target := func(p *script.Player) script.Creature {
		if on == nil {
			return p
		}
		return script.NPCOf(on)
	}
	if !w.srv.RunScript(t, me, "Ind1Director", func(s *script.Script, p *script.Player) {
		s.ApplyEffects(ref, script.NPCOf(caster), target(p))
	}) {
		t.Fatal("the debuff landing panicked")
	}
	w.srv.AdvanceUntil(t, "the debuff landed", func() bool {
		var level int32
		w.srv.RunScript(t, me, "Ind1Director", func(s *script.Script, p *script.Player) {
			level = s.AbnormalLevel(target(p), ref)
		})
		return level > 0
	})
}

// TestAgit02DoomGuardAgitHoldsItsGround ports Agit02DoomGuardAgit's
// created, no-desire, see-creature and attacked hooks, every draw 0. It
// spawns with flag 0 and, with no desire left, does nothing. It wants to
// attack the character it sees from where it stands (weight 200). A hit
// by the character, the creature it last took up, wants its debuff on it
// from where it stands (weight 1000000) and adds hate by the damage: 100
// for each twentieth of its maximum hit points. The hit never reaches its
// parent: no fight timer is armed (i_ai1 stays 0).
func TestAgit02DoomGuardAgitHoldsItsGround(t *testing.T) {
	t.Parallel()
	w := bootIndividual2b(t, raidBoot{}, ind2bTemplate(ind2bGuardID))
	n := w.spawn(t, ind2bGuardID, 1500, 0)
	w.srv.InstallNPCCastSeam(t, n, ind2bSkills())
	me := w.player.ObjectID()
	if got := ind1Slot(n, script.SlotFlag); got != 0 {
		t.Fatalf("created flag = %d, want 0", got)
	}
	w.ticks(t, 3)
	if d, ok := desire(n, actorai.IntentionWander); ok || n.AI().CurrentIntention() == actorai.IntentionWander {
		t.Fatalf("the guard wanders: desire %+v (%v), intention %v", d, ok, n.AI().CurrentIntention())
	}

	w.moveCharacter(t, n, -150, 0)
	w.ticks(t, 1)
	if got := n.AggroHate(w.player); got != 200 {
		t.Fatalf("hate of the character it sees = %v, want 200", got)
	}
	if !script.Same(script.NPCOf(n).TopDesireTarget(), script.PlayerOf(w.player)) {
		t.Fatal("setup: the character is not the guard's top desire target")
	}

	w.onQueue(t, n, func() { n.AI().Desires().Clear() })
	w.hit(t, n, 10)
	ind2bWantHold(t, "debuff after the hit", n, actorai.IntentionCast, me, 1000000)
	if d := ind1Casts(n)[ind2DebuffSkill]; d.Weight != 1000000 {
		t.Fatalf("debuff desire after the hit = %+v, want weight 1000000", d)
	}
	// The hit also makes the guard's clan call to itself, which shares its
	// flag: the damage's hate twice.
	if want := 200 + 2*ind2bDamageHate(10, float64(n.MaxHP())); n.AggroHate(w.player) != want {
		t.Fatalf("hate after the hit = %v, want %v", n.AggroHate(w.player), want)
	}
	ind2bWantHold(t, "attack after the hit", n, actorai.IntentionAttack, me, 2*ind2bDamageHate(10, float64(n.MaxHP())))
	if got := ind1Slot(n, script.SlotIntAI1); got != 0 {
		t.Fatalf("i_ai1 after the hit = %d, want 0: the hit reached its parent", got)
	}
}

// TestAgit02DoomGuardAgitAnswersItsFlag ports Agit02DoomGuardAgit's
// clan-attacked hook, every draw 0, which never calls its parent. A
// settled member that shares the hit guard's flag wants its debuff on the
// attacker from where it stands (weight 1000000), and wants to attack it,
// held in place, by the damage. A member with another flag does neither.
func TestAgit02DoomGuardAgitAnswersItsFlag(t *testing.T) {
	t.Parallel()
	for _, row := range []struct {
		name string
		flag int32
	}{{"same flag", 0}, {"other flag", 1}} {
		t.Run(row.name, func(t *testing.T) {
			t.Parallel()
			w := bootIndividual2b(t, raidBoot{}, ind2bTemplate(ind2bGuardID), ind2bTemplate(ind2bGuardTwoID))
			victim := w.spawn(t, ind2bGuardID, 1500, 0)
			member := w.spawn(t, ind2bGuardTwoID, 1500, 200)
			w.srv.InstallNPCCastSeam(t, member, ind2bSkills())
			w.onQueue(t, member, func() { script.NPCOf(member).SetInt(script.SlotFlag, row.flag) })
			w.ticks(t, 8)
			w.hit(t, victim, 10)
			if row.flag != 0 {
				if hate, casts := member.AggroHate(w.player), ind1Casts(member); hate != 0 || len(casts) != 0 {
					t.Fatalf("other flag member: hate %v, casts %+v, want neither", hate, casts)
				}
				return
			}
			ind2bWantHold(t, "member debuff", member, actorai.IntentionCast, w.player.ObjectID(), 1000000)
			if want := ind2bDamageHate(10, float64(member.MaxHP())); member.AggroHate(w.player) != want {
				t.Fatalf("member hate = %v, want %v", member.AggroHate(w.player), want)
			}
		})
	}
}

// TestAgit02DoomGuardAgitHatesAnAggroSpell ports Agit02DoomGuardAgit's
// see-spell hook: the character's self buff of 50 aggro points makes the
// guard want to attack it, held in place, by 150 for each twentieth of its
// maximum hit points the aggro makes.
func TestAgit02DoomGuardAgitHatesAnAggroSpell(t *testing.T) {
	t.Parallel()
	tmpl := ind2bTemplate(ind2bGuardID)
	tmpl.CanMove = false
	w := bootIndividual2b(t, ind2bSpellBoot(t), tmpl)
	n := w.spawn(t, ind2bGuardID, 400, 0)
	want := float64(ind2bAggro) / float64(n.MaxHP()) / 0.05 * 150
	w.castSelfBuff()
	w.srv.AdvanceUntil(t, "the guard hates the caster", func() bool { return n.AggroHate(w.player) > 0 })
	if got := n.AggroHate(w.player); got != want {
		t.Fatalf("hate of the caster = %v, want %v", got, want)
	}
	ind2bWantAttack(t, "after the spell", n, false)
}

// TestAgit02DoomKnightAgitPartyJumpsToItsMaster ports
// Agit02DoomKnightAgitParty's created and timer hooks: its 60 s timer
// "3001", first fired 1 s after it spawns. Its master out of the master's
// territory, on a draw under 1 of 3 the private jumps to its master (the
// character sees TeleportToLocation) and drops its attack desires; on a
// draw of 1 it stays.
func TestAgit02DoomKnightAgitPartyJumpsToItsMaster(t *testing.T) {
	t.Parallel()
	for _, row := range []struct {
		draw int
		jump bool
	}{{0, true}, {1, false}} {
		t.Run(fmt.Sprintf("draw %d", row.draw), func(t *testing.T) {
			t.Parallel()
			w := bootIndividual2b(t, raidBoot{}, ind4Leader(ind2bPartyKnightID, 1), ind2bTemplate(ind2bPartyKnightID))
			w.draws.set(4)
			leader := w.spawn(t, leaderID, 1500, 0)
			n := privatesOf(t, leader, 1)[0]
			w.teleport(t, leader, -1350, 0)
			if script.NPCOf(n).InMyTerritory() {
				t.Fatal("setup: the private is still in its master's territory")
			}
			w.onQueue(t, n, func() { script.NPCOf(n).AddAttackDesire(script.PlayerOf(w.player), 100) })
			w.draws.set(4, row.draw, 4)
			w.srv.Advance(t, time.Second)
			frames := w.srv.ReadQueued(t, w.srv.Client)
			x, y, _ := n.Position()
			mx, my, _ := leader.Position()
			if atMaster := x == mx && y == my; atMaster != row.jump {
				t.Fatalf("private at %d,%d, master at %d,%d: at the master = %v, want %v", x, y, mx, my, atMaster, row.jump)
			}
			if hates := n.AggroHate(w.player) > 0; hates == row.jump {
				t.Fatalf("hate of the character = %v, want it dropped only for a jump", n.AggroHate(w.player))
			}
			if got := len(framesOf(frames, serverpackets.OpcodeTeleportToLocation, n.ObjectID())); (got > 0) != row.jump {
				t.Fatalf("TeleportToLocation frames = %d, want them only for a jump", got)
			}
		})
	}
}

// TestAgit02DoomKnightAgitPartyAnswersItsParty ports
// Agit02DoomKnightAgitParty's party-attacked hook, every draw 0, which
// never calls its parent. A hit on its master makes the settled private
// want its debuff on the attacker (weight 1000000) and, sharing its
// master's flag, want to attack the attacker by the damage; with another
// flag it only casts. The debuff's effect is looked up on the master, not
// on the attacker: the character holding it still draws the debuff, the
// master holding it does not.
func TestAgit02DoomKnightAgitPartyAnswersItsParty(t *testing.T) {
	t.Parallel()
	for _, row := range []struct {
		name string
		flag int32
		held string
		cast bool
		hate bool
	}{
		{"same flag", 0, "", true, true},
		{"other flag", 1, "", true, false},
		{"debuff on the character", 0, "character", true, true},
		{"debuff on the master", 0, "master", false, true},
	} {
		t.Run(row.name, func(t *testing.T) {
			t.Parallel()
			w := bootIndividual2b(t, raidBoot{opts: []gameservertest.Option{gameservertest.WithScriptSkills(ind2bHeldDebuff())}},
				ind4Leader(ind2bPartyKnightID, 1), ind2bTemplate(ind2bPartyKnightID))
			leader := w.spawn(t, leaderID, 400, 0)
			n := privatesOf(t, leader, 1)[0]
			w.srv.InstallNPCCastSeam(t, n, ind2bSkills())
			w.onQueue(t, n, func() { script.NPCOf(n).SetInt(script.SlotFlag, row.flag) })
			switch row.held {
			case "character":
				ind2bLandDebuff(t, w, n, nil)
			case "master":
				ind2bLandDebuff(t, w, n, leader)
			}
			w.ticks(t, 9)
			w.onQueue(t, n, func() { script.NPCOf(n).RemoveAllAttackDesire(); n.AI().Desires().Clear() })
			w.hit(t, leader, 10)
			want := map[modelskill.ID]ind1Cast{}
			if row.cast {
				want[ind2DebuffSkill] = ind1Cast{w.player.ObjectID(), 1000000}
			}
			w.wantCasts(t, "after the hit on its master", n, want)
			wantHate := 0.0
			if row.hate {
				wantHate = ind2bDamageHate(10, float64(n.MaxHP()))
			}
			if got := n.AggroHate(w.player); got != wantHate {
				t.Fatalf("private hate = %v, want %v", got, wantHate)
			}
		})
	}
}

// TestAgit02DoomKnightAgitPartyHatesAnAggroSpellNotItsClan ports
// Agit02DoomKnightAgitParty's see-spell and clan-attacked hooks. The
// character's self buff of 50 aggro points makes it want to attack the
// character, closing in, by 150 for each twentieth of its maximum hit
// points the aggro makes. A hit on a member of its clan, which would draw
// its parent's debuff, adds nothing.
func TestAgit02DoomKnightAgitPartyHatesAnAggroSpellNotItsClan(t *testing.T) {
	t.Parallel()
	t.Run("spell", func(t *testing.T) {
		t.Parallel()
		tmpl := ind2bTemplate(ind2bPartyKnightID)
		tmpl.CanMove = false
		w := bootIndividual2b(t, ind2bSpellBoot(t), tmpl)
		n := w.spawn(t, ind2bPartyKnightID, 400, 0)
		want := float64(ind2bAggro) / float64(n.MaxHP()) / 0.05 * 150
		w.castSelfBuff()
		w.srv.AdvanceUntil(t, "the knight hates the caster", func() bool { return n.AggroHate(w.player) > 0 })
		if got := n.AggroHate(w.player); got != want {
			t.Fatalf("hate of the caster = %v, want %v", got, want)
		}
		ind2bWantAttack(t, "after the spell", n, true)
	})
	t.Run("clan", func(t *testing.T) {
		t.Parallel()
		member := warriorTemplate(unboundID, nil)
		member.Clans, member.ClanRange = []string{"doom"}, 500
		w := bootIndividual2b(t, raidBoot{}, ind2bTemplate(ind2bPartyKnightID), member)
		n := w.spawn(t, ind2bPartyKnightID, 1500, 0)
		w.srv.InstallNPCCastSeam(t, n, ind2bSkills())
		clan := w.spawn(t, unboundID, 1500, 100)
		w.ticks(t, 9)
		w.hit(t, clan, 10)
		if hate, casts := n.AggroHate(w.player), ind1Casts(n); hate != 0 || len(casts) != 0 {
			t.Fatalf("after a hit on its clan: hate %v, casts %+v, want neither", hate, casts)
		}
	})
}

// TestAgit02VampireAgitPartyAnswersItsClanNotItsParty ports
// Agit02VampireAgitParty's party-attacked and clan-attacked hooks, every
// draw 0. A hit on its master adds nothing. A hit on a member of its clan
// makes the settled vampire want its debuff on the attacker (weight
// 1000000) and, sharing the member's flag, want to attack the attacker by
// the damage. The debuff's effect is looked up on the hit member, not on
// the attacker: the character holding it still draws the debuff, the
// member holding it does not.
func TestAgit02VampireAgitPartyAnswersItsClanNotItsParty(t *testing.T) {
	t.Parallel()
	for _, row := range []struct {
		name string
		held string
		cast bool
	}{{"no debuff held", "", true}, {"debuff on the character", "character", true}, {"debuff on the member", "member", false}} {
		t.Run(row.name, func(t *testing.T) {
			t.Parallel()
			member := warriorTemplate(unboundID, nil)
			member.Clans, member.ClanRange = []string{"doom"}, 500
			w := bootIndividual2b(t, raidBoot{opts: []gameservertest.Option{gameservertest.WithScriptSkills(ind2bHeldDebuff())}},
				ind4Leader(ind2bVampireID, 1), ind2bTemplate(ind2bVampireID), member)
			leader := w.spawn(t, leaderID, 400, 0)
			n := privatesOf(t, leader, 1)[0]
			w.srv.InstallNPCCastSeam(t, n, ind2bSkills())
			clan := w.spawn(t, unboundID, 400, 100)
			switch row.held {
			case "character":
				ind2bLandDebuff(t, w, n, nil)
			case "member":
				ind2bLandDebuff(t, w, n, clan)
			}
			w.ticks(t, 9)
			w.onQueue(t, n, func() { script.NPCOf(n).RemoveAllAttackDesire(); n.AI().Desires().Clear() })

			w.hit(t, leader, 10)
			if hate, casts := n.AggroHate(w.player), ind1Casts(n); hate != 0 || len(casts) != 0 {
				t.Fatalf("after a hit on its master: hate %v, casts %+v, want neither", hate, casts)
			}

			w.hit(t, clan, 10)
			want := map[modelskill.ID]ind1Cast{}
			if row.cast {
				want[ind2DebuffSkill] = ind1Cast{w.player.ObjectID(), 1000000}
			}
			w.wantCasts(t, "after a hit on its clan", n, want)
			if want := ind2bDamageHate(10, float64(n.MaxHP())); n.AggroHate(w.player) != want {
				t.Fatalf("hate after a hit on its clan = %v, want %v", n.AggroHate(w.player), want)
			}
		})
	}
}

// TestWarriorHereticSelfRangePhysicalFollowsScriptEvents ports
// WarriorHereticSelfRangePhysical's created, attacked and script-event
// hooks, every draw 0. It spawns with i_ai4 0, and a hit while it neither
// attacks nor casts sets i_ai4. A hit by the creature it last took up
// wants its self range physical skill on it (weight 1000000), beside its
// parent's debuff. Event 10035 clears i_ai4, drops its attack desires and
// wants to walk to its spawn point (weight 100). Event 10002 drops its
// attack desires and wants to attack the named creature (weight 200);
// 10033 does the same and sets i_ai4. An event naming no creature does
// nothing.
func TestWarriorHereticSelfRangePhysicalFollowsScriptEvents(t *testing.T) {
	t.Parallel()
	w := bootIndividual2b(t, raidBoot{}, ind2bTemplate(ind2bHereticID))
	n := w.spawn(t, ind2bHereticID, 300, 0)
	w.srv.InstallNPCCastSeam(t, n, ind2bSkills())
	me := w.player.ObjectID()
	if got := ind1Slot(n, script.SlotIntAI4); got != 0 {
		t.Fatalf("created i_ai4 = %d, want 0", got)
	}
	w.hit(t, n, 10)
	if got := ind1Slot(n, script.SlotIntAI4); got != 1 {
		t.Fatalf("i_ai4 after a hit while idle = %d, want 1", got)
	}
	w.wantCasts(t, "after the first hit", n, nil)
	w.ticks(t, 1)
	w.onQueue(t, n, func() { n.AI().Desires().RemoveKind(actorai.IntentionCast) })
	w.hit(t, n, 10)
	w.wantCasts(t, "after a hit by its top desire target", n, map[modelskill.ID]ind1Cast{
		ind2bSelfRangeSkil: {me, 1000000},
		ind2DebuffSkill:    {me, 1000000},
	})

	w.sendEvent(t, n, 10035, 0, 0)
	if got := ind1Slot(n, script.SlotIntAI4); got != 0 {
		t.Fatalf("i_ai4 after event 10035 = %d, want 0", got)
	}
	if d, ok := desire(n, actorai.IntentionAttack); ok {
		t.Fatalf("attack desire after event 10035 = %+v, want none", d)
	}
	home := script.NPCOf(n).SpawnLocation()
	if d, ok := desire(n, actorai.IntentionMoveTo); !ok || d.Weight != 100 || d.Location.X != home.X || d.Location.Y != home.Y {
		t.Fatalf("move desire after event 10035 = %+v (%v), want weight 100 to its spawn point %d,%d", d, ok, home.X, home.Y)
	}

	w.sendEvent(t, n, 10033, 0, 0)
	if got := ind1Slot(n, script.SlotIntAI4); got != 0 {
		t.Fatalf("i_ai4 after event 10033 naming nobody = %d, want 0", got)
	}
	w.sendEvent(t, n, 10002, me, 0)
	wantDesire(t, "after event 10002", n, actorai.IntentionAttack, me, 200)
	if got := ind1Slot(n, script.SlotIntAI4); got != 0 {
		t.Fatalf("i_ai4 after event 10002 = %d, want 0", got)
	}
	w.sendEvent(t, n, 10033, me, 0)
	// The earlier desire is dropped, not added to.
	wantDesire(t, "after event 10033", n, actorai.IntentionAttack, me, 200)
	if got := ind1Slot(n, script.SlotIntAI4); got != 1 {
		t.Fatalf("i_ai4 after event 10033 = %d, want 1", got)
	}
}

// TestWarriorHereticSelfRangePhysicalAnswersItsClan ports
// WarriorHereticSelfRangePhysical's clan-attacked hook, every draw 0. A
// hit on a member of its clan while the heretic neither attacks nor casts
// sets i_ai4. Settled, with the character its top desire target, a hit on
// the member wants its self range physical skill on the character (weight
// 1000000), beside its parent's debuff.
func TestWarriorHereticSelfRangePhysicalAnswersItsClan(t *testing.T) {
	t.Parallel()
	member := warriorTemplate(unboundID, nil)
	member.Clans, member.ClanRange = []string{"doom"}, 500
	w := bootIndividual2b(t, raidBoot{}, ind2bTemplate(ind2bHereticID), member)
	n := w.spawn(t, ind2bHereticID, 1500, 0)
	w.srv.InstallNPCCastSeam(t, n, ind2bSkills())
	clan := w.spawn(t, unboundID, 1500, 100)
	me := w.player.ObjectID()
	if fightingIntention(n) {
		t.Fatalf("setup: intention %v, want neither attack nor cast", n.AI().CurrentIntention())
	}
	w.hit(t, clan, 10)
	if got := ind1Slot(n, script.SlotIntAI4); got != 1 {
		t.Fatalf("i_ai4 after a hit on its clan while idle = %d, want 1", got)
	}
	w.wantCasts(t, "after a hit on its clan while young", n, nil)

	w.ticks(t, 8)
	w.setTop(t, n, script.PlayerOf(w.player))
	w.hit(t, clan, 10)
	w.wantCasts(t, "after a hit on its clan by its top desire target", n, map[modelskill.ID]ind1Cast{
		ind2bSelfRangeSkil: {me, 1000000},
		ind2DebuffSkill:    {me, 1000000},
	})
}

// ind2bHereticDen is an npcmaker spawning one heretic in a 400 by 400
// territory.
const ind2bHereticDen = `<?xml version="1.0" encoding="utf-8"?>
<list>
	<territory name="heretic_den" minZ="-1000" maxZ="1000"><node x="0" y="0"/><node x="400" y="0"/><node x="400" y="400"/><node x="0" y="400"/></territory>
	<npcmaker name="heretic_den" territory="heretic_den" maximumNpcs="1">
		<ai type="default_maker"/>
		<npc id="22147" total="1" pos="40;20;30;0" respawn="60sec"/>
	</npcmaker>
</list>`

// TestWarriorHereticSelfRangePhysicalWalksHomeFromOutside ports
// WarriorHereticSelfRangePhysical's out-of-territory hook on a heretic its
// npcmaker spawned. Arriving outside its territory with i_ai4 clear, it
// wants to walk to its spawn point with weight 100. Released from the
// territory (i_ai4 set), its arrival outside wants no walk home.
func TestWarriorHereticSelfRangePhysicalWalksHomeFromOutside(t *testing.T) {
	t.Parallel()
	for _, row := range []struct {
		name     string
		released bool
	}{{"bound", false}, {"released", true}} {
		t.Run(row.name, func(t *testing.T) {
			t.Parallel()
			dir := t.TempDir()
			if err := os.WriteFile(filepath.Join(dir, "den.xml"), []byte(ind2bHereticDen), 0o644); err != nil {
				t.Fatalf("write spawnlist: %v", err)
			}
			table, err := gamexml.LoadSpawnlist(dir, zerolog.Nop(), 1)
			if err != nil {
				t.Fatalf("load spawnlist: %v", err)
			}
			w := bootIndividual2b(t, raidBoot{opts: []gameservertest.Option{gameservertest.WithNpcSpawns(table)}}, ind2bTemplate(ind2bHereticID))
			var heretic *npc.Hostile
			for _, obj := range w.srv.State.Objects() {
				if h, ok := obj.(*npc.Hostile); ok && h.NpcID() == int(ind2bHereticID) {
					heretic = h
				}
			}
			if heretic == nil {
				t.Fatal("the den spawned no heretic")
			}
			if row.released {
				w.onQueue(t, heretic, func() { script.NPCOf(heretic).SetInt(script.SlotIntAI4, 1) })
			}
			home := script.NPCOf(heretic).SpawnLocation()

			ind4WalkTo(t, w, heretic, location.Location{X: 40, Y: 700, Z: 30})
			if heretic.InTerritory() {
				t.Fatal("setup: the walk ended inside the territory")
			}
			d, ok := desire(heretic, actorai.IntentionMoveTo)
			if row.released {
				if ok {
					t.Fatalf("released, move desire after arriving outside = %+v, want none", d)
				}
				return
			}
			if !ok || d.Weight != 100 || d.Location.X != home.X || d.Location.Y != home.Y {
				t.Fatalf("move desire after arriving outside = %+v (%v), want weight 100 to its spawn point %d,%d", d, ok, home.X, home.Y)
			}
		})
	}
}

// TestWarriorHereticSelfRangePhysicalAggressiveAttacksOnSight ports
// WarriorHereticSelfRangePhysicalAggressive's see-creature hook: it wants
// to attack the character it sees (hate 200), where a
// WarriorHereticSelfRangePhysical beside it does not.
func TestWarriorHereticSelfRangePhysicalAggressiveAttacksOnSight(t *testing.T) {
	t.Parallel()
	w := bootIndividual2b(t, raidBoot{}, ind2bTemplate(ind2bHereticAggrID), ind2bTemplate(ind2bHereticID))
	aggressive := w.spawn(t, ind2bHereticAggrID, 150, 0)
	passive := w.spawn(t, ind2bHereticID, 0, 150)
	w.ticks(t, 2)
	if got := aggressive.AggroHate(w.player); got != 200 {
		t.Fatalf("aggressive hate of the character it sees = %v, want 200", got)
	}
	if got := passive.AggroHate(w.player); got != 0 {
		t.Fatalf("passive hate of the character it sees = %v, want none", got)
	}
}

// TestWarriorCastingCurseAggressiveCursesOnSight ports
// WarriorCastingCurseAggressive's see-creature hook, every draw 0.
// Settled, wandering in its territory, it wants its debuff on the
// character it sees (weight 1000000), then attacks it (hate 200); young,
// it only attacks.
func TestWarriorCastingCurseAggressiveCursesOnSight(t *testing.T) {
	t.Parallel()
	for _, row := range []struct {
		name   string
		settle int
		cast   bool
	}{{"settled", 9, true}, {"young", 0, false}} {
		t.Run(row.name, func(t *testing.T) {
			t.Parallel()
			w := bootIndividual2b(t, raidBoot{}, ind2bTemplate(ind2bCurseAggroID))
			n := w.spawn(t, ind2bCurseAggroID, 1500, 0)
			w.srv.InstallNPCCastSeam(t, n, ind2bSkills())
			w.ticks(t, row.settle)
			if row.cast && (n.LifeTime() <= 7 || n.AI().CurrentIntention() != actorai.IntentionWander) {
				t.Fatalf("setup: life time %d, intention %v, want past 7 and wander", n.LifeTime(), n.AI().CurrentIntention())
			}
			w.moveCharacter(t, n, -50, 0)
			w.ticks(t, 1)
			want := map[modelskill.ID]ind1Cast{}
			if row.cast {
				want[ind2DebuffSkill] = ind1Cast{w.player.ObjectID(), 1000000}
			}
			w.wantCasts(t, "after it saw the character", n, want)
			if got := n.AggroHate(w.player); got != 200 {
				t.Fatalf("hate of the character it sees = %v, want 200", got)
			}
		})
	}
}

// TestAgit02DoomKnightAgitReturnsHome ports Agit02DoomKnightAgit's
// created, no-desire and timer hooks. It spawns with flag 0 and does
// nothing with no desire left. Its 60 s timer "3001", first fired 1 s after
// it spawns: out of its territory, on a draw under 1 of 3, it jumps back to
// its spawn point (TeleportToLocation) and drops its attack desires; on a
// draw of 1 it stays.
func TestAgit02DoomKnightAgitReturnsHome(t *testing.T) {
	t.Parallel()
	for _, row := range []struct {
		draw int
		home bool
	}{{0, true}, {1, false}} {
		t.Run(fmt.Sprintf("draw %d", row.draw), func(t *testing.T) {
			t.Parallel()
			w := bootIndividual2b(t, raidBoot{}, ind2bTemplate(ind2bKnightID))
			w.draws.set(4)
			n := w.spawn(t, ind2bKnightID, 1500, 0)
			if got := ind1Slot(n, script.SlotFlag); got != 0 {
				t.Fatalf("created flag = %d, want 0", got)
			}
			if d, ok := desire(n, actorai.IntentionWander); ok {
				t.Fatalf("the knight queued a wander desire %+v", d)
			}
			home := script.NPCOf(n).SpawnLocation()
			w.teleport(t, n, -1350, 0)
			if script.NPCOf(n).InMyTerritory() {
				t.Fatal("setup: the knight is still in its territory")
			}
			w.onQueue(t, n, func() { script.NPCOf(n).AddAttackDesire(script.PlayerOf(w.player), 100) })
			w.draws.set(4, row.draw, 4)
			w.srv.Advance(t, time.Second)
			frames := w.srv.ReadQueued(t, w.srv.Client)
			x, y, _ := n.Position()
			if atHome := x == home.X && y == home.Y; atHome != row.home {
				t.Fatalf("knight at %d,%d, spawn point %d,%d: at home = %v, want %v", x, y, home.X, home.Y, atHome, row.home)
			}
			if _, attacking := desire(n, actorai.IntentionAttack); attacking == row.home {
				t.Fatalf("attack desire queued = %v, want %v", attacking, !row.home)
			}
			if got := len(framesOf(frames, serverpackets.OpcodeTeleportToLocation, n.ObjectID())); (got > 0) != row.home {
				t.Fatalf("TeleportToLocation frames = %d, want them only for a jump home", got)
			}
		})
	}
}

// TestAgit02DoomKnightAgitAnswersItsFlag ports Agit02DoomKnightAgit's
// clan-attacked hook, every draw 0, which never calls its parent. A
// settled member that shares the hit knight's flag wants its debuff on
// the attacker from where it stands (weight 1000000) and wants to attack
// it by the damage, closing in. A member with another flag does neither.
func TestAgit02DoomKnightAgitAnswersItsFlag(t *testing.T) {
	t.Parallel()
	for _, row := range []struct {
		name string
		flag int32
	}{{"same flag", 0}, {"other flag", 1}} {
		t.Run(row.name, func(t *testing.T) {
			t.Parallel()
			w := bootIndividual2b(t, raidBoot{}, ind2bTemplate(ind2bKnightID), ind2bTemplate(ind2bKnightTwoID))
			victim := w.spawn(t, ind2bKnightID, 1500, 0)
			member := w.spawn(t, ind2bKnightTwoID, 1500, 200)
			w.srv.InstallNPCCastSeam(t, member, ind2bSkills())
			w.onQueue(t, member, func() { script.NPCOf(member).SetInt(script.SlotFlag, row.flag) })
			w.ticks(t, 8)
			w.hit(t, victim, 10)
			if row.flag != 0 {
				if hate, casts := member.AggroHate(w.player), ind1Casts(member); hate != 0 || len(casts) != 0 {
					t.Fatalf("other flag member: hate %v, casts %+v, want neither", hate, casts)
				}
				return
			}
			ind2bWantHold(t, "member debuff", member, actorai.IntentionCast, w.player.ObjectID(), 1000000)
			if want := ind2bDamageHate(10, float64(member.MaxHP())); member.AggroHate(w.player) != want {
				t.Fatalf("member hate = %v, want %v", member.AggroHate(w.player), want)
			}
		})
	}
}

// TestCorePrivateDicorCursesOverLevelled ports CorePrivateDicor's attacked
// and see-spell hooks. Against the level 20 character, a level 11 Dicor
// wants its raid curse on a hit and its raid mute on the character's
// spell, weight 1000000; a level 12 one wants neither. A hit adds no hate.
func TestCorePrivateDicorCursesOverLevelled(t *testing.T) {
	t.Parallel()
	for _, row := range []struct {
		level int
		curse bool
	}{{11, true}, {12, false}} {
		t.Run(fmt.Sprintf("level %d", row.level), func(t *testing.T) {
			t.Parallel()
			tmpl := ind2bTemplate(ind2bDicorID)
			tmpl.Level, tmpl.CanMove = row.level, false
			w := bootIndividual2b(t, raidSpellBoot(t), tmpl)
			n := w.spawn(t, ind2bDicorID, 400, 0)
			w.srv.InstallNPCCastSeam(t, n, modelskill.NewTable([]modelskill.Definition{raidCurseDefinition(), ind1Skill(premoMuteID, 600, false)}))
			me := w.player.ObjectID()

			w.hit(t, n, 10)
			want := map[modelskill.ID]ind1Cast{}
			if row.curse {
				want[raidCurseID] = ind1Cast{me, 1000000}
			}
			w.wantCasts(t, "after the hit", n, want)
			if got := n.AggroHate(w.player); got != 0 {
				t.Errorf("hate after the hit = %v, want none", got)
			}

			w.castSelfBuff()
			if row.curse {
				w.srv.AdvanceUntil(t, "the raid mute desire", func() bool {
					_, ok := ind1Casts(n)[premoMuteID]
					return ok
				})
				want[premoMuteID] = ind1Cast{me, 1000000}
			} else {
				w.srv.Advance(t, 2*time.Second)
			}
			w.wantCasts(t, "after the spell", n, want)
		})
	}
}

// TestCorePrivateDicorShufflesItsHate ports CorePrivateDicor's created and
// timer hooks. It arms timer "3001" 90 s plus a draw of up to 240 s after
// it spawns (draw 0: 90 s). On a draw under 66 the tick shuffles its hate,
// raising the character it hates less than another NPC to that NPC's hate
// plus 200; on a draw of 66 it does not. Either way the tick re-arms with
// a fresh draw: 1000 brings the next tick 91 s later.
func TestCorePrivateDicorShufflesItsHate(t *testing.T) {
	t.Parallel()
	for _, row := range []struct {
		draw    int
		shuffle bool
	}{{65, true}, {66, false}} {
		t.Run(fmt.Sprintf("draw %d", row.draw), func(t *testing.T) {
			t.Parallel()
			tmpl := ind2bTemplate(ind2bDicorID)
			tmpl.CanMove = false
			w := bootIndividual2b(t, raidBoot{}, tmpl, warriorTemplate(unboundID, nil))
			n := w.spawn(t, ind2bDicorID, 400, 0)
			other := w.spawn(t, unboundID, 400, 200)
			ind2Hate(t, w, n, other, 1000, 100)

			w.srv.Advance(t, 89*time.Second)
			if got := n.AggroHate(w.player); got != 100 {
				t.Fatalf("hate of the character before 90 s = %v, want 100", got)
			}
			w.draws.set(0, row.draw, 1000)
			w.srv.Advance(t, time.Second)
			want := 100.0
			if row.shuffle {
				want = 1200
			}
			if got := n.AggroHate(w.player); got != want {
				t.Fatalf("hate of the character after the first tick = %v, want %v", got, want)
			}
			if row.shuffle {
				return
			}
			w.srv.Advance(t, 90*time.Second)
			if got := n.AggroHate(w.player); got != 100 {
				t.Fatalf("hate of the character 90 s after the first tick = %v, want 100", got)
			}
			w.srv.Advance(t, time.Second)
			if got := n.AggroHate(w.player); got != 1200 {
				t.Fatalf("hate of the character after the re-armed tick = %v, want 1200", got)
			}
		})
	}
}
