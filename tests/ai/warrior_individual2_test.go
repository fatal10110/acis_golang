package ai

import (
	"fmt"
	"testing"
	"time"

	actorai "github.com/fatal10110/acis_golang/internal/gameserver/model/actor/ai"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/npc"
	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/script"
	"github.com/fatal10110/acis_golang/internal/gameserver/script/ai/warrior"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
)

// The second individual behavior batch's NPC ids the scenarios spawn (#187
// PR 1), each a fixture monster of level 1 unless a scenario says
// otherwise.
const (
	ddMagicID          = int32(20037) // WarriorCastDDMagic
	ddMagicTwoID       = int32(20172) // WarriorCastDDMagic
	ddAggressiveID     = int32(21421) // WarriorCastDDMagicAggressive
	servantID          = int32(35632) // Agit02DoomServantAgit
	servantTwoID       = int32(35411) // Agit02DoomServantAgit
	corePrivateID      = int32(29008) // CorePrivateWarriorCastDDMagicAggressive
	perumID            = int32(29012) // CorePrivatePerum
	selfBufferID       = int32(21405) // WarriorCastDDMagicSelfBuff
	selfBuffAggroID    = int32(29992) // WarriorCastDDMagicSelfBuffAggressive, bound by the test
	curserID           = int32(20075) // WarriorCastingCurse
	curserTwoID        = int32(20261) // WarriorCastingCurse
	ind2DDMagicSkill   = modelskill.ID(9501)
	ind2DebuffSkill    = modelskill.ID(9502)
	ind2SelfBuffSkill  = modelskill.ID(9503)
	ind2SelfBuffStack  = "ind2_self_buff"
	ind2SelfBuffAggroP = "test.WarriorCastDDMagicSelfBuffAggressive"
)

// ind2Batch are the batch's classes, in scripts.xml order.
var ind2Batch = []struct {
	path string
	ctor func() script.Script
}{
	{"script.ai.individual.Monster.WarriorBase.Warrior.WarriorCastDDMagic.Agit02DoomServantAgit", warrior.Agit02DoomServantAgit},
	{"script.ai.individual.Monster.WarriorBase.Warrior.WarriorCastDDMagic.CorePrivateWarriorCastDDMagicAggressive.CorePrivatePerum", warrior.CorePrivatePerum},
	{"script.ai.individual.Monster.WarriorBase.Warrior.WarriorCastDDMagic.CorePrivateWarriorCastDDMagicAggressive.CorePrivateWarriorCastDDMagicAggressive", warrior.CorePrivateCastDDMagicAggressive},
	{"script.ai.individual.Monster.WarriorBase.Warrior.WarriorCastDDMagic.WarriorCastDDMagic", warrior.CastDDMagic},
	{"script.ai.individual.Monster.WarriorBase.Warrior.WarriorCastDDMagic.WarriorCastDDMagicAggressive", warrior.CastDDMagicAggressive},
	{"script.ai.individual.Monster.WarriorBase.Warrior.WarriorCastDDMagic.WarriorCastDDMagicSelfBuff", warrior.CastDDMagicSelfBuff},
	{"script.ai.individual.Monster.WarriorBase.Warrior.WarriorCastDDMagic.WarriorCastDDMagicSelfBuffAggressive", warrior.CastDDMagicSelfBuffAggressive},
	{"script.ai.individual.Monster.WarriorBase.Warrior.WarriorCastingCurse.WarriorCastingCurse", warrior.CastingCurse},
	// WarriorCastDDMagicSelfBuffAggressive binds no NPC of its own; the
	// scenarios bind its hooks to a fixture id to drive them.
	{ind2SelfBuffAggroP, func() script.Script {
		s := warrior.CastDDMagicSelfBuffAggressive()
		s.NPCs = []int32{selfBuffAggroID}
		return s
	}},
}

// bootIndividual2 boots the batch beside templates, with extra, every draw
// 0 until a scenario queues others.
func bootIndividual2(t *testing.T, extra raidBoot, templates ...*npc.Template) *ind1World {
	t.Helper()
	list, catalog := warriorScripts()
	for _, c := range ind2Batch {
		list = append(list, script.Listing{Path: c.path})
		catalog[c.path] = c.ctor
	}
	const director = "test.Ind2Director"
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

// ind2Template is a fixture batch NPC of id listing its DD magic, debuff
// and self buff by type, in the clan the scenario's NPCs share.
func ind2Template(id int32) *npc.Template {
	tmpl := warriorTemplate(id, nil)
	tmpl.SkillsByType = map[npc.SkillType]modelskill.Ref{
		npc.SkillTypeDDMagic:  {ID: ind2DDMagicSkill, Level: 1},
		npc.SkillTypeDebuff:   {ID: ind2DebuffSkill, Level: 1},
		npc.SkillTypeSelfBuff: {ID: ind2SelfBuffSkill, Level: 1},
	}
	tmpl.Clans, tmpl.ClanRange = []string{"doom"}, 500
	return tmpl
}

// ind2SelfBuff is the fixture self buff, holding an effect of its own
// stack for a minute.
func ind2SelfBuff() modelskill.Definition {
	d := ind1Skill(ind2SelfBuffSkill, 0, true)
	d.Effects = []modelskill.EffectTemplate{{Name: "Buff", Time: 60, Icon: true, StackType: ind2SelfBuffStack, StackOrder: 1}}
	return d
}

// ind2Skills are the batch NPCs' skills, the DD magic and debuff reaching
// 2000.
func ind2Skills() *modelskill.Table {
	return modelskill.NewTable([]modelskill.Definition{ind1Skill(ind2DDMagicSkill, 2000, false), ind1Skill(ind2DebuffSkill, 2000, false), ind2SelfBuff()})
}

// moveCharacter puts the character dx, dy from h.
func (w *ind1World) moveCharacter(t *testing.T, h *npc.Hostile, dx, dy int) {
	t.Helper()
	obj, ok := w.srv.State.Player(w.player.ObjectID())
	if !ok {
		t.Fatal("the character is not in the world")
	}
	x, y, z := h.Position()
	if err := w.srv.State.Move(obj, x+dx, y+dy, z); err != nil {
		t.Fatalf("move the character: %v", err)
	}
}

// TestWarriorCastDDMagicCastsAtWhatItFights ports WarriorCastDDMagic's
// created, attacked and timer hooks, every draw 0. The NPC spawns with
// i_ai1 and i_ai2 clear. The first hit arms the 5 s timer "2001" with no
// DD magic: the character is not yet the most hated. Once it fights, the
// timer wants the DD magic on the most hated, weight 1000000, which the
// NPC then casts (MagicSkillUse). A hit by the most hated wants the DD
// magic and marks the next tick to skip it; out of a fight the tick clears
// both slots and the timer stops.
func TestWarriorCastDDMagicCastsAtWhatItFights(t *testing.T) {
	t.Parallel()
	w := bootIndividual2(t, raidBoot{}, ind2Template(ddMagicID))
	n := w.spawn(t, ddMagicID, 300, 0)
	w.srv.InstallNPCCastSeam(t, n, ind2Skills())
	me := w.player.ObjectID()
	if a1, a2 := ind1Slot(n, script.SlotIntAI1), ind1Slot(n, script.SlotIntAI2); a1 != 0 || a2 != 0 {
		t.Fatalf("created i_ai1, i_ai2 = %d, %d, want 0, 0", a1, a2)
	}

	w.hit(t, n, 10)
	w.wantCasts(t, "after the first hit", n, nil)
	if got := ind1Slot(n, script.SlotIntAI1); got != 1 {
		t.Fatalf("i_ai1 after the first hit = %d, want 1", got)
	}
	w.ticks(t, 1)
	if got := n.AI().CurrentIntention(); got != actorai.IntentionAttack {
		t.Fatalf("intention after the first hit = %v, want attack", got)
	}
	w.srv.Advance(t, 5*time.Second)
	w.wantCasts(t, "after the first tick", n, map[modelskill.ID]ind1Cast{ind2DDMagicSkill: {me, 1000000}})
	if got := castsOf(w.ticks(t, 2), n, ind2DDMagicSkill); len(got) != 1 || got[0] != me {
		t.Fatalf("DD magic MagicSkillUse targets = %v, want one on %d", got, me)
	}

	w.onQueue(t, n, func() { n.AI().Desires().Clear() })
	w.hit(t, n, 10)
	w.wantCasts(t, "after a hit by the most hated", n, map[modelskill.ID]ind1Cast{ind2DDMagicSkill: {me, 1000000}})
	if got := ind1Slot(n, script.SlotIntAI2); got != 1 {
		t.Fatalf("i_ai2 after a hit by the most hated = %d, want 1", got)
	}
	w.onQueue(t, n, func() { n.AI().Desires().Clear() })
	w.ticks(t, 1)
	w.srv.Advance(t, 5*time.Second)
	if got := ind1Slot(n, script.SlotIntAI2); got != 0 {
		t.Fatalf("i_ai2 after the tick it skipped = %d, want 0", got)
	}
	w.wantCasts(t, "after the skipped tick", n, nil)

	w.onQueue(t, n, func() { script.NPCOf(n).RemoveAllAttackDesire(); n.AI().Desires().Clear() })
	w.ticks(t, 1)
	if got := n.AI().CurrentIntention(); got == actorai.IntentionAttack || got == actorai.IntentionCast {
		t.Fatalf("intention after the desires were dropped = %v, want neither attack nor cast", got)
	}
	w.srv.Advance(t, 5*time.Second)
	if a1, a2 := ind1Slot(n, script.SlotIntAI1), ind1Slot(n, script.SlotIntAI2); a1 != 0 || a2 != 0 {
		t.Fatalf("i_ai1, i_ai2 after the tick out of a fight = %d, %d, want 0, 0", a1, a2)
	}
	w.srv.Advance(t, 10*time.Second)
	w.wantCasts(t, "once the timer stopped", n, nil)
}

// TestWarriorCastDDMagicAnswersItsClanFromAfar ports WarriorCastDDMagic's
// clan-attacked hook, every draw 0: a settled clan member that is not
// attacking wants its DD magic, weight 1000000, on the attacker of another
// member when the attacker stands more than 100 from it; one standing at
// 50 gets none, and neither does a young member.
func TestWarriorCastDDMagicAnswersItsClanFromAfar(t *testing.T) {
	t.Parallel()
	for _, row := range []struct {
		name    string
		settle  int
		closeBy bool
		cast    bool
	}{{"far", 8, false, true}, {"near", 8, true, false}, {"young", 0, false, false}} {
		t.Run(row.name, func(t *testing.T) {
			t.Parallel()
			w := bootIndividual2(t, raidBoot{}, ind2Template(ddMagicID), ind2Template(ddMagicTwoID))
			victim := w.spawn(t, ddMagicID, 1500, 0)
			member := w.spawn(t, ddMagicTwoID, 1500, 200)
			w.srv.InstallNPCCastSeam(t, member, ind2Skills())
			w.ticks(t, row.settle)
			if row.closeBy {
				w.moveCharacter(t, member, -50, 0)
			}
			w.hit(t, victim, 10)
			want := map[modelskill.ID]ind1Cast{}
			if row.cast {
				want[ind2DDMagicSkill] = ind1Cast{w.player.ObjectID(), 1000000}
			}
			w.wantCasts(t, "member after the hit", member, want)
		})
	}
}

// TestWarriorCastDDMagicAggressiveCastsOnSight ports
// WarriorCastDDMagicAggressive's see-creature hook, every draw 0. Settled,
// wandering in its territory, it wants its DD magic, weight 1000000, on the
// character it sees more than 100 away, then attacks it (hate 200); seen
// at 50 it only attacks.
func TestWarriorCastDDMagicAggressiveCastsOnSight(t *testing.T) {
	t.Parallel()
	for _, row := range []struct {
		name string
		dx   int
		cast bool
	}{{"far", -150, true}, {"near", -50, false}} {
		t.Run(row.name, func(t *testing.T) {
			t.Parallel()
			w := bootIndividual2(t, raidBoot{}, ind2Template(ddAggressiveID))
			n := w.spawn(t, ddAggressiveID, 1500, 0)
			w.srv.InstallNPCCastSeam(t, n, ind2Skills())
			w.ticks(t, 9)
			if n.LifeTime() <= 7 || n.AI().CurrentIntention() != actorai.IntentionWander {
				t.Fatalf("setup: life time %d, intention %v, want past 7 and wander", n.LifeTime(), n.AI().CurrentIntention())
			}
			w.moveCharacter(t, n, row.dx, 0)
			w.ticks(t, 1)
			want := map[modelskill.ID]ind1Cast{}
			if row.cast {
				want[ind2DDMagicSkill] = ind1Cast{w.player.ObjectID(), 1000000}
			}
			w.wantCasts(t, "after it saw the character", n, want)
			if got := n.AggroHate(w.player); got != 200 {
				t.Fatalf("hate of the character it sees = %v, want 200", got)
			}
		})
	}
}

// TestAgit02DoomServantAgitStaysAndHoldsItsGround ports
// Agit02DoomServantAgit's created and no-desire hooks: it spawns with flag
// 0, and with no desire left it does nothing, where a
// WarriorCastDDMagicAggressive beside it wanders.
func TestAgit02DoomServantAgitStaysAndHoldsItsGround(t *testing.T) {
	t.Parallel()
	w := bootIndividual2(t, raidBoot{}, ind2Template(servantID), ind2Template(ddAggressiveID))
	servant := w.spawn(t, servantID, 1500, 0)
	roamer := w.spawn(t, ddAggressiveID, -1500, 0)
	if got := ind1Slot(servant, script.SlotFlag); got != 0 {
		t.Fatalf("created flag = %d, want 0", got)
	}
	w.ticks(t, 3)
	if got := roamer.AI().CurrentIntention(); got != actorai.IntentionWander {
		t.Fatalf("setup: the WarriorCastDDMagicAggressive intention = %v, want wander", got)
	}
	if got := servant.AI().CurrentIntention(); got == actorai.IntentionWander {
		t.Fatal("the servant wanders")
	}
	if d, ok := desire(servant, actorai.IntentionWander); ok {
		t.Fatalf("the servant queued a wander desire %+v", d)
	}
}

// TestAgit02DoomServantAgitReturnsHome ports Agit02DoomServantAgit's 60 s
// timer "3001", first fired 1 s after it spawns. Out of its territory, on a
// draw under 1 of 3, it jumps back to its spawn point (the character sees
// TeleportToLocation) and drops its attack desires; on a draw of 1 it
// stays.
func TestAgit02DoomServantAgitReturnsHome(t *testing.T) {
	t.Parallel()
	for _, row := range []struct {
		draw int
		home bool
	}{{0, true}, {1, false}} {
		t.Run(fmt.Sprintf("draw %d", row.draw), func(t *testing.T) {
			t.Parallel()
			w := bootIndividual2(t, raidBoot{}, ind2Template(servantID))
			w.draws.set(4)
			n := w.spawn(t, servantID, 1500, 0)
			home := script.NPCOf(n).SpawnLocation()
			w.teleport(t, n, -1350, 0)
			if script.NPCOf(n).InMyTerritory() {
				t.Fatal("setup: the servant is still in its territory")
			}
			w.onQueue(t, n, func() { script.NPCOf(n).AddAttackDesire(script.PlayerOf(w.player), 100) })
			if _, ok := desire(n, actorai.IntentionAttack); !ok {
				t.Fatal("setup: no attack desire on the character")
			}
			w.draws.set(4, row.draw, 4)
			w.srv.Advance(t, time.Second)
			frames := w.srv.ReadQueued(t, w.srv.Client)
			x, y, _ := n.Position()
			atHome := x == home.X && y == home.Y
			if atHome != row.home {
				t.Fatalf("servant at %d,%d, spawn point %d,%d: at home = %v, want %v", x, y, home.X, home.Y, atHome, row.home)
			}
			_, attacking := desire(n, actorai.IntentionAttack)
			if attacking == row.home {
				t.Fatalf("attack desire queued = %v, want %v", attacking, !row.home)
			}
			if got := len(framesOf(frames, serverpackets.OpcodeTeleportToLocation, n.ObjectID())); (got > 0) != row.home {
				t.Fatalf("TeleportToLocation frames = %d, want them only for a jump home", got)
			}
		})
	}
}

// TestAgit02DoomServantAgitShufflesItsHate ports the hate shuffle of
// Agit02DoomServantAgit's timer "3001": inside its territory it draws no
// 1-in-3 roll, and on a draw under 1 of 5 it shuffles its hate, raising the
// character it hates less than another NPC to that NPC's hate plus 200; on
// a draw of 1 it does not.
func TestAgit02DoomServantAgitShufflesItsHate(t *testing.T) {
	t.Parallel()
	for _, row := range []struct {
		draw int
		want float64
	}{{0, 1200}, {1, 100}} {
		t.Run(fmt.Sprintf("draw %d", row.draw), func(t *testing.T) {
			t.Parallel()
			w := bootIndividual2(t, raidBoot{}, ind2Template(servantID), warriorTemplate(unboundID, nil))
			w.draws.set(4)
			n := w.spawn(t, servantID, 400, 0)
			other := w.spawn(t, unboundID, 400, 200)
			ind2Hate(t, w, n, other, 1000, 100)

			w.draws.set(4, row.draw)
			w.srv.Advance(t, time.Second)
			if got := n.AggroHate(w.player); got != row.want {
				t.Fatalf("hate of the character after the tick = %v, want %v", got, row.want)
			}
		})
	}
}

// ind2Hate gives h hate of other and of the character, on h's queue.
func ind2Hate(t *testing.T, w *ind1World, h, other *npc.Hostile, otherHate, playerHate float64) {
	t.Helper()
	w.onQueue(t, h, func() {
		script.NPCOf(h).AddAttackDesire(script.NPCOf(other), otherHate)
		script.NPCOf(h).AddAttackDesire(script.PlayerOf(w.player), playerHate)
	})
	if got := h.AggroHate(w.player); got != playerHate {
		t.Fatalf("setup: hate of the character = %v, want %v", got, playerHate)
	}
	if got := h.AggroHate(other); got != otherHate {
		t.Fatalf("setup: hate of the other NPC = %v, want %v", got, otherHate)
	}
}

// TestAgit02DoomServantAgitAnswersItsFlag ports Agit02DoomServantAgit's
// clan-attacked hook, every draw 0, which never calls its parent. A
// settled member that shares the hit servant's flag and is not attacking
// wants its DD magic, from where it stands, on an attacker more than 100
// away (weight 1000000), and wants to attack it by the damage: 100 for
// each twentieth of its maximum hit points. A member with another flag
// does neither.
func TestAgit02DoomServantAgitAnswersItsFlag(t *testing.T) {
	t.Parallel()
	for _, row := range []struct {
		name string
		flag int32
	}{{"same flag", 0}, {"other flag", 1}} {
		t.Run(row.name, func(t *testing.T) {
			t.Parallel()
			w := bootIndividual2(t, raidBoot{}, ind2Template(servantID), ind2Template(servantTwoID))
			victim := w.spawn(t, servantID, 1500, 0)
			member := w.spawn(t, servantTwoID, 1500, 200)
			w.srv.InstallNPCCastSeam(t, member, ind2Skills())
			w.onQueue(t, member, func() { script.NPCOf(member).SetInt(script.SlotFlag, row.flag) })
			w.ticks(t, 8)
			w.hit(t, victim, 10)
			hate, cast := member.AggroHate(w.player), ind1Casts(member)[ind2DDMagicSkill]
			if row.flag != 0 {
				if hate != 0 || cast.Weight != 0 {
					t.Fatalf("other flag member: hate %v, cast %+v, want neither", hate, cast)
				}
				return
			}
			if want := 10.0 / float64(member.MaxHP()) / 0.05 * 100; hate != want {
				t.Fatalf("member hate = %v, want %v (10 of its %d hit points)", hate, want, member.MaxHP())
			}
			if cast.Weight != 1000000 || cast.MoveToTarget || cast.FinalTarget == nil || cast.FinalTarget.ObjectID() != w.player.ObjectID() {
				t.Fatalf("member DD magic desire = %+v, want weight 1000000 on the character, held in place", cast)
			}
		})
	}
}

// TestCorePrivateWarriorCastDDMagicCursesOverLevelled ports
// CorePrivateWarriorCastDDMagicAggressive's attacked and see-spell hooks.
// Against the level 20 character, a level 11 private wants its raid curse
// on a hit and its raid mute on the character's spell, weight 1000000; a
// level 12 one wants neither. A hit adds no hate.
func TestCorePrivateWarriorCastDDMagicCursesOverLevelled(t *testing.T) {
	t.Parallel()
	for _, row := range []struct {
		level int
		curse bool
	}{{11, true}, {12, false}} {
		t.Run(fmt.Sprintf("level %d", row.level), func(t *testing.T) {
			t.Parallel()
			tmpl := ind2Template(corePrivateID)
			tmpl.Level, tmpl.CanMove = row.level, false
			w := bootIndividual2(t, raidSpellBoot(t), tmpl)
			n := w.spawn(t, corePrivateID, 400, 0)
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

// TestCorePrivatePerumShufflesItsHate ports CorePrivatePerum's created and
// timer hooks. It arms timer "4001" 90 s plus a draw of up to 240 s after
// it spawns (draw 0: 90 s). On a draw under 66 the tick shuffles its hate,
// raising the character it hates less than another NPC to that NPC's hate
// plus 200; on a draw of 66 it does not. Either way the tick re-arms with
// a fresh draw: 1000 brings the next tick 91 s later.
func TestCorePrivatePerumShufflesItsHate(t *testing.T) {
	t.Parallel()
	for _, row := range []struct {
		draw    int
		shuffle bool
	}{{65, true}, {66, false}} {
		t.Run(fmt.Sprintf("draw %d", row.draw), func(t *testing.T) {
			t.Parallel()
			tmpl := ind2Template(perumID)
			tmpl.CanMove = false
			w := bootIndividual2(t, raidBoot{}, tmpl, warriorTemplate(unboundID, nil))
			n := w.spawn(t, perumID, 400, 0)
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

// TestWarriorCastDDMagicSelfBuffBuffsItself ports
// WarriorCastDDMagicSelfBuff's created and attacked hooks: it wants its
// self buff on itself as it spawns, weight 1000000, and again when the
// character hits it while it neither attacks nor casts and holds no effect
// of the buff's stack. Holding it, or fighting, it does not.
func TestWarriorCastDDMagicSelfBuffBuffsItself(t *testing.T) {
	t.Parallel()
	tmpl := ind2Template(selfBufferID)
	tmpl.CanMove = false
	w := bootIndividual2(t, raidBoot{opts: []gameservertest.Option{gameservertest.WithScriptSkills(ind2SelfBuff()), gameservertest.WithNpcSpawnSkills(ind2Skills())}}, tmpl)
	n := w.spawn(t, selfBufferID, 300, 0)
	self := n.ObjectID()
	w.wantCasts(t, "after it spawned", n, map[modelskill.ID]ind1Cast{ind2SelfBuffSkill: {self, 1000000}})

	w.onQueue(t, n, func() { n.AI().Desires().Clear() })
	w.hit(t, n, 10)
	casts := ind1Casts(n)
	if d, ok := casts[ind2SelfBuffSkill]; !ok || d.Weight != 1000000 || d.FinalTarget == nil || d.FinalTarget.ObjectID() != self {
		t.Fatalf("self buff desire after an idle hit = %+v (%v), want weight 1000000 on itself", d, ok)
	}

	w.ticks(t, 1)
	if !fightingIntention(n) {
		t.Fatalf("setup: intention %v, want attack or cast", n.AI().CurrentIntention())
	}
	w.onQueue(t, n, func() { n.AI().Desires().RemoveKind(actorai.IntentionCast) })
	w.hit(t, n, 10)
	if _, ok := ind1Casts(n)[ind2SelfBuffSkill]; ok {
		t.Fatal("self buff desire after a hit while it fights")
	}

	ind2ApplySelfBuff(t, w, n)
	w.onQueue(t, n, func() { script.NPCOf(n).RemoveAllAttackDesire(); n.AI().Desires().Clear() })
	w.srv.Advance(t, 2*time.Second)
	w.ticks(t, 1)
	if fightingIntention(n) {
		t.Fatalf("setup: intention %v after the desires were dropped, want neither attack nor cast", n.AI().CurrentIntention())
	}
	w.hit(t, n, 10)
	if _, ok := ind1Casts(n)[ind2SelfBuffSkill]; ok {
		t.Fatal("self buff desire after an idle hit while it holds the buff")
	}
}

// fightingIntention reports whether h attacks or casts.
func fightingIntention(h *npc.Hostile) bool {
	i := h.AI().CurrentIntention()
	return i == actorai.IntentionAttack || i == actorai.IntentionCast
}

// ind2ApplySelfBuff lands the self buff's effect on h, and waits for it.
func ind2ApplySelfBuff(t *testing.T, w *ind1World, h *npc.Hostile) {
	t.Helper()
	me := w.player.ObjectID()
	ref := modelskill.Ref{ID: ind2SelfBuffSkill, Level: 1}
	if !w.srv.RunScript(t, me, "Ind2Director", func(s *script.Script, _ *script.Player) {
		s.ApplyEffects(ref, script.NPCOf(h), script.NPCOf(h))
	}) {
		t.Fatal("the self buff landing panicked")
	}
	w.srv.AdvanceUntil(t, "the self buff landed", func() bool {
		var level int32
		w.srv.RunScript(t, me, "Ind2Director", func(s *script.Script, _ *script.Player) {
			level = s.AbnormalLevel(script.NPCOf(h), ref)
		})
		return level > 0
	})
}

// TestWarriorCastDDMagicSelfBuffAggressiveBuffsOnSight ports
// WarriorCastDDMagicSelfBuffAggressive's see-creature hook, every draw 0.
// It binds no NPC of its own, so the scenario binds its hooks to a fixture
// id. Seeing the character while it neither attacks nor casts, it wants
// its self buff on itself, weight 1000000, and attacks the character (hate
// 200). The buff test reads the effect on the creature it sees, not on
// itself: a buff on the NPC does not stop it.
func TestWarriorCastDDMagicSelfBuffAggressiveBuffsOnSight(t *testing.T) {
	t.Parallel()
	tmpl := ind2Template(selfBuffAggroID)
	tmpl.CanMove = false
	w := bootIndividual2(t, raidBoot{opts: []gameservertest.Option{gameservertest.WithScriptSkills(ind2SelfBuff())}}, tmpl)
	n := w.spawn(t, selfBuffAggroID, 1500, 0)
	w.srv.InstallNPCCastSeam(t, n, ind2Skills())
	ind2ApplySelfBuff(t, w, n)
	w.onQueue(t, n, func() { n.AI().Desires().Clear() })

	w.moveCharacter(t, n, -150, 0)
	w.ticks(t, 1)
	if d, ok := ind1Casts(n)[ind2SelfBuffSkill]; !ok || d.Weight != 1000000 || d.FinalTarget == nil || d.FinalTarget.ObjectID() != n.ObjectID() {
		t.Fatalf("self buff desire on sight = %+v (%v), want weight 1000000 on itself", d, ok)
	}
	if got := n.AggroHate(w.player); got != 200 {
		t.Fatalf("hate of the character it sees = %v, want 200", got)
	}
}

// TestWarriorCastingCurseCursesWhatItFights ports WarriorCastingCurse's
// created, attacked and timer hooks, every draw 0: the same 5 s fight
// timer as WarriorCastDDMagic, with its debuff. The first hit arms it; the
// tick wants the debuff on the most hated, weight 1000000; a hit by the
// most hated wants it too and marks the next tick to skip; out of a fight
// the tick clears both slots.
func TestWarriorCastingCurseCursesWhatItFights(t *testing.T) {
	t.Parallel()
	w := bootIndividual2(t, raidBoot{}, ind2Template(curserID))
	n := w.spawn(t, curserID, 300, 0)
	w.srv.InstallNPCCastSeam(t, n, ind2Skills())
	me := w.player.ObjectID()

	w.hit(t, n, 10)
	w.wantCasts(t, "after the first hit", n, nil)
	w.ticks(t, 1)
	w.srv.Advance(t, 5*time.Second)
	w.wantCasts(t, "after the first tick", n, map[modelskill.ID]ind1Cast{ind2DebuffSkill: {me, 1000000}})

	w.onQueue(t, n, func() { n.AI().Desires().Clear() })
	w.hit(t, n, 10)
	w.wantCasts(t, "after a hit by the most hated", n, map[modelskill.ID]ind1Cast{ind2DebuffSkill: {me, 1000000}})
	w.onQueue(t, n, func() { n.AI().Desires().Clear() })
	w.ticks(t, 1)
	w.srv.Advance(t, 5*time.Second)
	w.wantCasts(t, "after the skipped tick", n, nil)

	w.onQueue(t, n, func() { script.NPCOf(n).RemoveAllAttackDesire(); n.AI().Desires().Clear() })
	w.ticks(t, 1)
	w.srv.Advance(t, 5*time.Second)
	if a1, a2 := ind1Slot(n, script.SlotIntAI1), ind1Slot(n, script.SlotIntAI2); a1 != 0 || a2 != 0 {
		t.Fatalf("i_ai1, i_ai2 after the tick out of a fight = %d, %d, want 0, 0", a1, a2)
	}
}

// TestWarriorCastingCurseAnswersItsClan ports WarriorCastingCurse's
// clan-attacked hook, every draw 0: a settled clan member wants its debuff
// on the attacker of another member, weight 1000000, at any distance and
// whatever it is doing; a young member does not.
func TestWarriorCastingCurseAnswersItsClan(t *testing.T) {
	t.Parallel()
	for _, row := range []struct {
		name   string
		settle int
		cast   bool
	}{{"settled", 8, true}, {"young", 0, false}} {
		t.Run(row.name, func(t *testing.T) {
			t.Parallel()
			w := bootIndividual2(t, raidBoot{}, ind2Template(curserID), ind2Template(curserTwoID))
			victim := w.spawn(t, curserID, 1500, 0)
			member := w.spawn(t, curserTwoID, 1500, 200)
			w.srv.InstallNPCCastSeam(t, member, ind2Skills())
			w.ticks(t, row.settle)
			w.moveCharacter(t, member, -50, 0)
			w.hit(t, victim, 10)
			want := map[modelskill.ID]ind1Cast{}
			if row.cast {
				want[ind2DebuffSkill] = ind1Cast{w.player.ObjectID(), 1000000}
			}
			w.wantCasts(t, "member after the hit", member, want)
		})
	}
}
