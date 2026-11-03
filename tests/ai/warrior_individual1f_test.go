package ai

import (
	"fmt"
	"testing"
	"time"

	"github.com/fatal10110/acis_golang/internal/commons/wire"
	actorai "github.com/fatal10110/acis_golang/internal/gameserver/model/actor/ai"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/npc"
	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/clientpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/script"
	"github.com/fatal10110/acis_golang/internal/gameserver/script/ai/warrior"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
)

// The NPC ids the sixth PR of the first individual behavior batch (#186
// PR 6) spawns, each a fixture monster of level 1 unless a test says
// otherwise.
const (
	magical2ID      = int32(20411) // WarriorCast3SkillsMagical2
	magical2AggID   = int32(18001) // WarriorCast3SkillsMagical2Aggressive
	magical2RevID   = int32(20860) // WarriorCast3SkillsMagical2Revived
	magical2StoneID = int32(21331) // WarriorCast3SkillsMagical2StoneAggressive
	magical3ID      = int32(20611) // WarriorCast3SkillsMagical3
	magical3AggID   = int32(20821) // WarriorCast3SkillsMagical3Aggressive
	magical3RevID   = int32(21063) // WarriorCast3SkillsMagical3Revived
	magical4ID      = int32(21197) // WarriorCast3SkillsMagical4
)

// The batch's fixture skills, listed by type, beside those of the fifth
// PR's fixture template.
const (
	ind1fDD1ID        = modelskill.ID(9641) // DD_MAGIC1
	ind1fDD2ID        = modelskill.ID(9642) // DD_MAGIC2
	ind1fHoldID       = modelskill.ID(9643) // HOLD_MAGIC
	ind1fHealID       = modelskill.ID(9644) // MAGIC_HEAL
	ind1fBuffID       = modelskill.ID(9645) // BUFF
	ind1fDebuffID     = modelskill.ID(9646) // DEBUFF
	ind1fDebuffMarkID = modelskill.ID(9647) // lands the debuff's stack
)

// ind1fBatch are the sixth PR's classes, in scripts.xml order.
var ind1fBatch = []struct {
	path string
	ctor func() script.Script
}{
	{"script.ai.individual.Monster.WarriorBase.Warrior.WarriorCast3SkillsMagical2.WarriorCast3SkillsMagical2", warrior.Cast3SkillsMagical2},
	{"script.ai.individual.Monster.WarriorBase.Warrior.WarriorCast3SkillsMagical2.WarriorCast3SkillsMagical2Aggressive", warrior.Cast3SkillsMagical2Aggressive},
	{"script.ai.individual.Monster.WarriorBase.Warrior.WarriorCast3SkillsMagical2.WarriorCast3SkillsMagical2Revived", warrior.Cast3SkillsMagical2Revived},
	{"script.ai.individual.Monster.WarriorBase.Warrior.WarriorCast3SkillsMagical2.WarriorCast3SkillsMagical2StoneAggressive", warrior.Cast3SkillsMagical2StoneAggressive},
	{"script.ai.individual.Monster.WarriorBase.Warrior.WarriorCast3SkillsMagical3.WarriorCast3SkillsMagical3", warrior.Cast3SkillsMagical3},
	{"script.ai.individual.Monster.WarriorBase.Warrior.WarriorCast3SkillsMagical3.WarriorCast3SkillsMagical3Aggressive", warrior.Cast3SkillsMagical3Aggressive},
	{"script.ai.individual.Monster.WarriorBase.Warrior.WarriorCast3SkillsMagical3.WarriorCast3SkillsMagical3Revived", warrior.Cast3SkillsMagical3Revived},
	{"script.ai.individual.Monster.WarriorBase.Warrior.WarriorCast3SkillsMagical4.WarriorCast3SkillsMagical4", warrior.Cast3SkillsMagical4},
}

// bootIndividual1f boots the sixth PR's classes after the Warrior chain
// beside templates and an unbound monster of the batch's clan, with extra
// and a director script to land effects and call privates through, every
// NPC casting the batch's skills, every draw 0 until a scenario queues
// others.
func bootIndividual1f(t *testing.T, extra raidBoot, templates ...*npc.Template) *ind1World {
	t.Helper()
	list, catalog := warriorScripts()
	for _, c := range ind1fBatch {
		list = append(list, script.Listing{Path: c.path})
		catalog[c.path] = c.ctor
	}
	const director = "test.Ind1Director"
	catalog[director] = func() script.Script { return script.Script{} }
	list = append(list, script.Listing{Path: director})
	templates = append(templates, ind1fTemplate(unboundID))
	kinds := map[int32]script.NPCKind{}
	for _, tmpl := range templates {
		kinds[int32(tmpl.ID)] = script.KindHostile
	}
	draws := &ind1Draws{}
	opts := append([]gameservertest.Option{
		gameservertest.WithScriptRand(draws.rand),
		gameservertest.WithNPCScripts(kinds, list, catalog),
		gameservertest.WithNpcSpawnSkills(modelskill.NewTable(ind1fDefs())),
		gameservertest.WithScriptSkills(ind1fDefs()...),
	}, extra.opts...)
	w := bootRaid(t, raidBoot{opts: opts, ready: extra.ready}, templates...)
	return &ind1World{warriorWorld: w, draws: draws, heard: &ind1Heard{}}
}

// bootIndividual1fArcher boots the batch beside templates with the
// character holding the fixture bow.
func bootIndividual1fArcher(t *testing.T, templates ...*npc.Template) *ind1World {
	t.Helper()
	var bow int32
	w := bootIndividual1f(t, raidBoot{ready: func(srv *gameservertest.Server, objID int32) { bow = srv.GiveItem(t, objID, ind5eBowID, 1) }}, templates...)
	use := wire.NewPacketWriter(clientpackets.OpcodeUseItem)
	use.WriteInt32(bow)
	use.WriteInt32(0)
	w.srv.Client.Send(use.Bytes())
	w.srv.AdvanceUntil(t, "the bow in the hands", func() bool {
		var hand int32
		w.srv.RunScript(t, w.player.ObjectID(), "Ind1Director", func(_ *script.Script, p *script.Player) { hand = p.RightHandItemID() })
		return hand == ind5eBowID
	})
	w.srv.ReadQueued(t, w.srv.Client)
	return w
}

// ind1fTemplate is a fixture monster of id standing still, listing the
// fifth PR's skills and this batch's by type, in the clan the scenario's
// NPCs share.
func ind1fTemplate(id int32) *npc.Template {
	tmpl := ind1eTemplate(id, nil)
	for typ, skillID := range map[npc.SkillType]modelskill.ID{
		npc.SkillTypeDDMagic1: ind1fDD1ID, npc.SkillTypeDDMagic2: ind1fDD2ID, npc.SkillTypeHoldMagic: ind1fHoldID,
		npc.SkillTypeMagicHeal: ind1fHealID, npc.SkillTypeBuff: ind1fBuffID, npc.SkillTypeDebuff: ind1fDebuffID,
	} {
		tmpl.SkillsByType[typ] = modelskill.Ref{ID: skillID, Level: 1}
	}
	return tmpl
}

// ind1fDefs are the fifth PR's fixture skills and this batch's: the two DD
// magics and the hold, one-target attacks of reach 900; the heal, a
// one-target friendly skill; the buff, a self skill; the debuff, a
// one-target attack whose effect stacks as ind1f_debuff; and the mark,
// a self buff landing that stack for a minute.
func ind1fDefs() []modelskill.Definition {
	heal := ind1Skill(ind1fHealID, 900, false)
	heal.Offensive, heal.Magic, heal.SkillType, heal.Power = false, true, "HEAL", 0
	debuff := ind1Skill(ind1fDebuffID, 900, false)
	debuff.Effects = []modelskill.EffectTemplate{{Name: "Debuff", Time: 60, Icon: true, StackType: "ind1f_debuff", StackOrder: 1}}
	mark := ind1Skill(ind1fDebuffMarkID, 0, true)
	mark.Effects = []modelskill.EffectTemplate{{Name: "Buff", Time: 60, Icon: true, StackType: "ind1f_debuff", StackOrder: 1}}
	return append(ind1eDefs(),
		ind1Skill(ind1fDD1ID, 900, false), ind1Skill(ind1fDD2ID, 900, false), ind1Skill(ind1fHoldID, 900, false),
		heal, ind1Skill(ind1fBuffID, 0, true), debuff, mark,
	)
}

// ind1fMark lands the debuff's stack on c.
func (w *ind1World) ind1fMark(t *testing.T, c script.Creature) {
	t.Helper()
	w.ind5dLand(t, modelskill.Ref{ID: ind1fDebuffMarkID, Level: 1}, c)
}

// ind1fSettledAtWander spawns id 1500 away and lets it settle until it
// wanders.
func (w *ind1World) ind1fSettledAtWander(t *testing.T, id int32) *npc.Hostile {
	t.Helper()
	n := w.spawn(t, id, 1500, 0)
	w.ticks(t, 9)
	if got := n.AI().CurrentIntention(); got != actorai.IntentionWander {
		t.Fatalf("setup: intention = %v, want wander", got)
	}
	return n
}

// TestCast3SkillsMagical2BlastsAndDebuffsItsMostHated ports
// WarriorCast3SkillsMagical2's attacked hook, every draw 0. Its first hit
// finds it hating no one, so only its parent answers. Hit again from 300 by
// the character it hates most, it wants both DD magics and its debuff on
// the character, weight 1000000 each; the debuff not when the character
// already holds its stack. From 50 away, or hating another NPC most, it
// wants nothing.
func TestCast3SkillsMagical2BlastsAndDebuffsItsMostHated(t *testing.T) {
	t.Parallel()
	for _, row := range []struct {
		name          string
		dist          int
		marked, decoy bool
		want          []modelskill.ID
	}{
		{"far", 300, false, false, []modelskill.ID{ind1fDD1ID, ind1fDD2ID, ind1fDebuffID}},
		{"far and marked", 300, true, false, []modelskill.ID{ind1fDD1ID, ind1fDD2ID}},
		{"far hating another", 300, false, true, nil},
		{"close", 50, false, false, nil},
	} {
		t.Run(row.name, func(t *testing.T) {
			t.Parallel()
			w := bootIndividual1f(t, raidBoot{}, ind1fTemplate(magical2ID))
			n := w.spawn(t, magical2ID, row.dist, 0)
			me := w.player.ObjectID()
			if row.marked {
				w.ind1fMark(t, script.PlayerOf(w.player))
			}
			if row.decoy {
				ind2Hate(t, w, n, w.spawn(t, unboundID, row.dist, 200), 100000, 100)
			} else {
				w.hit(t, n, 10)
				w.wantCasts(t, "after the first hit", n, nil)
			}
			w.hit(t, n, 10)
			want := map[modelskill.ID]ind1Cast{}
			for _, id := range row.want {
				want[id] = ind1Cast{me, 1000000}
			}
			w.wantCasts(t, "after the second hit", n, want)
		})
	}
}

// TestCast3SkillsMagical2AnswersItsClan ports WarriorCast3SkillsMagical2's
// clan-attacked hook, every draw 0. A settled member that is not attacking
// wants both DD magics and its debuff on the character 300 away that hits
// its clan, weight 1000000 each, and its parent's hook adds
// int(10 / 8 * 100) = 125 hate; a member 50 away wants only the debuff; a
// fresh member wants nothing and, its parent's hook waiting too, takes no
// hate.
func TestCast3SkillsMagical2AnswersItsClan(t *testing.T) {
	t.Parallel()
	for _, row := range []struct {
		name    string
		dist    int
		settled bool
		want    []modelskill.ID
	}{
		{"300 away", 300, true, []modelskill.ID{ind1fDD1ID, ind1fDD2ID, ind1fDebuffID}},
		{"50 away", 50, true, []modelskill.ID{ind1fDebuffID}},
		{"fresh", 300, false, nil},
	} {
		t.Run(row.name, func(t *testing.T) {
			t.Parallel()
			w := bootIndividual1f(t, raidBoot{}, ind1fTemplate(magical2ID))
			member := w.spawn(t, magical2ID, row.dist, 0)
			v := w.spawn(t, unboundID, 0, 100)
			if row.settled {
				w.ticks(t, 8)
			}
			w.hit(t, v, 10)
			want := map[modelskill.ID]ind1Cast{}
			for _, id := range row.want {
				want[id] = ind1Cast{w.player.ObjectID(), 1000000}
			}
			w.wantCasts(t, "after the hit on its clan", member, want)
			wantHate := 0.0
			if row.settled {
				wantHate = 125
			}
			if got := member.AggroHate(w.player); got != wantHate {
				t.Fatalf("member hate after the hit on its clan = %v, want %v", got, wantHate)
			}
		})
	}
}

// TestCast3SkillsMagical2AggressiveCastsOnSight ports
// WarriorCast3SkillsMagical2Aggressive's see-creature hook, every draw 0.
// Settled and wandering in its territory, it wants both DD magics and its
// debuff on the character it sees 300 away, weight 1000000 each, and turns
// on it with weight 200; fresh it only turns on it.
func TestCast3SkillsMagical2AggressiveCastsOnSight(t *testing.T) {
	t.Parallel()
	for _, settled := range []bool{true, false} {
		t.Run(fmt.Sprintf("settled %v", settled), func(t *testing.T) {
			t.Parallel()
			w := bootIndividual1f(t, raidBoot{}, ind1fTemplate(magical2AggID))
			var n *npc.Hostile
			want := map[modelskill.ID]ind1Cast{}
			if settled {
				n = w.ind1fSettledAtWander(t, magical2AggID)
				for _, id := range []modelskill.ID{ind1fDD1ID, ind1fDD2ID, ind1fDebuffID} {
					want[id] = ind1Cast{w.player.ObjectID(), 1000000}
				}
			} else {
				n = w.spawn(t, magical2AggID, 1500, 0)
			}
			w.moveCharacter(t, n, 300, 0)
			w.ticks(t, 1)
			w.wantCasts(t, "on sight", n, want)
			if got := n.AggroHate(w.player); got != 200 {
				t.Fatalf("hate of the character it sees = %v, want 200", got)
			}
		})
	}
}

// TestCast3SkillsMagicalRevivedChildrenStunTheNamedCreature ports the
// created hooks of WarriorCast3SkillsMagical2Revived and
// WarriorCast3SkillsMagical3Revived. Called with spawn parameters 1000 and
// the character, each wants its stun on the character, weight 10000 from
// it and 10000 from its WarriorBase parent, and to attack the character,
// 500 from each; called with none it wants neither.
func TestCast3SkillsMagicalRevivedChildrenStunTheNamedCreature(t *testing.T) {
	t.Parallel()
	for _, row := range []struct {
		name            string
		revived, master int32
	}{{"magical2", magical2RevID, magical2ID}, {"magical3", magical3RevID, magical3ID}} {
		t.Run(row.name, func(t *testing.T) {
			t.Parallel()
			w := bootIndividual1f(t, raidBoot{}, ind1fTemplate(row.revived), ind1fTemplate(row.master))
			master := w.spawn(t, row.master, 1500, 0)
			me := w.player.ObjectID()
			named := w.ind4cCallPrivate(t, master, row.revived, 1500, 200, script.Params{P1: 1000, P2: me})
			w.wantCasts(t, "called naming the character", named, map[modelskill.ID]ind1Cast{ind1dStunID: {me, 20000}})
			if got := named.AggroHate(w.player); got != 1000 {
				t.Fatalf("hate of the named character = %v, want 1000", got)
			}
			plain := w.ind4cCallPrivate(t, master, row.revived, 1500, 400, script.Params{})
			w.wantCasts(t, "called naming no one", plain, nil)
			if got := plain.AggroHate(w.player); got != 0 {
				t.Fatalf("hate of the character with no name = %v, want none", got)
			}
		})
	}
}

// TestCast3SkillsMagical2StonePetrifiesAllies ports
// WarriorCast3SkillsMagical2StoneAggressive's see-creature, attacked and
// clan-attacked hooks for a Ketra and a Varka member. A character allied
// with its faction is not attacked on sight; its hit makes it want to
// petrify the character (4515, weight 1000000 from its attacked hook and as
// much again from its own clan call, which the hit makes first to itself),
// stop hating it and want nothing of its parent. A character with no
// alliance is attacked on sight with weight 200.
func TestCast3SkillsMagical2StonePetrifiesAllies(t *testing.T) {
	t.Parallel()
	for _, row := range []struct {
		name     string
		faction  string
		standing int
	}{{"ketra", "ketra_orc_clan", 1}, {"varka", "varka_silenos_clan", -1}} {
		t.Run(row.name, func(t *testing.T) {
			t.Parallel()
			tmpl := ind1fTemplate(magical2StoneID)
			tmpl.Clans, tmpl.ClanRange = []string{row.faction}, 500
			w := bootIndividual1f(t, raidBoot{}, tmpl)
			ch := ind3Character(t, w)
			ch.SetVarkaKetraAlliance(row.standing)

			allied := w.spawn(t, magical2StoneID, 150, 0)
			w.srv.InstallNPCCastSeam(t, allied, modelskill.NewTable([]modelskill.Definition{npcSkill(petrifyID, 1, true), ind1Skill(ind1fDD1ID, 900, false)}))
			w.ticks(t, 2)
			if got := allied.AggroHate(w.player); got != 0 {
				t.Fatalf("hate of an ally it sees = %v, want none", got)
			}
			w.hit(t, allied, 10)
			w.hit(t, allied, 10)
			if d, ok := castDesireOf(allied, petrifyID); !ok || d.FinalTarget == nil || d.FinalTarget.ObjectID() != w.player.ObjectID() {
				t.Fatalf("petrify desire = %+v (%v), want on the character", d, ok)
			}
			for _, id := range []modelskill.ID{ind1fDD1ID, ind1fDD2ID, ind1fDebuffID} {
				if d, ok := castDesireOf(allied, id); ok {
					t.Fatalf("desire of skill %d after an ally's hits = %+v, want none", id, d)
				}
			}
			if got := allied.AggroHate(w.player); got != 0 {
				t.Fatalf("hate of the ally that hit it = %v, want none", got)
			}

			ch.SetVarkaKetraAlliance(0)
			open := w.spawn(t, magical2StoneID, 0, 150)
			w.ticks(t, 2)
			if got := open.AggroHate(w.player); got != 200 {
				t.Fatalf("hate of a character with no alliance it sees = %v, want 200", got)
			}
		})
	}
}

// TestCast3SkillsMagical2StonePetrifiesAlliedCaster ports
// WarriorCast3SkillsMagical2StoneAggressive's see-spell hook for a Ketra
// member that sees the character cast a self buff. A buff with aggro points
// cast by a character with no alliance, or a buff without aggro points cast
// by a Ketra ally, wants no petrify. A buff with aggro points cast by a
// Ketra ally makes it want to petrify the caster (4515, weight 1000000)
// and leaves it no hate.
func TestCast3SkillsMagical2StonePetrifiesAlliedCaster(t *testing.T) {
	t.Parallel()
	tmpl := ind1fTemplate(magical2StoneID)
	tmpl.Clans, tmpl.ClanRange = []string{"ketra_orc_clan"}, 500
	w := bootIndividual1f(t, ind1eCasterBoot(t), tmpl)
	ch := ind3Character(t, w)
	stone := w.spawn(t, magical2StoneID, 150, 0)
	w.srv.InstallNPCCastSeam(t, stone, modelskill.NewTable([]modelskill.Definition{npcSkill(petrifyID, 1, true)}))

	for _, row := range []struct {
		standing int
		skill    modelskill.ID
	}{{0, aggroBuffID}, {1, plainBuffID}} {
		ch.SetVarkaKetraAlliance(row.standing)
		castOwnSkill(w.warriorWorld, row.skill)
		w.srv.Advance(t, 2*time.Second)
		w.srv.ReadQueued(t, w.srv.Client)
		if d, ok := castDesireOf(stone, petrifyID); ok {
			t.Fatalf("standing %d, buff %d: it wants %+v, want no petrify", row.standing, row.skill, d)
		}
	}

	w.clearDesires(t, stone)
	castOwnSkill(w.warriorWorld, aggroBuffID)
	w.srv.AdvanceUntil(t, "the petrify desire", func() bool {
		_, ok := castDesireOf(stone, petrifyID)
		return ok
	})
	if d, _ := castDesireOf(stone, petrifyID); d.Weight != 1000000 || d.FinalTarget == nil || d.FinalTarget.ObjectID() != w.player.ObjectID() {
		t.Fatalf("petrify desire = %+v, want on the caster, weight 1000000", d)
	}
	if got := stone.AggroHate(w.player); got != 0 {
		t.Fatalf("hate of the allied caster = %v, want none", got)
	}
}

// TestCast3SkillsMagical3DebuffsBlastsAndHolds ports
// WarriorCast3SkillsMagical3's created and attacked hooks against the
// unarmed character. It appears with i_ai0 0. Hit by the one it most
// wants to attack it wants its debuff on it and, from farther than 100,
// its DD magic, weight 1000000 each; the hold follows the bands of
// WarriorCastHoldMagic: from 400 on a draw under 50 even with no top
// desire target, from 200 on a draw under 50 by its top desire target or
// under 10 with a decoy its top desire target, never from 50.
func TestCast3SkillsMagical3DebuffsBlastsAndHolds(t *testing.T) {
	t.Parallel()
	for _, row := range []struct {
		name string
		dist int
		top  string // "me", "decoy" or ""
		draw int
		want []modelskill.ID
		held bool
	}{
		{"top, 50 away", 50, "me", 0, []modelskill.ID{ind1fDebuffID}, false},
		{"top, 200 away", 200, "me", 0, []modelskill.ID{ind1fDebuffID, ind1eDDID, ind1fHoldID}, true},
		{"no top, 400 away", 400, "", 0, []modelskill.ID{ind1fHoldID}, true},
		{"no top, 400 away, draw 50", 400, "", 50, nil, false},
		{"decoy, 200 away, draw 9", 200, "decoy", 9, []modelskill.ID{ind1fHoldID}, true},
		{"decoy, 200 away, draw 10", 200, "decoy", 10, nil, false},
	} {
		t.Run(row.name, func(t *testing.T) {
			t.Parallel()
			w := bootIndividual1f(t, raidBoot{}, ind1fTemplate(magical3ID))
			n := w.spawn(t, magical3ID, row.dist, 0)
			ind5eHeld(t, "once created", n, false)
			switch row.top {
			case "me":
				w.setTop(t, n, script.PlayerOf(w.player))
			case "decoy":
				w.setTop(t, n, script.NPCOf(w.spawn(t, unboundID, 0, 1500)))
			}
			w.draws.set(row.draw)
			w.hit(t, n, 10)
			want := map[modelskill.ID]ind1Cast{}
			for _, id := range row.want {
				want[id] = ind1Cast{w.player.ObjectID(), 1000000}
			}
			w.wantCasts(t, "after the hit", n, want)
			ind5eHeld(t, "after the hit", n, row.held)
		})
	}
}

// TestCast3SkillsMagical3HoldsAnArcher ports WarriorCast3SkillsMagical3's
// bow branch: from 50 away, where no range band holds, the character
// attacking with a bow draws the hold on a second draw under 50, not on 50.
func TestCast3SkillsMagical3HoldsAnArcher(t *testing.T) {
	t.Parallel()
	w := bootIndividual1fArcher(t, ind1fTemplate(magical3ID))
	n := w.spawn(t, magical3ID, 50, 0)
	w.draws.set(0, 0, 50)
	w.hit(t, n, 10)
	ind5bNoCast(t, "a hit by an archer, second draw 50", n, ind1fHoldID)
	w.draws.set(0)
	w.hit(t, n, 10)
	ind5bWantCast(t, "a hit by an archer, second draw 0", n, ind1fHoldID, w.player.ObjectID())
	ind5eHeld(t, "a hit by an archer", n, true)
}

// TestCast3SkillsMagical3HoldsItsLastAttackerLater ports
// WarriorCast3SkillsMagical3's timer "2001", started 12 s after a hit:
// unless it held someone, it wants the hold on its last attacker on a draw
// under 80, not on 80.
func TestCast3SkillsMagical3HoldsItsLastAttackerLater(t *testing.T) {
	t.Parallel()
	w := bootIndividual1f(t, raidBoot{}, ind1fTemplate(magical3ID))
	me := w.player.ObjectID()
	missed := w.spawn(t, magical3ID, 50, 0)
	w.hit(t, missed, 10)
	ind5bNoCast(t, "the hit from 50 away", missed, ind1fHoldID)
	w.draws.set(80)
	w.srv.Advance(t, 12*time.Second)
	ind5bNoCast(t, "the timer, draw 80", missed, ind1fHoldID)

	w.draws.set(0)
	n := w.spawn(t, magical3ID, -50, 0)
	w.hit(t, n, 10)
	w.srv.Advance(t, 11*time.Second)
	ind5bNoCast(t, "11 s after the hit", n, ind1fHoldID)
	w.srv.Advance(t, time.Second)
	ind5bWantCast(t, "the timer, draw 0", n, ind1fHoldID, me)
	ind5eHeld(t, "the timer, draw 0", n, true)
}

// TestCast3SkillsMagical3AnswersItsClan ports WarriorCast3SkillsMagical3's
// clan-attacked hook, every draw 0. A settled member that is not attacking
// wants its debuff and, from 300, its DD magic on the character that hits
// its clan, weight 1000000 each. The debuff waits only on the member
// itself holding the debuff's stack: a marked character still draws it.
func TestCast3SkillsMagical3AnswersItsClan(t *testing.T) {
	t.Parallel()
	for _, row := range []struct {
		name   string
		dist   int
		marked string // "member", "character" or ""
		want   []modelskill.ID
	}{
		{"300 away", 300, "", []modelskill.ID{ind1fDebuffID, ind1eDDID}},
		{"50 away", 50, "", []modelskill.ID{ind1fDebuffID}},
		{"member marked", 300, "member", []modelskill.ID{ind1eDDID}},
		{"character marked", 300, "character", []modelskill.ID{ind1fDebuffID, ind1eDDID}},
	} {
		t.Run(row.name, func(t *testing.T) {
			t.Parallel()
			w := bootIndividual1f(t, raidBoot{}, ind1fTemplate(magical3ID))
			member := w.spawn(t, magical3ID, row.dist, 0)
			v := w.spawn(t, unboundID, 0, 100)
			switch row.marked {
			case "member":
				w.ind1fMark(t, script.NPCOf(member))
			case "character":
				w.ind1fMark(t, script.PlayerOf(w.player))
			}
			w.ticks(t, 8)
			w.hit(t, v, 10)
			want := map[modelskill.ID]ind1Cast{}
			for _, id := range row.want {
				want[id] = ind1Cast{w.player.ObjectID(), 1000000}
			}
			w.wantCasts(t, "after the hit on its clan", member, want)
		})
	}
}

// TestCast3SkillsMagical3AggressiveCastsOnSight ports
// WarriorCast3SkillsMagical3Aggressive's see-creature hook, every draw 0.
// Settled and wandering in its territory, it wants its debuff and its DD
// magic on the character it sees 300 away, weight 1000000 each, and turns
// on it with weight 200; fresh it only turns on it.
func TestCast3SkillsMagical3AggressiveCastsOnSight(t *testing.T) {
	t.Parallel()
	for _, settled := range []bool{true, false} {
		t.Run(fmt.Sprintf("settled %v", settled), func(t *testing.T) {
			t.Parallel()
			w := bootIndividual1f(t, raidBoot{}, ind1fTemplate(magical3AggID))
			var n *npc.Hostile
			want := map[modelskill.ID]ind1Cast{}
			if settled {
				n = w.ind1fSettledAtWander(t, magical3AggID)
				for _, id := range []modelskill.ID{ind1fDebuffID, ind1eDDID} {
					want[id] = ind1Cast{w.player.ObjectID(), 1000000}
				}
			} else {
				n = w.spawn(t, magical3AggID, 1500, 0)
			}
			w.moveCharacter(t, n, 300, 0)
			w.ticks(t, 1)
			w.wantCasts(t, "on sight", n, want)
			if got := n.AggroHate(w.player); got != 200 {
				t.Fatalf("hate of the character it sees = %v, want 200", got)
			}
		})
	}
}

// TestCast3SkillsMagical4BuffsThenCancels ports WarriorCast3SkillsMagical4's
// created and attacked hooks, every draw 0. It appears with i_ai0, i_ai1
// and i_ai2 clear. Every hit is first its own clan call, which, while it
// is not attacking, wants its heal on itself. The character's first hit
// also wants its buff on itself and marks i_ai1; the second its cancel on
// the character, once, marking i_ai2, unless the character holds its
// check mark; a third nothing more.
func TestCast3SkillsMagical4BuffsThenCancels(t *testing.T) {
	t.Parallel()
	for _, marked := range []bool{false, true} {
		t.Run(fmt.Sprintf("marked %v", marked), func(t *testing.T) {
			t.Parallel()
			w := bootIndividual1f(t, raidBoot{}, ind1fTemplate(magical4ID))
			n := w.spawn(t, magical4ID, 50, 0)
			me, self := w.player.ObjectID(), n.ObjectID()
			for _, slot := range []script.IntSlot{script.SlotIntAI0, script.SlotIntAI1, script.SlotIntAI2} {
				if got := ind1Slot(n, slot); got != 0 {
					t.Fatalf("created slot %v = %d, want 0", slot, got)
				}
			}
			if marked {
				w.ind5dLand(t, modelskill.Ref{ID: ind1eCheckID, Level: 1}, script.PlayerOf(w.player))
			}
			w.hit(t, n, 10)
			w.wantCasts(t, "after the first hit", n, map[modelskill.ID]ind1Cast{ind1fBuffID: {self, 1000000}, ind1fHealID: {self, 1000000}})
			if got := ind1Slot(n, script.SlotIntAI1); got != 1 {
				t.Fatalf("i_ai1 after the first hit = %d, want 1", got)
			}
			w.clear(t, n)
			w.hit(t, n, 10)
			want := map[modelskill.ID]ind1Cast{ind1fHealID: {self, 1000000}}
			wantI2 := int32(0)
			if !marked {
				want[ind1eCancelID] = ind1Cast{me, 1000000}
				wantI2 = 1
			}
			w.wantCasts(t, "after the second hit", n, want)
			if got := ind1Slot(n, script.SlotIntAI2); got != wantI2 {
				t.Fatalf("i_ai2 after the second hit = %d, want %d", got, wantI2)
			}
			w.clear(t, n)
			w.hit(t, n, 10)
			w.wantCasts(t, "after the third hit", n, map[modelskill.ID]ind1Cast{ind1fHealID: {self, 1000000}})
		})
	}
}

// TestCast3SkillsMagical4HealsItselfWhenHurt ports the heal of
// WarriorCast3SkillsMagical4's attacked hook, every draw 0: hit at half its
// HP, it wants its heal on itself, and no buff, which it keeps for above
// half its HP.
func TestCast3SkillsMagical4HealsItselfWhenHurt(t *testing.T) {
	t.Parallel()
	w := bootIndividual1f(t, raidBoot{}, ind1fTemplate(magical4ID))
	n := w.spawn(t, magical4ID, 50, 0)
	w.setHPShare(t, n, 0.5)
	w.hit(t, n, 10)
	if d, ok := castDesireOf(n, ind1fHealID); !ok || d.FinalTarget == nil || d.FinalTarget.ObjectID() != n.ObjectID() {
		t.Fatalf("heal desire = %+v (%v), want on itself", d, ok)
	}
	ind5bNoCast(t, "a hit at half its HP", n, ind1fBuffID)
}

// TestCast3SkillsMagical4HealsItsClan ports WarriorCast3SkillsMagical4's
// clan-attacked hook: a member that is not attacking wants its heal on
// the clan member the character hits, weight 1000000, on a draw under 33,
// and not on 33.
func TestCast3SkillsMagical4HealsItsClan(t *testing.T) {
	t.Parallel()
	for _, draw := range []int{0, 33} {
		t.Run(fmt.Sprintf("draw %d", draw), func(t *testing.T) {
			t.Parallel()
			w := bootIndividual1f(t, raidBoot{}, ind1fTemplate(magical4ID))
			member := w.spawn(t, magical4ID, 300, 0)
			v := w.spawn(t, unboundID, 0, 100)
			w.draws.set(draw)
			w.hit(t, v, 10)
			want := map[modelskill.ID]ind1Cast{}
			if draw < 33 {
				want[ind1fHealID] = ind1Cast{v.ObjectID(), 1000000}
			}
			w.wantCasts(t, "after the hit on its clan", member, want)
		})
	}
}
