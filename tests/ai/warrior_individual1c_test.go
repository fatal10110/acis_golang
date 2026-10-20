package ai

import (
	"fmt"
	"slices"
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

// The NPC ids the third PR of the first individual behavior batch (#186
// PR 3) spawns, each a fixture monster of level 1 unless a scenario says
// otherwise.
const (
	coreKnightID   = int32(29007) // CorePrivateWarriorAggressive
	medusaID       = int32(20158) // Medusa
	golemID        = int32(22050) // WarriorRunAwayPhysicalSpecialGolemAggressive
	timerBombID    = int32(18340) // WarriorTimerBomb
	archerID       = int32(20011) // WarriorUseBow
	steadyArcherID = int32(22132) // WarriorUseBowNotFleeAggressive
)

// The skills the batch's fixture NPCs list by type or cast by id.
const (
	ind1cSpecialID   = modelskill.ID(9461) // PHYSICAL_SPECIAL
	ind1cLongRangeID = modelskill.ID(9462) // LONG_RANGE_PHYSICAL_SPECIAL
	ind1cBlastID     = modelskill.ID(9463) // SELF_RANGE_DD_MAGIC
	ind1cHealID      = modelskill.ID(9464) // MAGIC_HEAL
	medusaPoisonID   = modelskill.ID(4320) // Medusa's fatal poison, level 3
	coreMuteID       = modelskill.ID(4215) // the raid mute
)

// ind1cBatch are the third PR's classes, in scripts.xml order.
var ind1cBatch = []struct {
	path string
	ctor func() script.Script
}{
	{"script.ai.individual.Monster.WarriorBase.Warrior.WarriorAggressive.CorePrivateWarriorAggressive", warrior.CorePrivateWarriorAggressive},
	{"script.ai.individual.Monster.WarriorBase.Warrior.WarriorAggressive.Medusa", warrior.Medusa},
	{"script.ai.individual.Monster.WarriorBase.Warrior.WarriorRunAwayPhysicalSpecialGolemAggressive", warrior.RunAwayPhysicalSpecialGolemAggressive},
	{"script.ai.individual.Monster.WarriorBase.Warrior.WarriorTimerBomb", warrior.TimerBomb},
	{"script.ai.individual.Monster.WarriorBase.Warrior.WarriorUseBow", warrior.UseBow},
	{"script.ai.individual.Monster.WarriorBase.Warrior.WarriorUseBowNotFleeAggressive", warrior.UseBowNotFleeAggressive},
}

// bootIndividual1c boots the third PR's classes after the Warrior chain
// beside templates, with extra, every NPC casting the batch's skills,
// every draw 0 until a scenario queues others.
func bootIndividual1c(t *testing.T, extra raidBoot, templates ...*npc.Template) *ind1World {
	t.Helper()
	list, catalog := warriorScripts()
	for _, c := range ind1cBatch {
		list = append(list, script.Listing{Path: c.path})
		catalog[c.path] = c.ctor
	}
	kinds := map[int32]script.NPCKind{}
	for _, tmpl := range templates {
		kinds[int32(tmpl.ID)] = script.KindHostile
	}
	draws := &ind1Draws{}
	opts := append([]gameservertest.Option{
		gameservertest.WithScriptRand(draws.rand),
		gameservertest.WithNPCScripts(kinds, list, catalog),
		gameservertest.WithNpcSpawnSkills(ind1cSkills()),
	}, extra.opts...)
	w := bootRaid(t, raidBoot{opts: opts, ready: extra.ready}, templates...)
	return &ind1World{warriorWorld: w, draws: draws, heard: &ind1Heard{}}
}

// ind1cTemplate is a fixture monster of id listing every skill of the
// batch by its type, in the clan the scenario's NPCs share.
func ind1cTemplate(id int32) *npc.Template {
	tmpl := warriorTemplate(id, nil)
	tmpl.SkillsByType = map[npc.SkillType]modelskill.Ref{
		npc.SkillTypePhysicalSpecial:          {ID: ind1cSpecialID, Level: 1},
		npc.SkillTypeLongRangePhysicalSpecial: {ID: ind1cLongRangeID, Level: 1},
		npc.SkillTypeSelfRangeDDMagic:         {ID: ind1cBlastID, Level: 1},
		npc.SkillTypeMagicHeal:                {ID: ind1cHealID, Level: 1},
	}
	tmpl.Clans, tmpl.ClanRange = []string{"ind1c"}, 600
	return tmpl
}

// ind1cSkills are the skills the batch's fixture NPCs cast: the strikes
// one-target attacks of reach 900, the blast and the heal self skills, the
// heal also on another, and Medusa's poison.
func ind1cSkills() *modelskill.Table {
	heal := ind1Skill(ind1cHealID, 600, true)
	heal.Target = modelskill.TargetOne
	poison := ind1Skill(medusaPoisonID, 600, false)
	poison.Level = 3
	return modelskill.NewTable([]modelskill.Definition{
		ind1Skill(ind1cSpecialID, 900, false), ind1Skill(ind1cLongRangeID, 900, false),
		ind1Skill(ind1cBlastID, 0, true), heal, poison,
	})
}

// TestCorePrivateWarriorAggressiveCursesAndMutesOverLevelled ports
// CorePrivateWarriorAggressive's attacked and see-spell hooks. Against the
// level 20 character, a level 11 Death Knight (more than 8 levels below)
// wants its raid curse on the character when hit and its raid mute when it
// sees the character cast, both with weight 1000000; a level 12 one wants
// neither. Its attacked hook never calls its parent: a hit adds no hate.
func TestCorePrivateWarriorAggressiveCursesAndMutesOverLevelled(t *testing.T) {
	t.Parallel()
	for _, row := range []struct {
		level int
		curse bool
	}{{11, true}, {12, false}} {
		t.Run(fmt.Sprintf("level %d", row.level), func(t *testing.T) {
			t.Parallel()
			tmpl := warriorTemplate(coreKnightID, nil)
			tmpl.Level, tmpl.CanMove = row.level, false
			w := bootIndividual1c(t, raidSpellBoot(t), tmpl)
			n := w.spawn(t, coreKnightID, 400, 0)
			w.srv.InstallNPCCastSeam(t, n, modelskill.NewTable([]modelskill.Definition{raidCurseDefinition(), ind1Skill(coreMuteID, 600, false)}))
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
					_, ok := ind1Casts(n)[coreMuteID]
					return ok
				})
				want[coreMuteID] = ind1Cast{me, 1000000}
			} else {
				w.srv.Advance(t, 2*time.Second)
			}
			w.wantCasts(t, "after the spell", n, want)
		})
	}
}

// TestCorePrivateWarriorAggressiveShufflesItsHate ports
// CorePrivateWarriorAggressive's created and timer hooks: 90 s after it
// appears (a draw of 0 over the 240 s spread) timer "3001" shuffles its
// hate on a draw under 66, here moving the other NPC's 1000 onto the
// character's 100, and re-arms either way.
func TestCorePrivateWarriorAggressiveShufflesItsHate(t *testing.T) {
	t.Parallel()
	for _, row := range []struct {
		draw    int
		shuffle bool
	}{{65, true}, {66, false}} {
		t.Run(fmt.Sprintf("draw %d", row.draw), func(t *testing.T) {
			t.Parallel()
			tmpl := ind1cTemplate(coreKnightID)
			tmpl.CanMove = false
			w := bootIndividual1c(t, raidBoot{}, tmpl, warriorTemplate(unboundID, nil))
			n := w.spawn(t, coreKnightID, 400, 0)
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

// TestMedusaPoisonsHerMostHatedBelowAFifth ports Medusa's attacked hook.
// Hit by the creature she hates most while below a fifth of her HP, on a
// draw under 3, she calls out (NpcString 1000452) and wants her fatal
// poison, level 3, on it, weight 1000000. On a draw of 3, at a quarter of
// her HP, or hit by another than her most hated, she does neither. Her
// parent's attacked hook runs either way: the hit adds its Warrior hate.
func TestMedusaPoisonsHerMostHatedBelowAFifth(t *testing.T) {
	t.Parallel()
	const line = "It's a good day to die! Welcome to hell, maggot!"
	for _, row := range []struct {
		name      string
		draw      int
		hpShare   float64
		otherMost bool
		poison    bool
	}{
		{"poisons", 2, 0.1, false, true},
		{"draw 3", 3, 0.1, false, false},
		{"a quarter of her HP", 0, 0.25, false, false},
		{"another most hated", 0, 0.1, true, false},
	} {
		t.Run(row.name, func(t *testing.T) {
			t.Parallel()
			tmpl := ind1cTemplate(medusaID)
			tmpl.CanMove = false
			w := bootIndividual1c(t, raidBoot{}, tmpl, warriorTemplate(unboundID, nil))
			n := w.spawn(t, medusaID, 400, 0)
			other := w.spawn(t, unboundID, 400, 200)
			otherHate := 50.0
			if row.otherMost {
				otherHate = 1000
			}
			ind2Hate(t, w, n, other, otherHate, 100)
			w.clear(t, n)
			w.onQueue(t, n, func() { n.SetHP(n.MaxHPValue() * row.hpShare) })
			w.srv.ReadQueued(t, w.srv.Client)

			w.draws.set(0, row.draw)
			w.hit(t, n, 10)
			frames := w.srv.ReadQueued(t, w.srv.Client)
			want := map[modelskill.ID]ind1Cast{}
			var lines []string
			if row.poison {
				want[medusaPoisonID] = ind1Cast{w.player.ObjectID(), 1000000}
				lines = []string{line}
				if d := ind1Casts(n)[medusaPoisonID]; d.Skill.Level != 3 {
					t.Errorf("poison level = %d, want 3", d.Skill.Level)
				}
			}
			w.wantCasts(t, "after the hit", n, want)
			if got := says(frames, n.ObjectID()); !slices.Equal(got, lines) {
				t.Errorf("Medusa says %q, want %q", got, lines)
			}
			if got := n.AggroHate(w.player); got != 100+125 {
				t.Errorf("hate of the character after the hit = %v, want 225", got)
			}
		})
	}
}

// TestRunAwayGolemStrikesFromAfar ports the golem's see-creature, attacked
// and timer "1002" hooks, every draw 0. Seeing the character 300 away it
// turns on it (200) and wants its long range strike on it; its parents'
// see-creature hooks do not run. A hit from there makes it want its
// physical special (a draw under 33) and its long range strike on the
// attacker, and adds the Warrior hate; 5 s after the sighting it wants the
// long range strike on that attacker again, farther than 200. A hit from
// 150 wants no long range strike.
func TestRunAwayGolemStrikesFromAfar(t *testing.T) {
	t.Parallel()
	tmpl := ind1cTemplate(golemID)
	tmpl.CanMove = false
	w := bootIndividual1c(t, raidBoot{}, tmpl)
	n := w.spawn(t, golemID, 1500, 0)
	me := w.player.ObjectID()
	w.moveCharacter(t, n, 300, 0)
	w.srv.Settle(t)
	w.ticks(t, 1)
	if got := n.AggroHate(w.player); got != 200 {
		t.Fatalf("hate of the character it sees = %v, want 200", got)
	}
	w.wantCasts(t, "after it saw the character", n, map[modelskill.ID]ind1Cast{ind1cLongRangeID: {me, 1000000}})

	w.clear(t, n)
	w.hit(t, n, 10)
	w.wantCasts(t, "after a hit from 300", n, map[modelskill.ID]ind1Cast{ind1cSpecialID: {me, 1000000}, ind1cLongRangeID: {me, 1000000}})
	if got := n.AggroHate(w.player); got != 200+125 {
		t.Fatalf("hate after the hit = %v, want 325", got)
	}

	w.clear(t, n)
	w.srv.AdvanceUntil(t, "the 5 s long range strike", func() bool {
		_, ok := ind1Casts(n)[ind1cLongRangeID]
		return ok
	})
	w.wantCasts(t, "after timer 1002", n, map[modelskill.ID]ind1Cast{ind1cLongRangeID: {me, 1000000}})

	w.clear(t, n)
	w.moveCharacter(t, n, 150, 0)
	w.srv.Settle(t)
	w.hit(t, n, 10)
	w.wantCasts(t, "after a hit from 150", n, map[modelskill.ID]ind1Cast{ind1cSpecialID: {me, 1000000}})
}

// TestRunAwayGolemRunsFromACloseAttacker ports the golem's created,
// attacked and timer "1003" hooks with a drift range of 250. A first hit
// from closer than 100 sets i_ai1 and, 2 s later, makes it want to run 250
// from that attacker, weight 1000, clearing i_ai1 again. A hit from 150
// sets nothing.
func TestRunAwayGolemRunsFromACloseAttacker(t *testing.T) {
	t.Parallel()
	for _, row := range []struct {
		name string
		dist int
		runs bool
	}{{"close", 50, true}, {"150 away", 150, false}} {
		t.Run(row.name, func(t *testing.T) {
			t.Parallel()
			w := bootIndividual1c(t, raidBoot{opts: []gameservertest.Option{gameservertest.WithMaxDriftRange(250)}}, ind1cTemplate(golemID))
			n := w.spawn(t, golemID, 1500, 0)
			if got := ind1Slot(n, script.SlotIntAI1); got != 0 {
				t.Fatalf("i_ai1 after it appeared = %d, want 0", got)
			}
			w.moveCharacter(t, n, row.dist, 0)
			w.srv.Settle(t)
			w.hit(t, n, 10)
			want := int32(0)
			if row.runs {
				want = 1
			}
			if got := ind1Slot(n, script.SlotIntAI1); got != want {
				t.Fatalf("i_ai1 after the hit = %d, want %d", got, want)
			}
			w.srv.Advance(t, 2*time.Second)
			w.srv.Settle(t)
			d, ok := desire(n, actorai.IntentionFlee)
			if !row.runs {
				if ok {
					t.Fatalf("flee desire after a hit from %d = %+v, want none", row.dist, d)
				}
				return
			}
			if !ok || d.Distance != 250 || d.Weight != 1000 || d.FinalTarget == nil || d.FinalTarget.ObjectID() != w.player.ObjectID() {
				t.Fatalf("flee desire = %+v (%v), want a 250 run from the character, weight 1000", d, ok)
			}
			if got := ind1Slot(n, script.SlotIntAI1); got != 0 {
				t.Fatalf("i_ai1 after the run = %d, want 0", got)
			}
		})
	}
}

// moveTypes returns the move types (0 walk, 1 run) the ChangeMoveType
// frames of the object id show, in order.
func moveTypes(frames [][]byte, objectID int32) []int32 {
	var out []int32
	for _, f := range framesOf(frames, serverpackets.OpcodeChangeMoveType, objectID) {
		out = append(out, int32(f[5])|int32(f[6])<<8|int32(f[7])<<16|int32(f[8])<<24)
	}
	return out
}

// TestTimerBombSwitchesItsGaitThenBlows ports WarriorTimerBomb's created,
// timer and skill-finished hooks, every draw 0. It does not run its
// parent's created hook. Timer "1010" fires 1 s after it appears (a draw
// of 0, plus 1) and makes it walk, then every 3 s (a draw of 0, raised to
// 3) it runs and walks in turn. Timer "1009" fires 10 s after it appears
// (a draw of 0, raised to 10) and makes it want its self range blast on
// itself; the blast's landing kills it.
func TestTimerBombSwitchesItsGaitThenBlows(t *testing.T) {
	t.Parallel()
	w := bootIndividual1c(t, raidBoot{}, ind1cTemplate(timerBombID))
	n := w.spawn(t, timerBombID, 300, 0)
	id := n.ObjectID()

	w.srv.Advance(t, time.Second)
	frames := w.srv.ReadQueued(t, w.srv.Client)
	if got := moveTypes(frames, id); !slices.Equal(got, []int32{0}) {
		t.Fatalf("move types after 1 s = %v, want [0] (walk)", got)
	}
	if got := ind1Slot(n, script.SlotIntAI0); got != 1 {
		t.Fatalf("i_ai0 while it walks = %d, want 1", got)
	}
	w.srv.Advance(t, 3*time.Second)
	frames = w.srv.ReadQueued(t, w.srv.Client)
	if got := moveTypes(frames, id); !slices.Equal(got, []int32{1}) {
		t.Fatalf("move types after 4 s = %v, want [1] (run)", got)
	}
	if got := ind1Slot(n, script.SlotIntAI0); got != 0 {
		t.Fatalf("i_ai0 while it runs = %d, want 0", got)
	}
	if _, ok := ind1Casts(n)[ind1cBlastID]; ok {
		t.Fatal("the bomb wants its blast before 10 s")
	}

	w.srv.Advance(t, 6*time.Second)
	w.srv.Settle(t)
	ind5bWantCast(t, "after timer 1009", n, ind1cBlastID, id)
	frames = ind5bRunUntil(t, w, "the bomb's death", n.Dead)
	if got := castsOf(frames, n, ind1cBlastID); len(got) != 1 || got[0] != id {
		t.Fatalf("blasts on itself = %v, want one", got)
	}
	if len(framesOf(frames, serverpackets.OpcodeDie, id)) != 1 {
		t.Fatal("the bomb did not die after its blast")
	}
}

// TestTimerBombBlastsOnACloseHit ports WarriorTimerBomb's attacked hook:
// a hit from closer than 200 makes it want its blast on itself, weight
// 1000000; one from 300 does not. Its parent's attacked hook never runs:
// no hit adds hate.
func TestTimerBombBlastsOnACloseHit(t *testing.T) {
	t.Parallel()
	w := bootIndividual1c(t, raidBoot{}, ind1cTemplate(timerBombID))
	far := w.spawn(t, timerBombID, 300, 0)
	near := w.spawn(t, timerBombID, 0, 150)

	w.hit(t, far, 10)
	ind5bNoCast(t, "a hit from 300 away", far, ind1cBlastID)
	w.hit(t, near, 10)
	ind5bWantCast(t, "a hit from 150 away", near, ind1cBlastID, near.ObjectID())
	for _, h := range []*npc.Hostile{far, near} {
		if got := h.AggroHate(w.player); got != 0 {
			t.Errorf("bomb %d hate after the hit = %v, want none", h.ObjectID(), got)
		}
	}
}

// TestUseBowRunsFromACloseAttacker ports WarriorUseBow's created,
// attacked and timer hooks with a drift range of 250. A first hit from
// closer than 100 sets i_ai2 and, 2 s later (timer "100002"), makes it
// want to run 250 from that attacker, weight 100000, clearing i_ai2. A hit
// from 150 sets nothing. Its parent's attacked hook runs either way: the
// hit adds its Warrior hate.
func TestUseBowRunsFromACloseAttacker(t *testing.T) {
	t.Parallel()
	for _, row := range []struct {
		name string
		dist int
		runs bool
	}{{"close", 50, true}, {"150 away", 150, false}} {
		t.Run(row.name, func(t *testing.T) {
			t.Parallel()
			w := bootIndividual1c(t, raidBoot{opts: []gameservertest.Option{gameservertest.WithMaxDriftRange(250)}}, ind1cTemplate(archerID))
			n := w.spawn(t, archerID, 1500, 0)
			if got := ind1Slot(n, script.SlotIntAI2); got != 0 {
				t.Fatalf("i_ai2 after it appeared = %d, want 0", got)
			}
			w.moveCharacter(t, n, row.dist, 0)
			w.srv.Settle(t)
			w.hit(t, n, 10)
			if got := n.AggroHate(w.player); got != 125 {
				t.Fatalf("hate after the hit = %v, want 125", got)
			}
			want := int32(0)
			if row.runs {
				want = 1
			}
			if got := ind1Slot(n, script.SlotIntAI2); got != want {
				t.Fatalf("i_ai2 after the hit = %d, want %d", got, want)
			}
			w.srv.Advance(t, 2*time.Second)
			w.srv.Settle(t)
			d, ok := desire(n, actorai.IntentionFlee)
			if !row.runs {
				if ok {
					t.Fatalf("flee desire after a hit from %d = %+v, want none", row.dist, d)
				}
				return
			}
			if !ok || d.Distance != 250 || d.Weight != 100000 || d.FinalTarget == nil || d.FinalTarget.ObjectID() != w.player.ObjectID() {
				t.Fatalf("flee desire = %+v (%v), want a 250 run from the character, weight 100000", d, ok)
			}
			if got := ind1Slot(n, script.SlotIntAI2); got != 0 {
				t.Fatalf("i_ai2 after the run = %d, want 0", got)
			}
		})
	}
}

// TestUseBowNotFleeHealsAndShoots ports WarriorUseBowNotFleeAggressive's
// attacked hook. Below a third of its HP it wants its heal on itself,
// weight 1000000, and its own clan-attacked hook, which a hit runs on the
// NPC hit as on its clan, adds that weight once more; with the attacker its
// top desire target and a draw under 33 it wants its long range strike on
// it, weight 1000000. At 0.4 of its HP, on a draw of 33, or with another
// top desire target, it wants neither.
func TestUseBowNotFleeHealsAndShoots(t *testing.T) {
	t.Parallel()
	for _, row := range []struct {
		name     string
		hpShare  float64
		draw     int
		otherTop bool
		heal     bool
		shoot    bool
	}{
		{"low and fighting the attacker", 0.3, 32, false, true, true},
		{"draw 33", 0.4, 33, false, false, false},
		{"another top desire target", 0.4, 0, true, false, false},
		{"fighting the attacker", 0.4, 0, false, false, true},
	} {
		t.Run(row.name, func(t *testing.T) {
			t.Parallel()
			tmpl := ind1cTemplate(steadyArcherID)
			tmpl.CanMove = false
			w := bootIndividual1c(t, raidBoot{}, tmpl, warriorTemplate(unboundID, nil))
			n := w.spawn(t, steadyArcherID, 1500, 0)
			other := w.spawn(t, unboundID, 1500, 300)
			w.moveCharacter(t, n, 300, 0)
			w.srv.Settle(t)
			top := script.Creature(script.PlayerOf(w.player))
			if row.otherTop {
				top = script.NPCOf(other)
			}
			w.setTop(t, n, top)
			w.onQueue(t, n, func() { n.SetHP(n.MaxHPValue() * row.hpShare) })
			w.draws.set(0, row.draw)
			w.hit(t, n, 10)
			want := map[modelskill.ID]ind1Cast{}
			if row.heal {
				want[ind1cHealID] = ind1Cast{n.ObjectID(), 2000000}
			}
			if row.shoot {
				want[ind1cLongRangeID] = ind1Cast{w.player.ObjectID(), 1000000}
			}
			w.wantCasts(t, "after the hit", n, want)
		})
	}
}

// TestUseBowNotFleeHealsItsClan ports WarriorUseBowNotFleeAggressive's
// clan-attacked hook: a clan member hit below a third of its HP gets the
// archer's heal, weight 1000000; one at 0.4 of its HP does not.
func TestUseBowNotFleeHealsItsClan(t *testing.T) {
	t.Parallel()
	for _, row := range []struct {
		hpShare float64
		heal    bool
	}{{0.3, true}, {0.4, false}} {
		t.Run(fmt.Sprintf("hp %v", row.hpShare), func(t *testing.T) {
			t.Parallel()
			archer := ind1cTemplate(steadyArcherID)
			archer.CanMove = false
			victimTmpl := ind1cTemplate(unboundID)
			victimTmpl.CanMove = false
			w := bootIndividual1c(t, raidBoot{}, archer, victimTmpl)
			victim := w.spawn(t, unboundID, 1500, 0)
			member := w.spawn(t, steadyArcherID, 1500, 150)
			w.moveCharacter(t, victim, 300, 0)
			w.srv.Settle(t)
			w.clear(t, member)
			w.onQueue(t, victim, func() { victim.SetHP(victim.MaxHPValue() * row.hpShare) })
			w.hit(t, victim, 10)
			casts := ind1Casts(member)
			d, ok := casts[ind1cHealID]
			if row.heal && (!ok || d.Weight != 1000000 || d.FinalTarget == nil || d.FinalTarget.ObjectID() != victim.ObjectID()) {
				t.Fatalf("heal desire of the member = %+v (%v), want weight 1000000 on the victim", d, ok)
			}
			if !row.heal && ok {
				t.Fatalf("heal desire of the member = %+v, want none", d)
			}
		})
	}
}
