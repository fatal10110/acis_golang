package ai

import (
	"testing"
	"time"

	actorai "github.com/fatal10110/acis_golang/internal/gameserver/model/actor/ai"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/attackable"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/npc"
	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/script"
	"github.com/fatal10110/acis_golang/internal/gameserver/script/ai/warrior"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
)

// The NPC ids the seventh PR of the first individual behavior batch (#186
// PR 7) spawns, each a fixture monster of level 1.
const (
	magical4RevID = int32(21206) // WarriorCast3SkillsMagical4Revived
	farmID        = int32(16019) // FarmGrowthStep
	farmTwoID     = int32(16021) // FarmGrowthStep
	farmThreeID   = int32(16023) // FarmGrowthStep
	growth1ID     = int32(21451) // WarriorGrowthStep1
	growth2ID     = int32(21452) // WarriorGrowthStep2
	growth2TwoID  = int32(21456) // WarriorGrowthStep2
	growth3ID     = int32(21460) // WarriorGrowthStep3Aggressive
	growth4ID     = int32(21468) // WarriorGrowthStep4Aggressive
)

// The two foods a player feeds the beasts.
const (
	ind1gFood1 = modelskill.ID(2188)
	ind1gFood2 = modelskill.ID(2189)
)

// ind1gBatch are the seventh PR's classes, in scripts.xml order.
var ind1gBatch = []struct {
	path string
	ctor func() script.Script
}{
	{"script.ai.individual.Monster.WarriorBase.Warrior.WarriorCast3SkillsMagical4.WarriorCast3SkillsMagical4Revived", warrior.Cast3SkillsMagical4Revived},
	{"script.ai.individual.Monster.WarriorBase.Warrior.WarriorGrowth.FarmGrowthStep", warrior.FarmGrowthStep},
	{"script.ai.individual.Monster.WarriorBase.Warrior.WarriorGrowth.WarriorGrowthStep1", warrior.WarriorGrowthStep1},
	{"script.ai.individual.Monster.WarriorBase.Warrior.WarriorGrowth.WarriorGrowthStep2", warrior.WarriorGrowthStep2},
	{"script.ai.individual.Monster.WarriorBase.Warrior.WarriorGrowth.WarriorGrowthStep3Aggressive", warrior.WarriorGrowthStep3Aggressive},
	{"script.ai.individual.Monster.WarriorBase.Warrior.WarriorGrowth.WarriorGrowthStep4Aggressive", warrior.WarriorGrowthStep4Aggressive},
}

// bootIndividual1g boots the sixth and seventh PRs' classes after the
// Warrior chain beside templates and an unbound monster, with a director
// script to call privates and send script events through, every NPC
// casting the sixth PR's skills, every draw 0 until a scenario queues
// others.
func bootIndividual1g(t *testing.T, templates ...*npc.Template) *ind1World {
	t.Helper()
	list, catalog := warriorScripts()
	for _, c := range append(ind1fBatch[:len(ind1fBatch):len(ind1fBatch)], ind1gBatch...) {
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
	w := bootRaid(t, raidBoot{opts: []gameservertest.Option{
		gameservertest.WithScriptRand(draws.rand),
		gameservertest.WithNPCScripts(kinds, list, catalog),
		gameservertest.WithNpcSpawnSkills(modelskill.NewTable(ind1fDefs())),
		gameservertest.WithScriptSkills(ind1fDefs()...),
	}}, templates...)
	return &ind1World{warriorWorld: w, draws: draws, heard: &ind1Heard{}}
}

// ind1gTemplate is the sixth PR's fixture monster of id with ai as its AI
// parameters.
func ind1gTemplate(id int32, ai npc.AIParams) *npc.Template {
	tmpl := ind1fTemplate(id)
	tmpl.AIParams = ai
	return tmpl
}

// ind1gFeed lands food on h from feeder, on the feeder's queue, as a
// spice's cast does: a one-target beast feed of 150 aggro points.
func (w *ind1World) ind1gFeed(t *testing.T, h *npc.Hostile, feeder attackable.Combatant, food modelskill.ID) {
	t.Helper()
	spice := ind1Skill(food, 900, false)
	spice.Magic, spice.SkillType, spice.Power, spice.AggroPoints = false, "BEAST_FEED", 0, 150
	feed := func() { h.SkillAttacked(feeder, spice) }
	if other, ok := feeder.(*npc.Hostile); ok {
		w.onQueue(t, other, feed)
		return
	}
	done := make(chan struct{})
	if !w.srv.PlayerQueue(t, feeder.ObjectID()).Post(func() {
		defer close(done)
		feed()
	}) {
		t.Fatal("the feeder's queue is closed")
	}
	<-done
	w.srv.Settle(t)
}

// ind1gGrown returns the one NPC of id in the world, failing unless it
// stands where from stood with the character and food as its spawn
// parameters, and from left the world.
func (w *ind1World) ind1gGrown(t *testing.T, from *npc.Hostile, id int32, food modelskill.ID) *npc.Hostile {
	t.Helper()
	if w.inWorld(from) {
		t.Fatalf("npc %d still in the world after it grew", from.NpcID())
	}
	grown := w.liveNPCs(id)
	if len(grown) != 1 {
		t.Fatalf("grown %d = %d NPCs, want 1", id, len(grown))
	}
	g := grown[0]
	x, y, _ := from.Position()
	if gx, gy, _ := g.Position(); gx != x || gy != y {
		t.Fatalf("grown %d at %d,%d, want %d,%d", id, gx, gy, x, y)
	}
	if p1, p2 := ind1Slot(g, script.SlotParam1), ind1Slot(g, script.SlotParam2); p1 != w.player.ObjectID() || p2 != int32(food) {
		t.Fatalf("grown %d spawn parameters = %d, %d, want the character %d and the food %d", id, p1, p2, w.player.ObjectID(), food)
	}
	return g
}

// ind1gWantEat fails unless h wants its eating social, social 1 for
// MoveAroundSocial frames of 30 a second, weight 200, or already took it
// up.
func ind1gWantEat(t *testing.T, when string, h *npc.Hostile, timer int) {
	t.Helper()
	d, ok := desire(h, actorai.IntentionSocial)
	if !ok && h.AI().CurrentIntention() == actorai.IntentionSocial {
		return
	}
	if !ok || d.ItemObjectID != 1 || d.Timer != timer || d.Weight != 200 {
		t.Fatalf("%s: social desire = %+v (%v), want social 1 for %d, weight 200", when, d, ok, timer)
	}
}

// ind1gWantFeeder fails unless h keeps the character as its feeder (c_ai0)
// with food (i_ai3), marked i_ai4.
func (w *ind1World) ind1gWantFeeder(t *testing.T, when string, h *npc.Hostile, food modelskill.ID, i4 int32) {
	t.Helper()
	c0 := script.NPCOf(h).Slot(script.SlotCreature0)
	if c0 == nil || c0.ObjectID() != w.player.ObjectID() {
		t.Fatalf("%s: c_ai0 = %v, want the character", when, c0)
	}
	if a3, a4 := ind1Slot(h, script.SlotIntAI3), ind1Slot(h, script.SlotIntAI4); a3 != int32(food) || a4 != i4 {
		t.Fatalf("%s: i_ai3, i_ai4 = %d, %d, want %d, %d", when, a3, a4, food, i4)
	}
}

// TestCast3SkillsMagical4RevivedStunsTheNamedCreature ports
// WarriorCast3SkillsMagical4Revived's created hook: called with spawn
// parameters 1000 and the character, it wants its stun (4663) on the
// character and to attack it; called naming no one, neither.
func TestCast3SkillsMagical4RevivedStunsTheNamedCreature(t *testing.T) {
	t.Parallel()
	w := bootIndividual1g(t, ind1fTemplate(magical4RevID), ind1fTemplate(magical4ID))
	master := w.spawn(t, magical4ID, 1500, 0)
	me := w.player.ObjectID()
	named := w.ind4cCallPrivate(t, master, magical4RevID, 1500, 200, script.Params{P1: 1000, P2: me})
	w.wantCasts(t, "called naming the character", named, map[modelskill.ID]ind1Cast{ind1dStunID: {me, 20000}})
	if got := named.AggroHate(w.player); got != 1000 {
		t.Fatalf("hate of the named character = %v, want 1000", got)
	}
	plain := w.ind4cCallPrivate(t, master, magical4RevID, 1500, 400, script.Params{})
	w.wantCasts(t, "called naming no one", plain, nil)
	if got := plain.AggroHate(w.player); got != 0 {
		t.Fatalf("hate of the character with no name = %v, want none", got)
	}
}

// farmAI are the fixture calf's AI parameters: its food 2188, TakeSocial
// 30 (a 1 s growth), MoveAroundSocial 70 and four distinct silhouettes.
var farmAI = npc.AIParams{
	"FeedID1": "2188-1", "TakeSocial": "30", "MoveAroundSocial": "70",
	"silhouette1": "16021", "silhouette2": "16023", "silhouette_ex": "21460", "silhouette_ex2": "21468",
}

// TestFarmGrowthStepGrowsIntoItsSilhouette ports FarmGrowthStep's created,
// attacked and timer hooks, every draw 0 unless a row queues others. It
// appears with i_ai4 0, wanting its self buff. Fed its food it eats
// (social 1 for 70 * 1000 / 30), says its first line (2004), keeps the
// feeder and the food, and 1 s later grows: for a draw over 50 into its
// first or second silhouette, else into the fighter's or the other
// occupations' one. The grown NPC stands where the calf stood with the
// feeder and the food as its spawn parameters, and the calf leaves.
func TestFarmGrowthStepGrowsIntoItsSilhouette(t *testing.T) {
	t.Parallel()
	for _, row := range []struct {
		name  string
		class int
		draws []int
		want  int32
	}{
		{"fighter", 0, nil, growth3ID},
		{"mystic", 10, nil, growth4ID},
		{"first silhouette", 10, []int{51, 0}, farmTwoID},
		{"second silhouette", 0, []int{51, 50}, farmThreeID},
	} {
		t.Run(row.name, func(t *testing.T) {
			t.Parallel()
			w := bootIndividual1g(t,
				ind1gTemplate(farmID, farmAI), ind1fTemplate(farmTwoID), ind1fTemplate(farmThreeID),
				ind1fTemplate(growth3ID), ind1fTemplate(growth4ID),
			)
			w.setClass(t, w.player, row.class)
			farm := w.spawn(t, farmID, 300, 0)
			if got := ind1Slot(farm, script.SlotIntAI4); got != 0 {
				t.Fatalf("created i_ai4 = %d, want 0", got)
			}
			ind5bWantCast(t, "as it appears", farm, ind2SelfBuffSkill, farm.ObjectID())
			w.clearDesires(t, farm)

			w.ind1gFeed(t, farm, w.player, ind1gFood1)
			if got := says(w.srv.ReadQueued(t, w.srv.Client), farm.ObjectID()); len(got) != 1 || got[0] != "What did you just do to me?" {
				t.Fatalf("lines after the feed = %q, want the first", got)
			}
			ind1gWantEat(t, "after the feed", farm, 70*1000/30)
			w.ind1gWantFeeder(t, "after the feed", farm, ind1gFood1, 1)

			w.srv.Advance(t, 900*time.Millisecond)
			if !w.inWorld(farm) {
				t.Fatal("the calf grew before its eating ended")
			}
			w.draws.set(0, row.draws...)
			w.srv.Advance(t, 100*time.Millisecond)
			w.ind1gGrown(t, farm, row.want, ind1gFood1)
		})
	}
}

// TestFarmGrowthStepHatesWhatIsNotFood ports FarmGrowthStep's attacked
// hook for a plain hit, every draw 0: the calf hates the character by the
// damage, then, fighting it, wants its blast and its physical special on
// it; a feed by the character it fights wants the same strikes too.
func TestFarmGrowthStepHatesWhatIsNotFood(t *testing.T) {
	t.Parallel()
	w := bootIndividual1g(t, ind1gTemplate(farmID, farmAI))
	me := w.player.ObjectID()
	farm := w.spawn(t, farmID, 100, 0)
	w.clearDesires(t, farm)
	w.hit(t, farm, 80)
	if got := farm.AggroHate(w.player); got <= 0 {
		t.Fatalf("hate after a plain hit = %v, want some", got)
	}
	w.setTop(t, farm, script.PlayerOf(w.player))
	w.ind1gFeed(t, farm, w.player, ind1gFood1)
	w.wantCasts(t, "fed by the character it fights", farm, map[modelskill.ID]ind1Cast{
		ind1cBlastID: {farm.ObjectID(), 1000000}, ind1cSpecialID: {me, 1000000},
	})
}

// growth1AI are the fixture young beast's AI parameters: its two foods,
// a 1 s growth, and the silhouettes the tests grow it into.
var growth1AI = npc.AIParams{
	"FeedID1": "2188-1", "FeedID2": "2189-1", "TakeSocial": "30", "MoveAroundSocial": "78",
	"FeedID1_warrior_silhouette1": "21452", "FeedID1_warrior_silhouette2": "29990",
	"FeedID1_wizard_silhouette1": "21456", "FeedID1_wizard_silhouette2": "29990",
	"FeedID2_warrior_silhouette1": "21456", "FeedID2_warrior_silhouette2": "29990",
	"FeedID2_wizard_silhouette1": "29990", "FeedID2_wizard_silhouette2": "29990",
}

// TestWarriorGrowthStep1GrowsByItsFood ports WarriorGrowthStep1's created,
// attacked and timer hooks. Fed either food it eats, keeps the feeder and
// the food, and hates no one, its parent left out; 1 s later it grows into
// that food's warrior silhouette for a draw under 50, else into its wizard
// one. The grown beast (WarriorGrowthStep2) keeps the character as its
// feeder and the food, and attacks the character.
func TestWarriorGrowthStep1GrowsByItsFood(t *testing.T) {
	t.Parallel()
	for _, row := range []struct {
		name  string
		food  modelskill.ID
		draws []int
		want  int32
	}{
		{"first food, warrior", ind1gFood1, nil, growth2ID},
		{"first food, wizard", ind1gFood1, []int{50, 0}, growth2TwoID},
		{"second food, warrior", ind1gFood2, nil, growth2TwoID},
	} {
		t.Run(row.name, func(t *testing.T) {
			t.Parallel()
			w := bootIndividual1g(t, ind1gTemplate(growth1ID, growth1AI), ind1fTemplate(growth2ID), ind1fTemplate(growth2TwoID))
			n := w.spawn(t, growth1ID, 300, 0)
			w.clearDesires(t, n)
			w.ind1gFeed(t, n, w.player, row.food)
			ind1gWantEat(t, "after the feed", n, 78*1000/30)
			w.ind1gWantFeeder(t, "after the feed", n, row.food, 1)
			if got := n.AggroHate(w.player); got != 0 {
				t.Fatalf("hate after the feed = %v, want none", got)
			}
			w.draws.set(0, row.draws...)
			w.srv.Advance(t, time.Second)
			g := w.ind1gGrown(t, n, row.want, row.food)
			w.ind1gWantFeeder(t, "the grown beast", g, row.food, 1)
			if got := g.AggroHate(w.player); got <= 0 {
				t.Fatalf("the grown beast's hate of the character = %v, want some", got)
			}
		})
	}
}

// TestWarriorGrowthStep1SpoilsWhenFedByAnother ports WarriorGrowthStep1's
// attacked hook for a second feeder: fed by the character and then by
// another creature, it marks itself spoiled (i_ai4 2) and never grows. A
// plain hit reaches its parent, which hates the character.
func TestWarriorGrowthStep1SpoilsWhenFedByAnother(t *testing.T) {
	t.Parallel()
	w := bootIndividual1g(t, ind1gTemplate(growth1ID, growth1AI), ind1fTemplate(growth2ID))
	n := w.spawn(t, growth1ID, 300, 0)
	other := w.spawn(t, unboundID, 300, 100)
	w.ind1gFeed(t, n, w.player, ind1gFood1)
	w.ind1gFeed(t, n, other, ind1gFood1)
	if got := ind1Slot(n, script.SlotIntAI4); got != 2 {
		t.Fatalf("i_ai4 after another feeder = %d, want 2", got)
	}
	w.srv.Advance(t, time.Second)
	if !w.inWorld(n) || len(w.liveNPCs(growth2ID)) != 0 {
		t.Fatal("a spoiled beast grew")
	}
	w.hit(t, n, 80)
	if got := n.AggroHate(w.player); got <= 0 {
		t.Fatalf("hate after a plain hit = %v, want some", got)
	}
}

// growth2AI are the fixture beast grown once's AI parameters: a 1 s
// growth, its two silhouettes and its last one.
var growth2AI = npc.AIParams{
	"TakeSocial": "30", "MoveAroundSocial": "78",
	"silhouette1": "21460", "silhouette2": "29990", "silhouette_ex2": "21468",
}

// TestWarriorGrowthStep2GrowsLikelierWithEachFeed ports WarriorGrowthStep2's
// created, attacked and timer hooks. Called with the character as its
// feeder and food 2188, it keeps both and attacks the character. Fed by
// the character, it eats; with the draw 34, its first feed (33%) does not
// grow it and says nothing, its second (66%) does: 1 s later it grows into
// its first silhouette (WarriorGrowthStep3Aggressive). The other food is no
// feed: fighting the character, it wants its physical special on it.
func TestWarriorGrowthStep2GrowsLikelierWithEachFeed(t *testing.T) {
	t.Parallel()
	w := bootIndividual1g(t, ind1gTemplate(growth2ID, growth2AI), ind1fTemplate(growth3ID))
	me := w.player.ObjectID()
	master := w.spawn(t, unboundID, 1500, 0)
	n := w.ind4cCallPrivate(t, master, growth2ID, 300, 0, script.Params{P1: me, P2: int32(ind1gFood1)})
	w.ind1gWantFeeder(t, "as it appears", n, ind1gFood1, 1)
	if got := n.AggroHate(w.player); got <= 0 {
		t.Fatalf("hate of its feeder as it appears = %v, want some", got)
	}

	w.setTop(t, n, script.PlayerOf(w.player))
	w.ind1gFeed(t, n, w.player, ind1gFood2)
	w.wantCasts(t, "the other food", n, map[modelskill.ID]ind1Cast{ind1cSpecialID: {me, 1000000}})

	w.clearDesires(t, n)
	w.draws.set(34)
	w.ind1gFeed(t, n, w.player, ind1gFood1)
	ind1gWantEat(t, "the first feed", n, 78*1000/30)
	if got := says(w.srv.ReadQueued(t, w.srv.Client), n.ObjectID()); len(got) != 0 {
		t.Fatalf("lines after the first feed = %q, want none", got)
	}
	if a2, a4 := ind1Slot(n, script.SlotIntAI2), ind1Slot(n, script.SlotIntAI4); a2 != 1 || a4 != 1 {
		t.Fatalf("i_ai2, i_ai4 after the first feed = %d, %d, want 1, 1", a2, a4)
	}
	w.ind1gFeed(t, n, w.player, ind1gFood1)
	if a2, a4 := ind1Slot(n, script.SlotIntAI2), ind1Slot(n, script.SlotIntAI4); a2 != 2 || a4 != 3 {
		t.Fatalf("i_ai2, i_ai4 after the second feed = %d, %d, want 2, 3", a2, a4)
	}
	w.draws.set(0)
	w.srv.Advance(t, time.Second)
	w.ind1gGrown(t, n, growth3ID, ind1gFood1)
}

// TestWarriorGrowthStep2SaysItsLine ports WarriorGrowthStep2's line: fed
// with every draw 0 it says its first (2014).
func TestWarriorGrowthStep2SaysItsLine(t *testing.T) {
	t.Parallel()
	w := bootIndividual1g(t, ind1gTemplate(growth2ID, growth2AI))
	master := w.spawn(t, unboundID, 1500, 0)
	n := w.ind4cCallPrivate(t, master, growth2ID, 300, 0, script.Params{P1: w.player.ObjectID(), P2: int32(ind1gFood1)})
	w.srv.ReadQueued(t, w.srv.Client)
	w.ind1gFeed(t, n, w.player, ind1gFood1)
	if got := says(w.srv.ReadQueued(t, w.srv.Client), n.ObjectID()); len(got) != 1 || got[0] != "Don't keep trying to tame me.  I don't want to be tamed." {
		t.Fatalf("lines after the feed = %q, want its first", got)
	}
}

// TestWarriorGrowthStep4MakesItsNeighboursGrow ports WarriorGrowthStep4's
// created hook and WarriorGrowthStep2's script-event hook, every draw 0. A
// fully grown beast appearing tells the beasts within 700 (event 10018):
// one grown once with a feeder grows into its last silhouette, one with no
// feeder stays as it is. The new beast keeps the character as its feeder
// and attacks it.
func TestWarriorGrowthStep4MakesItsNeighboursGrow(t *testing.T) {
	t.Parallel()
	w := bootIndividual1g(t, ind1gTemplate(growth2ID, npc.AIParams{"silhouette_ex2": "16021"}), ind1fTemplate(growth4ID), ind1fTemplate(farmTwoID))
	me := w.player.ObjectID()
	master := w.spawn(t, unboundID, 1500, 0)
	fed := w.ind4cCallPrivate(t, master, growth2ID, 300, 0, script.Params{P1: me, P2: int32(ind1gFood1)})
	unfed := w.ind4cCallPrivate(t, master, growth2ID, 300, 300, script.Params{})
	grown := w.ind4cCallPrivate(t, master, growth4ID, 300, 150, script.Params{P1: me, P2: int32(ind1gFood1)})
	w.ind1gGrown(t, fed, farmTwoID, ind1gFood1)
	if !w.inWorld(unfed) {
		t.Fatal("the beast with no feeder grew")
	}
	w.ind1gWantFeeder(t, "the fully grown beast", grown, ind1gFood1, 1)
	if got := grown.AggroHate(w.player); got <= 0 {
		t.Fatalf("the fully grown beast's hate of the character = %v, want some", got)
	}
}

// TestWarriorGrowthStep4BlastsWhatItFights ports WarriorGrowthStep4's
// attacked and timer hooks, every draw 0. Its first hit starts its 5 s
// timer (i_ai1 1); fighting the character, the tick wants its DD magic on
// it, weight 1000000. A hit by its top desire target marks i_ai2, so the
// next tick wants nothing and clears the mark.
func TestWarriorGrowthStep4BlastsWhatItFights(t *testing.T) {
	t.Parallel()
	w := bootIndividual1g(t, ind1fTemplate(growth4ID))
	me := w.player.ObjectID()
	n := w.spawn(t, growth4ID, 300, 0)
	w.setTop(t, n, script.PlayerOf(w.player))
	w.hit(t, n, 10)
	if a1, a2 := ind1Slot(n, script.SlotIntAI1), ind1Slot(n, script.SlotIntAI2); a1 != 1 || a2 != 0 {
		t.Fatalf("i_ai1, i_ai2 after the first hit = %d, %d, want 1, 0", a1, a2)
	}
	w.srv.Advance(t, 5*time.Second)
	ind5bWantCast(t, "the tick", n, ind1eDDID, me)

	w.setTop(t, n, script.PlayerOf(w.player))
	w.hit(t, n, 10)
	if got := ind1Slot(n, script.SlotIntAI2); got != 1 {
		t.Fatalf("i_ai2 after a hit by its top desire target = %d, want 1", got)
	}
	w.srv.Advance(t, 5*time.Second)
	ind5bNoCast(t, "the tick after a hit", n, ind1eDDID)
	if got := ind1Slot(n, script.SlotIntAI2); got != 0 {
		t.Fatalf("i_ai2 after the tick = %d, want 0", got)
	}
}

// TestWarriorGrowthStep3GrowsByItsFeedersOccupation ports
// WarriorGrowthStep3Aggressive's attacked and timer hooks, every draw 0.
// Called with the character as its feeder, fed by it, it says its first
// line (2019) and, for a draw within 20% of one feed, grows 1 s later: for
// a draw of 50 or less into the silhouette of the feeder's occupation, the
// fighter's here.
func TestWarriorGrowthStep3GrowsByItsFeedersOccupation(t *testing.T) {
	t.Parallel()
	ai := npc.AIParams{
		"TakeSocial": "30", "MoveAroundSocial": "72",
		"silhouette1": "29990", "silhouette2": "29990", "silhouette_ex": "16021", "silhouette_ex2": "16023",
	}
	w := bootIndividual1g(t, ind1gTemplate(growth3ID, ai), ind1fTemplate(farmTwoID), ind1fTemplate(farmThreeID))
	w.setClass(t, w.player, 0)
	master := w.spawn(t, unboundID, 1500, 0)
	n := w.ind4cCallPrivate(t, master, growth3ID, 300, 0, script.Params{P1: w.player.ObjectID(), P2: int32(ind1gFood1)})
	w.srv.ReadQueued(t, w.srv.Client)
	w.ind1gFeed(t, n, w.player, ind1gFood1)
	if got := says(w.srv.ReadQueued(t, w.srv.Client), n.ObjectID()); len(got) != 1 || got[0] != "Grrrrr...." {
		t.Fatalf("lines after the feed = %q, want its first", got)
	}
	ind1gWantEat(t, "after the feed", n, 72*1000/30)
	w.ind1gWantFeeder(t, "after the feed", n, ind1gFood1, 3)
	w.srv.Advance(t, time.Second)
	w.ind1gGrown(t, n, farmTwoID, ind1gFood1)
}
