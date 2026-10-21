package ai

import (
	"context"
	"fmt"
	"math"
	"testing"
	"time"

	gamesql "github.com/fatal10110/acis_golang/internal/gameserver/data/sql"
	"github.com/fatal10110/acis_golang/internal/gameserver/data/sql/sqltest"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/npc"
	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/script"
	"github.com/fatal10110/acis_golang/internal/gameserver/script/ai/warrior"
	skillstate "github.com/fatal10110/acis_golang/internal/gameserver/skill"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
)

// The NPC ids the fifth PR of the first individual behavior batch (#186
// PR 5) spawns, each a fixture monster of level 1 unless a test says
// otherwise.
const (
	revivalID    = int32(20830) // WarriorCast3SkillsApproachRevivalAggressive
	cast3StoneID = int32(21324) // WarriorCast3SkillsApproachStoneAggressive
	curseID      = int32(21301) // WarriorCast3SkillsCurse
	curseAggrID  = int32(21302) // WarriorCast3SkillsCurseAggressive
	magicalID    = int32(20610) // WarriorCast3SkillsMagical
	magicalAggID = int32(20822) // WarriorCast3SkillsMagicalAggressive
	revivedID    = int32(20859) // WarriorCast3SkillsMagicalRevived
	valeMasterID = int32(29024) // ZakenPrivateValeMaster
)

// The batch's fixture skills, listed by type.
const (
	ind1eDDID           = modelskill.ID(9601) // DD_MAGIC
	ind1eSleepID        = modelskill.ID(9602) // SLEEP_MAGIC
	ind1eCancelID       = modelskill.ID(9603) // CANCEL_MAGIC
	ind1eCheckID        = modelskill.ID(9604) // CHECK_MAGIC
	ind1eDebuffID       = modelskill.ID(9605) // DEBUFF
	ind1eDebuffCancelID = modelskill.ID(9606) // DEBUFF_CANCEL
)

// ind1eBatch are the fifth PR's classes, in scripts.xml order.
var ind1eBatch = []struct {
	path string
	ctor func() script.Script
}{
	{"script.ai.individual.Monster.WarriorBase.Warrior.WarriorCast3SkillsApproach.WarriorCast3SkillsApproachRevivalAggressive", warrior.Cast3SkillsApproachRevivalAggressive},
	{"script.ai.individual.Monster.WarriorBase.Warrior.WarriorCast3SkillsApproach.WarriorCast3SkillsApproachStoneAggressive", warrior.Cast3SkillsApproachStoneAggressive},
	{"script.ai.individual.Monster.WarriorBase.Warrior.WarriorCast3SkillsMagical.WarriorCast3SkillsCurse", warrior.Cast3SkillsCurse},
	{"script.ai.individual.Monster.WarriorBase.Warrior.WarriorCast3SkillsMagical.WarriorCast3SkillsCurseAggressive", warrior.Cast3SkillsCurseAggressive},
	{"script.ai.individual.Monster.WarriorBase.Warrior.WarriorCast3SkillsMagical.WarriorCast3SkillsMagical", warrior.Cast3SkillsMagical},
	{"script.ai.individual.Monster.WarriorBase.Warrior.WarriorCast3SkillsMagical.WarriorCast3SkillsMagicalAggressive", warrior.Cast3SkillsMagicalAggressive},
	{"script.ai.individual.Monster.WarriorBase.Warrior.WarriorCast3SkillsMagical.WarriorCast3SkillsMagicalRevived", warrior.Cast3SkillsMagicalRevived},
	{"script.ai.individual.Monster.WarriorBase.Warrior.WarriorCast3SkillsMagical.ZakenPrivateValeMaster", warrior.ZakenPrivateValeMaster},
}

// bootIndividual1e boots the fifth PR's classes after the Warrior chain
// beside templates and an unbound monster of the batch's clan, with extra
// and a director script to land effects and call privates through, every
// NPC casting the batch's skills, every draw 0 until a scenario queues
// others.
func bootIndividual1e(t *testing.T, extra raidBoot, templates ...*npc.Template) *ind1World {
	t.Helper()
	list, catalog := warriorScripts()
	for _, c := range ind1eBatch {
		list = append(list, script.Listing{Path: c.path})
		catalog[c.path] = c.ctor
	}
	const director = "test.Ind1Director"
	catalog[director] = func() script.Script { return script.Script{} }
	list = append(list, script.Listing{Path: director})
	templates = append(templates, ind1eTemplate(unboundID, nil))
	kinds := map[int32]script.NPCKind{}
	for _, tmpl := range templates {
		kinds[int32(tmpl.ID)] = script.KindHostile
	}
	draws := &ind1Draws{}
	opts := append([]gameservertest.Option{
		gameservertest.WithScriptRand(draws.rand),
		gameservertest.WithNPCScripts(kinds, list, catalog),
		gameservertest.WithNpcSpawnSkills(modelskill.NewTable(ind1eDefs())),
		gameservertest.WithScriptSkills(ind1eDefs()...),
	}, extra.opts...)
	w := bootRaid(t, raidBoot{opts: opts, ready: extra.ready}, templates...)
	return &ind1World{warriorWorld: w, draws: draws, heard: &ind1Heard{}}
}

// ind1eTemplate is a fixture monster of id with ai as its AI parameters,
// standing still, listing the batch's skills by type, in the clan the
// scenario's NPCs share.
func ind1eTemplate(id int32, ai npc.AIParams) *npc.Template {
	tmpl := ind1cTemplate(id)
	tmpl.AIParams, tmpl.CanMove = ai, false
	for typ, skillID := range map[npc.SkillType]modelskill.ID{
		npc.SkillTypeSelfBuff: ind2SelfBuffSkill, npc.SkillTypeDDMagic: ind1eDDID, npc.SkillTypeSleepMagic: ind1eSleepID,
		npc.SkillTypeCancelMagic: ind1eCancelID, npc.SkillTypeCheckMagic: ind1eCheckID, npc.SkillTypeDebuff: ind1eDebuffID,
		npc.SkillTypeDebuffCancel: ind1eDebuffCancelID,
	} {
		tmpl.SkillsByType[typ] = modelskill.Ref{ID: skillID, Level: 1}
	}
	return tmpl
}

// ind1eDefs are the skills the batch's fixture NPCs cast and land: the
// physical special, the DD magic, the sleep, the cancel and the debuff
// one-target attacks of reach 900, the blast and the self buff self
// skills, the stun of a called NPC, and the check mark and the debuff
// cancel, buffs holding an effect of their own stack for a minute.
func ind1eDefs() []modelskill.Definition {
	mark := func(id modelskill.ID, stack string) modelskill.Definition {
		d := ind1Skill(id, 0, true)
		d.Effects = []modelskill.EffectTemplate{{Name: "Buff", Time: 60, Icon: true, StackType: stack, StackOrder: 1}}
		return d
	}
	return []modelskill.Definition{
		ind1Skill(ind1cSpecialID, 900, false), ind1Skill(ind1cBlastID, 0, true), ind2SelfBuff(), ind1Skill(ind1dStunID, 900, false),
		ind1Skill(ind1eDDID, 900, false), ind1Skill(ind1eSleepID, 900, false), ind1Skill(ind1eCancelID, 900, false),
		ind1Skill(ind1eDebuffID, 900, false), mark(ind1eCheckID, "ind1e_check"), mark(ind1eDebuffCancelID, "ind1e_cancel"),
	}
}

// ind1eKill kills h with the character as its killer, and lets its dying
// hooks run.
func (w *ind1World) ind1eKill(t *testing.T, h *npc.Hostile) {
	t.Helper()
	w.onQueue(t, h, func() { script.NPCOf(h).DoDie(script.PlayerOf(w.player)) })
	w.srv.ReadQueued(t, w.srv.Client)
	w.srv.Advance(t, 3500*time.Millisecond)
}

// ind1eLevel returns the level at which the character holds an effect of
// ref's kind, -1 for none.
func (w *ind1World) ind1eLevel(t *testing.T, ref modelskill.Ref) int32 {
	t.Helper()
	level := int32(-1)
	w.srv.RunScript(t, w.player.ObjectID(), "Ind1Director", func(s *script.Script, p *script.Player) {
		level = s.AbnormalLevel(p, ref)
	})
	return level
}

// TestCast3SkillsMagicalBlastsAndCancels ports WarriorCast3SkillsMagical's
// attacked hook, every draw 0. Its first hit finds it hating no one, so only
// its parent answers. Hit again by the character it hates most from 300,
// it wants its DD magic on the character; from 50 its cancel, once: a third
// hit wants nothing, and a character holding its check mark is never
// cancelled.
func TestCast3SkillsMagicalBlastsAndCancels(t *testing.T) {
	t.Parallel()
	for _, row := range []struct {
		name   string
		dist   int
		marked bool
		want   modelskill.ID
	}{{"far", 300, false, ind1eDDID}, {"close", 50, false, ind1eCancelID}, {"close and marked", 50, true, 0}} {
		t.Run(row.name, func(t *testing.T) {
			t.Parallel()
			w := bootIndividual1e(t, raidBoot{}, ind1eTemplate(magicalID, nil))
			n := w.spawn(t, magicalID, row.dist, 0)
			me := w.player.ObjectID()
			if row.marked {
				w.ind5dLand(t, modelskill.Ref{ID: ind1eCheckID, Level: 1}, script.PlayerOf(w.player))
			}
			w.hit(t, n, 10)
			w.wantCasts(t, "after the first hit", n, nil)
			if got := n.AggroHate(w.player); got <= 0 {
				t.Fatalf("hate after the first hit = %v, want its parent's", got)
			}
			w.hit(t, n, 10)
			want := map[modelskill.ID]ind1Cast{}
			if row.want != 0 {
				want[row.want] = ind1Cast{me, 1000000}
			}
			w.wantCasts(t, "after the second hit", n, want)
			if row.dist > 100 {
				return
			}
			wantI1 := int32(1)
			if row.marked {
				wantI1 = 0
			}
			if got := ind1Slot(n, script.SlotIntAI1); got != wantI1 {
				t.Fatalf("i_ai1 after the second hit = %d, want %d", got, wantI1)
			}
			w.clear(t, n)
			w.hit(t, n, 10)
			w.wantCasts(t, "after the third hit", n, nil)
		})
	}
}

// TestCast3SkillsMagicalSleepsASecondAttacker ports the attacked hook's
// sleep, every draw 0. Hating another NPC most, the first hit by the
// character only sets i_ai0; the second wants its sleep on the character,
// weight 1000000.
func TestCast3SkillsMagicalSleepsASecondAttacker(t *testing.T) {
	t.Parallel()
	w := bootIndividual1e(t, raidBoot{}, ind1eTemplate(magicalID, nil))
	n := w.spawn(t, magicalID, 300, 0)
	other := w.spawn(t, unboundID, 300, 200)
	ind2Hate(t, w, n, other, 100000, 100)
	w.hit(t, n, 10)
	w.wantCasts(t, "after the first hit", n, nil)
	if got := ind1Slot(n, script.SlotIntAI0); got != 1 {
		t.Fatalf("i_ai0 after the first hit = %d, want 1", got)
	}
	w.hit(t, n, 10)
	w.wantCasts(t, "after the second hit", n, map[modelskill.ID]ind1Cast{ind1eSleepID: {w.player.ObjectID(), 1000000}})
}

// TestCast3SkillsMagicalAnswersItsClan ports WarriorCast3SkillsMagical's
// clan-attacked hook, every draw 0. A settled member that is not attacking
// wants its DD magic on the character 300 away that hits its clan, weight
// 1000000, and its parent's hook adds int(10 / 8 * 100) = 125 hate; a
// member 50 away wants nothing but the hate.
func TestCast3SkillsMagicalAnswersItsClan(t *testing.T) {
	t.Parallel()
	for _, row := range []struct {
		dist  int
		blast bool
	}{{300, true}, {50, false}} {
		t.Run(fmt.Sprintf("%d away", row.dist), func(t *testing.T) {
			t.Parallel()
			w := bootIndividual1e(t, raidBoot{}, ind1eTemplate(magicalID, nil))
			member := w.spawn(t, magicalID, row.dist, 0)
			v := w.spawn(t, unboundID, 0, 100)
			w.ticks(t, 8)
			w.hit(t, v, 10)
			want := map[modelskill.ID]ind1Cast{}
			if row.blast {
				want[ind1eDDID] = ind1Cast{w.player.ObjectID(), 1000000}
			}
			w.wantCasts(t, "after the hit on its clan", member, want)
			if got := member.AggroHate(w.player); got != 125 {
				t.Fatalf("member hate after the hit on its clan = %v, want 125", got)
			}
		})
	}
}

// TestCast3SkillsMagicalAggressiveBlastsOnSight ports
// WarriorCast3SkillsMagicalAggressive's see-creature hook, every draw 0.
// Settled, out of a fight and in its territory, it wants its DD magic on
// the character it sees 300 away, weight 1000000, and turns on it; fresh it
// only turns on it.
func TestCast3SkillsMagicalAggressiveBlastsOnSight(t *testing.T) {
	t.Parallel()
	for _, settled := range []bool{true, false} {
		t.Run(fmt.Sprintf("settled %v", settled), func(t *testing.T) {
			t.Parallel()
			w := bootIndividual1e(t, raidBoot{}, ind1eTemplate(magicalAggID, nil))
			n := w.spawn(t, magicalAggID, 3000, 0)
			if settled {
				w.ticks(t, 8)
			}
			w.clear(t, n)
			w.moveCharacter(t, n, 300, 0)
			w.srv.Settle(t)
			w.ticks(t, 1)
			if got := n.AggroHate(w.player); got == 0 {
				t.Fatal("no hate of the character it sees")
			}
			if settled {
				ind5bWantCast(t, "after it saw the character", n, ind1eDDID, w.player.ObjectID())
			} else {
				ind5bNoCast(t, "after it saw the character", n, ind1eDDID)
			}
		})
	}
}

// TestCast3SkillsMagicalRevivedStunsTheNamedCreature ports
// WarriorCast3SkillsMagicalRevived's created hook. Called with spawn
// parameters 1000 and the character, it wants its stun on the character,
// weight 10000 from it and 10000 from its WarriorBase parent, and to attack
// the character, 500 from each; called with none it wants neither.
func TestCast3SkillsMagicalRevivedStunsTheNamedCreature(t *testing.T) {
	t.Parallel()
	w := bootIndividual1e(t, raidBoot{}, ind1eTemplate(revivedID, nil), ind1eTemplate(magicalID, nil))
	master := w.spawn(t, magicalID, 1500, 0)
	me := w.player.ObjectID()
	named := w.ind4cCallPrivate(t, master, revivedID, 1500, 200, script.Params{P1: 1000, P2: me})
	w.wantCasts(t, "called naming the character", named, map[modelskill.ID]ind1Cast{ind1dStunID: {me, 20000}})
	if got := named.AggroHate(w.player); got != 1000 {
		t.Fatalf("hate of the named character = %v, want 1000", got)
	}
	plain := w.ind4cCallPrivate(t, master, revivedID, 1500, 400, script.Params{})
	w.wantCasts(t, "called naming no one", plain, nil)
	if got := plain.AggroHate(w.player); got != 0 {
		t.Fatalf("hate of the character with no name = %v, want none", got)
	}
}

// TestCast3SkillsCurseDebuffsWhenLeftAlone ports WarriorCast3SkillsCurse's
// created, attacked and timer hooks, every draw 0. It appears with i_ai3
// and i_ai4 clear and itself in c_ai2. Its first hit arms the 5 s timer
// "5001": left alone, the timer wants its debuff on the character it hates
// most, weight 1000000, and keeps the character in c_ai2. A second hit
// before the timer defers the debuff by 5 s more.
func TestCast3SkillsCurseDebuffsWhenLeftAlone(t *testing.T) {
	t.Parallel()
	for _, hits := range []int{1, 2} {
		t.Run(fmt.Sprintf("%d hits", hits), func(t *testing.T) {
			t.Parallel()
			w := bootIndividual1e(t, raidBoot{}, ind1eTemplate(curseID, nil))
			n := w.spawn(t, curseID, 1500, 0)
			me := w.player.ObjectID()
			if ind1Slot(n, script.SlotIntAI3) != 0 || ind1Slot(n, script.SlotIntAI4) != 0 {
				t.Fatalf("created i_ai3 %d, i_ai4 %d, want both 0", ind1Slot(n, script.SlotIntAI3), ind1Slot(n, script.SlotIntAI4))
			}
			if c := script.NPCOf(n).Slot(script.SlotCreature2); c == nil || c.ObjectID() != n.ObjectID() {
				t.Fatalf("created c_ai2 = %v, want itself", c)
			}
			for range hits {
				w.hit(t, n, 10)
			}
			// The second hit, by the character it now hates most from 1500,
			// also wants its WarriorCast3SkillsMagical parent's DD magic.
			want := map[modelskill.ID]ind1Cast{}
			if hits == 2 {
				want[ind1eDDID] = ind1Cast{me, 1000000}
			}
			w.wantCasts(t, "after the hits", n, want)
			w.srv.Advance(t, 5*time.Second)
			w.srv.Settle(t)
			if hits == 2 {
				w.wantCasts(t, "5 s after two hits", n, want)
				w.srv.Advance(t, 5*time.Second)
				w.srv.Settle(t)
			}
			want[ind1eDebuffID] = ind1Cast{me, 1000000}
			w.wantCasts(t, "after the timer", n, want)
			if c := script.NPCOf(n).Slot(script.SlotCreature2); c == nil || c.ObjectID() != me {
				t.Fatalf("c_ai2 after the debuff = %v, want the character", c)
			}
		})
	}
}

// TestCast3SkillsCurseLiftsItsDebuffAsItDies ports WarriorCast3SkillsCurse's
// dying hook. One that dies remembering only itself lands nothing; dying
// with the character in c_ai2, its debuff cancel lands on the character.
func TestCast3SkillsCurseLiftsItsDebuffAsItDies(t *testing.T) {
	t.Parallel()
	w := bootIndividual1e(t, raidBoot{}, ind1eTemplate(curseID, nil))
	ref := modelskill.Ref{ID: ind1eDebuffCancelID, Level: 1}
	uncursed := w.spawn(t, curseID, 1500, 0)
	w.die(t, uncursed)
	if got := w.ind1eLevel(t, ref); got > 0 {
		t.Fatalf("debuff cancel level after the death of one that cursed no one = %d, want none", got)
	}
	cursed := w.spawn(t, curseID, 0, 1500)
	w.onQueue(t, cursed, func() { script.NPCOf(cursed).SetSlot(script.SlotCreature2, script.PlayerOf(w.player)) })
	w.onQueue(t, cursed, func() { script.NPCOf(cursed).DoDie(nil) })
	w.srv.AdvanceUntil(t, "the debuff cancel landed", func() bool { return w.ind1eLevel(t, ref) > 0 })
}

// TestCast3SkillsCurseAggressiveAttacksOnSight ports
// WarriorCast3SkillsCurseAggressive's see-creature hook: it wants to attack
// the character it sees, weight 200, where its parent sees it and wants
// nothing.
func TestCast3SkillsCurseAggressiveAttacksOnSight(t *testing.T) {
	t.Parallel()
	w := bootIndividual1e(t, raidBoot{}, ind1eTemplate(curseAggrID, nil), ind1eTemplate(curseID, nil))
	aggr := w.spawn(t, curseAggrID, 150, 0)
	plain := w.spawn(t, curseID, 0, 150)
	w.ticks(t, 2)
	if got := aggr.AggroHate(w.player); got != 200 {
		t.Fatalf("hate of the character it sees = %v, want 200", got)
	}
	if got := plain.AggroHate(w.player); got != 0 {
		t.Fatalf("its parent's hate of the character it sees = %v, want none", got)
	}
}

// TestCast3SkillsApproachRevivalLeavesItsSilhouette ports
// WarriorCast3SkillsApproachRevivalAggressive's created, attacked and dying
// hooks. It appears with i_ai4 clear; a hit keeps its top desire target in
// i_ai4 when that is a player or summon: not itself, the character. Killed by the character it leaves its silhouette (a
// WarriorCast3SkillsMagicalRevived) where it fell, called with spawn
// parameters 1000 and the character, which wants to stun and attack it.
// Its parent's dying hook never runs. One that dies with no killer leaves
// nothing.
func TestCast3SkillsApproachRevivalLeavesItsSilhouette(t *testing.T) {
	t.Parallel()
	ai := npc.AIParams{"silhouette": fmt.Sprint(revivedID)}
	w := bootIndividual1e(t, raidBoot{}, ind1eTemplate(revivalID, ai), ind1eTemplate(revivedID, nil))
	me := w.player.ObjectID()
	n := w.spawn(t, revivalID, 1500, 0)
	if got := ind1Slot(n, script.SlotIntAI4); got != 0 {
		t.Fatalf("created i_ai4 = %d, want 0", got)
	}
	w.setTop(t, n, script.NPCOf(n))
	w.hit(t, n, 10)
	if got := ind1Slot(n, script.SlotIntAI4); got != 0 {
		t.Fatalf("i_ai4 after a hit with itself as its top desire target = %d, want 0", got)
	}
	w.setTop(t, n, script.PlayerOf(w.player))
	w.hit(t, n, 10)
	if got := ind1Slot(n, script.SlotIntAI4); got != me {
		t.Fatalf("i_ai4 after the hit = %d, want the character %d", got, me)
	}
	x, y, _ := n.Position()
	w.ind1eKill(t, n)
	silhouettes := n.Minions()
	if len(silhouettes) != 1 {
		t.Fatalf("silhouettes left = %d, want 1", len(silhouettes))
	}
	s := silhouettes[0].(*npc.Hostile)
	if sx, sy, _ := s.Position(); s.NpcID() != int(revivedID) || sx != x || sy != y {
		t.Fatalf("silhouette %d at %d,%d, want %d where it fell, %d,%d", s.NpcID(), sx, sy, revivedID, x, y)
	}
	w.wantCasts(t, "the silhouette as it appears", s, map[modelskill.ID]ind1Cast{ind1dStunID: {me, 20000}})

	lone := w.spawn(t, revivalID, 0, 1500)
	w.die(t, lone)
	if got := len(lone.Minions()); got != 0 {
		t.Fatalf("silhouettes left with no killer = %d, want none", got)
	}
}

// TestCast3SkillsApproachStonePetrifiesAllies ports
// WarriorCast3SkillsApproachStoneAggressive's see-creature, attacked and
// clan-attacked hooks for a Ketra and a Varka member. A character allied
// with its faction is not attacked on sight; its hit makes it want to
// petrify the character (4515, weight 1000000 from its attacked hook and as
// much again from its own clan call, which the hit makes first to itself),
// stop hating it and strike it with nothing else. A character with no
// alliance is attacked on sight with weight 200.
func TestCast3SkillsApproachStonePetrifiesAllies(t *testing.T) {
	t.Parallel()
	for _, row := range []struct {
		name     string
		faction  string
		standing int
	}{{"ketra", "ketra_orc_clan", 1}, {"varka", "varka_silenos_clan", -1}} {
		t.Run(row.name, func(t *testing.T) {
			t.Parallel()
			tmpl := ind1eTemplate(cast3StoneID, nil)
			tmpl.Clans, tmpl.ClanRange = []string{row.faction}, 500
			w := bootIndividual1e(t, raidBoot{}, tmpl)
			ch := ind3Character(t, w)
			ch.SetVarkaKetraAlliance(row.standing)

			allied := w.spawn(t, cast3StoneID, 150, 0)
			w.srv.InstallNPCCastSeam(t, allied, modelskill.NewTable([]modelskill.Definition{npcSkill(petrifyID, 1, true), ind1Skill(ind1cSpecialID, 900, false)}))
			w.ticks(t, 2)
			if got := allied.AggroHate(w.player); got != 0 {
				t.Fatalf("hate of an ally it sees = %v, want none", got)
			}
			w.hit(t, allied, 10)
			if d, ok := castDesireOf(allied, petrifyID); !ok || d.Weight != 2000000 || d.FinalTarget == nil || d.FinalTarget.ObjectID() != w.player.ObjectID() {
				t.Fatalf("petrify desire = %+v (%v), want on the character, weight 2000000", d, ok)
			}
			if d, ok := castDesireOf(allied, ind1cSpecialID); ok {
				t.Fatalf("physical special desire after an ally's hit = %+v, want none", d)
			}
			if got := allied.AggroHate(w.player); got != 0 {
				t.Fatalf("hate of the ally that hit it = %v, want none", got)
			}

			ch.SetVarkaKetraAlliance(0)
			open := w.spawn(t, cast3StoneID, 0, 150)
			w.ticks(t, 2)
			if got := open.AggroHate(w.player); got != 200 {
				t.Fatalf("hate of a character with no alliance it sees = %v, want 200", got)
			}
		})
	}
}

// TestCast3SkillsApproachStonePetrifiesAlliedCaster ports
// WarriorCast3SkillsApproachStoneAggressive's see-spell hook for a Ketra
// member that sees the character cast a self buff. A buff with aggro points
// cast by a character with no alliance, or a buff without aggro points cast
// by a Ketra ally, goes on to the parent hooks, which want no petrify. A
// buff with aggro points cast by a Ketra ally makes it want to petrify the
// caster (4515, weight 1000000) and leaves it no hate.
func TestCast3SkillsApproachStonePetrifiesAlliedCaster(t *testing.T) {
	t.Parallel()
	tmpl := ind1eTemplate(cast3StoneID, nil)
	tmpl.Clans, tmpl.ClanRange = []string{"ketra_orc_clan"}, 500
	w := bootIndividual1e(t, ind1eCasterBoot(t), tmpl)
	ch := ind3Character(t, w)
	stone := w.spawn(t, cast3StoneID, 150, 0)
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

// ind1eCasterBoot boots the character knowing two self buffs, one with
// aggro points and one without.
func ind1eCasterBoot(t *testing.T) raidBoot {
	t.Helper()
	buff := func(id modelskill.ID, aggro int) modelskill.Definition {
		return modelskill.Definition{
			ID: id, Level: 1, Activation: modelskill.ActivationActive, Target: modelskill.TargetSelf,
			SkillType: "BUFF", HitTime: 500, StaticHitTime: true, StaticReuse: true, AggroPoints: aggro,
		}
	}
	db := sqltest.SharedDB(t)
	defs := []modelskill.Definition{buff(aggroBuffID, 100), buff(plainBuffID, 0)}
	table := modelskill.NewTable(append([]modelskill.Definition{{ID: 248, Level: 3}, {ID: 294, Level: 1}}, defs...))
	return raidBoot{
		opts: []gameservertest.Option{
			gameservertest.WithSkills(skillstate.NewPersistence(gamesql.NewSkillSaveStore(db), table, gamesql.NewCharacterSkillStore(db))),
			gameservertest.WithScriptSkills(defs...),
		},
		ready: func(srv *gameservertest.Server, objID int32) {
			for _, d := range defs {
				if err := srv.KnownSkills.SetKnownSkill(context.Background(), objID, 0, int(d.ID), 1); err != nil {
					t.Fatalf("seed skill %d: %v", d.ID, err)
				}
			}
		},
	}
}

// TestZakenPrivateValeMasterKeepsWhereItWasCreated ports
// ZakenPrivateValeMaster's created hook: it keeps the x, y and z it was
// created at in its weight point, respawn time and flag, and, calling no
// parent, never shouts as a created Warrior does.
func TestZakenPrivateValeMasterKeepsWhereItWasCreated(t *testing.T) {
	t.Parallel()
	shout := npc.AIParams{"ShoutMsg1": ind2dCreatedShout}
	w := bootIndividual1e(t, raidBoot{}, ind1eTemplate(valeMasterID, shout))
	v := w.spawnFrames(t, valeMasterID, 700, 40)
	if got := says(v.frames, v.h.ObjectID()); len(got) != 0 {
		t.Fatalf("vale master created shouts = %q, want none", got)
	}
	x, y, z := v.h.Position()
	scratch := script.NPCOf(v.h)
	for _, slot := range []struct {
		name string
		slot script.IntSlot
		want int
	}{{"weight point", script.SlotWeightPoint, x}, {"respawn time", npc.IntRespawnTime, y}, {"flag", script.SlotFlag, z}} {
		if got := scratch.Int(slot.slot); int(got) != slot.want {
			t.Errorf("created %s = %d, want %d", slot.name, got, slot.want)
		}
	}
}

// TestZakenPrivateValeMasterSeesOnlyItsHeight ports ZakenPrivateValeMaster's
// see-creature hook. It notices the character only less than 100 above or
// below it: in its territory it wants to attack it (hate 200), then runs
// its own attacked hook as if the character had hit it for 9, which hates
// by that damage. At 100 above or below it notices nothing.
func TestZakenPrivateValeMasterSeesOnlyItsHeight(t *testing.T) {
	t.Parallel()
	for _, row := range []struct {
		dz     int
		notice bool
	}{{0, true}, {99, true}, {-99, true}, {100, false}, {-100, false}} {
		t.Run(fmt.Sprintf("dz %d", row.dz), func(t *testing.T) {
			t.Parallel()
			tmpl := ind1eTemplate(valeMasterID, nil)
			tmpl.Level = 15
			w := bootIndividual1e(t, raidBoot{}, tmpl)
			n := w.spawn(t, valeMasterID, 1500, 0)
			w.ticks(t, 9)
			obj, ok := w.srv.State.Player(w.player.ObjectID())
			if !ok {
				t.Fatal("the character is not in the world")
			}
			x, y, z := n.Position()
			if err := w.srv.State.Move(obj, x-50, y, z+row.dz); err != nil {
				t.Fatalf("move the character: %v", err)
			}
			w.ticks(t, 1)
			want := 0.0
			if row.notice {
				// The attacked hook the sight runs hates by the damage, 100
				// for each level-and-seven of it.
				want = 200 + 9.0/float64(15+7)*100
			}
			if got := n.AggroHate(w.player); math.Abs(got-want) > 1e-9 {
				t.Fatalf("hate of the character %d above = %v, want %v", row.dz, got, want)
			}
		})
	}
}

// TestZakenPrivateValeMasterCursesAndMutesOverLevelled ports
// ZakenPrivateValeMaster's attacked and see-spell hooks. Against the level
// 20 character, a level 11 vale master (9 levels below, more than 8) wants
// its raid curse on the character when hit and its raid mute, instead of
// its parent's answer, when it sees the character cast, each weight
// 1000000; a level 12 one (8 levels below) wants neither. Standing 150
// below the character it never notices it by sight, and 50 away its first
// hit wants no DD magic.
func TestZakenPrivateValeMasterCursesAndMutesOverLevelled(t *testing.T) {
	t.Parallel()
	for _, row := range []struct {
		level int
		curse bool
	}{{11, true}, {12, false}} {
		t.Run(fmt.Sprintf("level %d", row.level), func(t *testing.T) {
			t.Parallel()
			tmpl := ind1eTemplate(valeMasterID, nil)
			tmpl.Level = row.level
			w := bootIndividual1e(t, raidSpellBoot(t), tmpl)
			n := w.spawnBelow(t, valeMasterID, 50, 0, 150)
			w.srv.InstallNPCCastSeam(t, n, modelskill.NewTable([]modelskill.Definition{raidCurseDefinition(), ind1Skill(premoMuteID, 600, false)}))
			me := w.player.ObjectID()

			w.hit(t, n, 10)
			want := map[modelskill.ID]ind1Cast{}
			if row.curse {
				want[raidCurseID] = ind1Cast{me, 1000000}
			}
			w.wantCasts(t, "after the hit", n, want)
			if got := n.AggroHate(w.player); got <= 0 {
				t.Errorf("hate after the hit = %v, want the parent's", got)
			}

			w.castSelfBuff()
			if row.curse {
				w.srv.AdvanceUntil(t, "the raid mute desire", func() bool { return hasCast(n, premoMuteID) })
				want[premoMuteID] = ind1Cast{me, 1000000}
			} else {
				w.srv.Advance(t, 2*time.Second)
			}
			w.wantCasts(t, "after the spell", n, want)
		})
	}
}
