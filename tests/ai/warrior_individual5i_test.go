package ai

import (
	"fmt"
	"slices"
	"testing"
	"time"

	actorai "github.com/fatal10110/acis_golang/internal/gameserver/model/actor/ai"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/npc"
	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/script"
	"github.com/fatal10110/acis_golang/internal/gameserver/script/ai/warrior"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
)

// The NPC ids of the fifth individual behavior batch's ninth PR the tests
// spawn.
const (
	ind5iToClanID     = int32(20058) // WarriorRunAwayToClan
	ind5iToClanAggrID = int32(20211) // WarriorRunAwayToClanAggressive
	ind5iSlowID       = int32(29975) // WarriorSlowType, bound by the test
	ind5iSlow1ID      = int32(20101) // WarriorSlowType1
	ind5iSlow1AggrID  = int32(21385) // WarriorSlowType1Aggressive
	ind5iSlow2ID      = int32(20201) // WarriorSlowType2
	ind5iSlow2AggrID  = int32(20996) // WarriorSlowType2Aggressive
	ind5iRunnerAggrID = int32(29976) // WarriorRunAwayPhysicalSpecialAggressive, bound by the test
)

// ind5iSlowMagic is the slowing magic the batch's fixture NPCs list.
const ind5iSlowMagic = modelskill.ID(9590) // DD_MAGIC_SLOW

// ind5iBatch are the PR's classes, in scripts.xml order, after the runner
// one derives from. The two binding no NPC of their own are bound to
// fixture ids.
var ind5iBatch = []struct {
	path string
	ctor func() script.Script
}{
	{"script.ai.individual.Monster.WarriorBase.Warrior.WarriorRunAwayPhysicalSpecial.WarriorRunAwayPhysicalSpecial", warrior.RunAwayPhysicalSpecial},
	{"script.ai.individual.Monster.WarriorBase.Warrior.WarriorRunAwayPhysicalSpecial.WarriorRunAwayPhysicalSpecialAggressive", func() script.Script {
		s := warrior.RunAwayPhysicalSpecialAggressive()
		s.NPCs = []int32{ind5iRunnerAggrID}
		return s
	}},
	{"script.ai.individual.Monster.WarriorBase.Warrior.WarriorRunAwayToClan.WarriorRunAwayToClan", warrior.RunAwayToClan},
	{"script.ai.individual.Monster.WarriorBase.Warrior.WarriorRunAwayToClan.WarriorRunAwayToClanAggressive", warrior.RunAwayToClanAggressive},
	{"script.ai.individual.Monster.WarriorBase.Warrior.WarriorSlowType.WarriorSlowType", func() script.Script {
		s := warrior.SlowType()
		s.NPCs = []int32{ind5iSlowID}
		return s
	}},
	{"script.ai.individual.Monster.WarriorBase.Warrior.WarriorSlowType.WarriorSlowType1", warrior.SlowType1},
	{"script.ai.individual.Monster.WarriorBase.Warrior.WarriorSlowType.WarriorSlowType1Aggressive", warrior.SlowType1Aggressive},
	{"script.ai.individual.Monster.WarriorBase.Warrior.WarriorSlowType.WarriorSlowType2", warrior.SlowType2},
	{"script.ai.individual.Monster.WarriorBase.Warrior.WarriorSlowType.WarriorSlowType2Aggressive", warrior.SlowType2Aggressive},
}

// ind5iDefs are the batch's skills: the slowing magic, of reach 2000.
func ind5iDefs() []modelskill.Definition {
	return []modelskill.Definition{ind1Skill(ind5iSlowMagic, 2000, false)}
}

// ind5iTemplate is a fixture monster of id listing the slowing magic.
func ind5iTemplate(id int32) *npc.Template {
	return ind5dTemplate(id, nil, map[npc.SkillType]modelskill.ID{npc.SkillTypeDDMagicSlow: ind5iSlowMagic})
}

// ind5iStill is ind5iTemplate for an NPC that does not move.
func ind5iStill(id int32) *npc.Template {
	tmpl := ind5iTemplate(id)
	tmpl.CanMove = false
	return tmpl
}

// ind5iCalm makes h stop fighting: it stops hating the creature it hates
// most and wants only to wander, and the AI runs, a quarter second apart,
// until its swing ends and it neither attacks nor casts.
func (w *ind1World) ind5iCalm(t *testing.T, h *npc.Hostile) {
	t.Helper()
	w.onQueue(t, h, func() {
		sn := script.NPCOf(h)
		if c := sn.MostHated(); c != nil {
			sn.StopHate(c)
		}
		sn.RemoveAllDesire()
		sn.AddWanderDesire(5, 1000)
	})
	for i := 0; ; i++ {
		if in := script.NPCOf(h).Intention(); in != script.IntentionAttack && in != script.IntentionCast {
			return
		}
		if i == 8 {
			t.Fatalf("it never calmed down: %v %+v", script.NPCOf(h).Intention(), h.AI().Desires().Snapshot())
		}
		w.srv.Advance(t, 250*time.Millisecond)
		cycle(t, w.srv)
	}
}

// bootIndividual5i boots the PR's classes after the Warrior chain beside
// templates and an unbound decoy monster that records the script events it
// hears, with a director script to send events through, every NPC casting
// the slowing magic, every draw 0 until a test queues others.
func bootIndividual5i(t *testing.T, extra raidBoot, templates ...*npc.Template) *ind1World {
	t.Helper()
	list, catalog := warriorScripts()
	for _, c := range ind5iBatch {
		list = append(list, script.Listing{Path: c.path})
		catalog[c.path] = c.ctor
	}
	heard := &ind1Heard{}
	const listener, director = "test.Ind1Listener", "test.Ind1Director"
	catalog[listener] = func() script.Script {
		return script.Script{Behavior: true, NPCs: []int32{ind1ListenerID}, Hooks: script.Hooks{
			OnScriptEvent: func(_ *script.Script, e script.ScriptEvent) {
				heard.mu.Lock()
				defer heard.mu.Unlock()
				heard.events = append(heard.events, fmt.Sprintf("event=%d arg1=%d arg2=%d", e.EventID, e.Arg1, e.Arg2))
			},
		}}
	}
	catalog[director] = func() script.Script { return script.Script{} }
	list = append(list, script.Listing{Path: listener}, script.Listing{Path: director})
	templates = append(templates, warriorTemplate(unboundID, nil))
	kinds := map[int32]script.NPCKind{}
	for _, tmpl := range templates {
		kinds[int32(tmpl.ID)] = script.KindHostile
	}
	draws := &ind1Draws{}
	opts := append([]gameservertest.Option{
		gameservertest.WithScriptRand(draws.rand),
		gameservertest.WithNPCScripts(kinds, list, catalog),
		gameservertest.WithNpcSpawnSkills(modelskill.NewTable(ind5iDefs())),
		gameservertest.WithScriptSkills(ind5iDefs()...),
	}, extra.opts...)
	w := bootRaid(t, raidBoot{opts: opts, ready: extra.ready}, templates...)
	return &ind1World{warriorWorld: w, draws: draws, heard: heard}
}

// ind5iRunnerDen is an npcmaker spawning one runner to its clan whose spawn
// names a flee point 200 east of it.
const ind5iRunnerDen = `<?xml version="1.0" encoding="utf-8"?>
<list>
	<territory name="clan_den" minZ="-1000" maxZ="1000"><node x="0" y="0"/><node x="400" y="0"/><node x="400" y="400"/><node x="0" y="400"/></territory>
	<npcmaker name="clan_den" territory="clan_den" maximumNpcs="1">
		<ai type="default_maker"/>
		<npc id="20058" total="1" pos="40;20;30;0" respawn="60sec"><ai><set name="flee_x" val="240"/><set name="flee_y" val="20"/><set name="flee_z" val="30"/></ai></npc>
	</npcmaker>
</list>`

// ind5iDenRunner boots the den with the listener 100 from the flee point
// and returns the runner.
func ind5iDenRunner(t *testing.T) (*ind1World, *npc.Hostile) {
	t.Helper()
	w := bootIndividual5i(t, raidBoot{opts: []gameservertest.Option{gameservertest.WithNpcSpawns(ind3bSpawnTable(t, ind5iRunnerDen))}}, ind5iTemplate(ind5iToClanID))
	live := w.liveNPCs(ind5iToClanID)
	if len(live) != 1 {
		t.Fatalf("setup: %d runners in the world, want 1", len(live))
	}
	if _, err := w.srv.NpcSpawns.AddSpawn(ind1ListenerID, 240, 120, 30, 0, false, 0); err != nil {
		t.Fatalf("spawn the listener: %v", err)
	}
	w.srv.Settle(t)
	w.srv.ReadQueued(t, w.srv.Client)
	return w, live[0]
}

// ind5iSetCharacterHP sets the character's HP to ratio of its maximum, on
// its queue.
func (w *ind1World) ind5iSetCharacterHP(t *testing.T, ratio float64) {
	t.Helper()
	pc, ok := w.player.(interface {
		SetHP(float64)
		MaxHPValue() float64
	})
	if !ok {
		t.Fatalf("setup: the character %T cannot set its HP", w.player)
	}
	done := make(chan struct{})
	if !w.srv.PlayerQueue(t, w.player.ObjectID()).Post(func() {
		defer close(done)
		pc.SetHP(pc.MaxHPValue() * ratio)
	}) {
		t.Fatal("the character's queue is closed")
	}
	<-done
	w.srv.Settle(t)
}

// ind5iFlee checks the runner lets its first hit pass and does not run at
// 60% HP, on a draw of 10, at exactly 33% HP or for a character at
// exactly 25% HP, then, at 45% HP with its desires dropped, a
// hit on a draw of 0 makes it shout the line of draw line, keep the
// character in c_ai0 and run to its flee point, weight 100000000.
func (w *ind1World) ind5iFlee(t *testing.T, h *npc.Hostile, line int) {
	t.Helper()
	if a0, a4 := ind1Slot(h, script.SlotIntAI0), ind1Slot(h, script.SlotIntAI4); a0 != 0 || a4 != 0 {
		t.Fatalf("created i_ai0, i_ai4 = %d, %d, want 0, 0", a0, a4)
	}
	w.ind5hSetHP(t, h, 0.45)
	w.draws.set(0)
	w.hit(t, h, 1)
	if a0, a4 := ind1Slot(h, script.SlotIntAI0), ind1Slot(h, script.SlotIntAI4); a0 != 0 || a4 != 1 {
		t.Fatalf("i_ai0, i_ai4 after the first hit = %d, %d, want 0, 1", a0, a4)
	}
	w.ind5hSetHP(t, h, 0.6)
	w.hit(t, h, 1)
	if got := ind1Slot(h, script.SlotIntAI0); got != 0 {
		t.Fatalf("i_ai0 after a hit at 60%% HP = %d, want 0", got)
	}
	w.ind5hSetHP(t, h, 0.45)
	w.draws.set(99, 10)
	w.hit(t, h, 1)
	if got := ind1Slot(h, script.SlotIntAI0); got != 0 {
		t.Fatalf("i_ai0 after a hit on a draw of 10 = %d, want 0", got)
	}
	// Its hooks see its HP before the hit lands.
	w.ind5hSetHP(t, h, 0.33)
	if r := script.NPCOf(h).HPRatio(); r != 0.33 {
		t.Fatalf("setup: runner HP ratio = %v, want 0.33", r)
	}
	w.draws.set(99, 0)
	w.hit(t, h, 1)
	if got := ind1Slot(h, script.SlotIntAI0); got != 0 {
		t.Fatalf("i_ai0 after a hit at 33%% HP = %d, want 0", got)
	}
	w.ind5iSetCharacterHP(t, 0.25)
	w.ind5hSetHP(t, h, 0.45)
	w.draws.set(99, 0)
	w.hit(t, h, 1)
	if got := ind1Slot(h, script.SlotIntAI0); got != 0 {
		t.Fatalf("i_ai0 after a hit from a character at 25%% HP = %d, want 0", got)
	}
	w.ind5iSetCharacterHP(t, 1)
	w.ind5hSetHP(t, h, 0.45)
	w.ind5dClear(t, h)
	w.srv.ReadQueued(t, w.srv.Client)
	w.draws.set(99, 0, line)
	w.hit(t, h, 1)
	if got := ind1Slot(h, script.SlotIntAI0); got != 1 {
		t.Fatalf("i_ai0 after a hit at 45%% HP = %d, want 1 (running)", got)
	}
	if c0 := script.NPCOf(h).Slot(script.SlotCreature0); c0 == nil || c0.ObjectID() != w.player.ObjectID() {
		t.Fatalf("runner c_ai0 = %v, want the character it fled", c0)
	}
	frames := w.srv.ReadQueued(t, w.srv.Client)
	if got, want := says(frames, h.ObjectID()), []string{script.NpcString(ind5iLine(line))}; !slices.Equal(got, want) {
		t.Fatalf("runner lines = %q, want %q", got, want)
	}
	if d, ok := desire(h, actorai.IntentionMoveTo); !ok || d.Weight != 100000000 || d.Location.X != 240 || d.Location.Y != 20 || d.Location.Z != 30 {
		t.Fatalf("running move desire = %+v (%v), want 240,20,30 with weight 100000000", d, ok)
	}
}

// ind5iLine is the NpcString the runner shouts on draw i5, written out the
// long way.
func ind5iLine(i5 int) int32 {
	switch {
	case i5 < 77:
		return 1000007 + int32(i5/7)
	case i5 < 95:
		return 1000018 + int32((i5-77)/2)
	}
	return 1000027
}

// ind5iArrive walks the running runner to its flee point and checks it
// wanders there and calls its kin within 400 to the character.
func (w *ind1World) ind5iArrive(t *testing.T, h *npc.Hostile) {
	t.Helper()
	w.heard.take()
	w.ind5hWalk(t, h)
	if got := ind1Slot(h, script.SlotIntAI0); got != 2 {
		t.Fatalf("runner i_ai0 at its flee point = %d, want 2", got)
	}
	if d, ok := desire(h, actorai.IntentionWander); !ok || d.Weight != 50 || d.Timer != 5 {
		t.Fatalf("runner at its flee point wander desire = %+v (%v), want timer 5 and weight 50", d, ok)
	}
	if got, want := w.heard.take(), []string{fmt.Sprintf("event=10000 arg1=%d arg2=0", w.player.ObjectID())}; !slices.Equal(got, want) {
		t.Fatalf("events heard at the flee point = %q, want %q", got, want)
	}
}

// TestRunAwayToClanRunsCallsItsKinAndHeadsHome ports WarriorRunAwayToClan's
// created, attacked, move-finished and timer hooks. Its first hit only
// marks i_ai4; later, between a third and half its HP, a hit on a draw
// under 10 makes it shout a flee line and run. At the flee point it
// wanders and broadcasts script event 10000 with the character within 400.
// 15 s later, below 70% HP and not attacking, it drops its wander and
// walks home, weight 1000000; there it wanders, weight 1000, and the run
// ends.
func TestRunAwayToClanRunsCallsItsKinAndHeadsHome(t *testing.T) {
	t.Parallel()
	w, h := ind5iDenRunner(t)
	home := script.NPCOf(h).SpawnLocation()
	w.ind5iFlee(t, h, 77)
	w.ind5iArrive(t, h)

	w.srv.Advance(t, 14900*time.Millisecond)
	if got := ind1Slot(h, script.SlotIntAI0); got != 2 {
		t.Fatalf("runner i_ai0 before its timer = %d, want 2", got)
	}
	w.srv.Advance(t, 100*time.Millisecond)
	if got := ind1Slot(h, script.SlotIntAI0); got != 3 {
		t.Fatalf("runner i_ai0 after its timer = %d, want 3 (on its way home)", got)
	}
	if d, ok := desire(h, actorai.IntentionWander); ok {
		t.Fatalf("runner still wants to wander: %+v", d)
	}
	if d, ok := desire(h, actorai.IntentionMoveTo); !ok || d.Weight != 1000000 || d.Location.X != home.X || d.Location.Y != home.Y {
		t.Fatalf("runner move desire after its timer = %+v (%v), want its spawn point %d,%d with weight 1000000", d, ok, home.X, home.Y)
	}
	if ind5gAttackWeight(h, w.player.ObjectID()) == 0 {
		t.Fatal("the timer dropped the runner's attack desire")
	}

	w.ind5hWalk(t, h)
	if got := ind1Slot(h, script.SlotIntAI0); got != 0 {
		t.Fatalf("runner i_ai0 back home = %d, want 0", got)
	}
	if d, ok := desire(h, actorai.IntentionWander); !ok || d.Weight != 1000 || d.Timer != 5 {
		t.Fatalf("runner back home wander desire = %+v (%v), want timer 5 and weight 1000", d, ok)
	}
}

// TestRunAwayToClanHealedGivesUpTheRun runs the 2001 timer of a runner at
// 70% HP or more: it drops every desire, wants its spawn point, weight 50,
// and the run ends at once. Its shout follows the draw: a draw of 95 or
// more shouts the last line.
func TestRunAwayToClanHealedGivesUpTheRun(t *testing.T) {
	t.Parallel()
	w, h := ind5iDenRunner(t)
	home := script.NPCOf(h).SpawnLocation()
	w.ind5iFlee(t, h, 95)
	w.ind5iArrive(t, h)
	w.ind5hSetHP(t, h, 0.75)
	w.srv.Advance(t, 15*time.Second)
	if got := ind1Slot(h, script.SlotIntAI0); got != 0 {
		t.Fatalf("healed runner i_ai0 after its timer = %d, want 0", got)
	}
	got := h.AI().Desires().Snapshot()
	if len(got) != 1 || got[0].Kind != actorai.IntentionMoveTo || got[0].Weight != 50 || got[0].Location.X != home.X || got[0].Location.Y != home.Y {
		t.Fatalf("healed runner desires = %+v, want only its spawn point %d,%d with weight 50", got, home.X, home.Y)
	}
}

// ind5iDetour drops h's desires and walks it to a point off its course,
// once the AI turns from its fight to the walk.
func (w *ind1World) ind5iDetour(t *testing.T, h *npc.Hostile) {
	t.Helper()
	w.ind5dClear(t, h)
	w.onQueue(t, h, func() { script.NPCOf(h).AddMoveToDesire(script.Loc{X: 140, Y: 220, Z: 30}, 200000000) })
	for i := 0; h.AI().CurrentIntention() != actorai.IntentionMoveTo; i++ {
		if i == 8 {
			t.Fatalf("the detour never started: %v", h.AI().CurrentIntention())
		}
		cycle(t, w.srv)
	}
	w.ind5hWalk(t, h)
}

// TestRunAwayToClanWalksOnAfterADetour ports WarriorRunAwayToClan's
// move-finished hook for walks that end elsewhere. Running, a walk ending
// off its flee point wants the flee point again, weight 100000000, and
// keeps it running, with no call to its kin; it then runs on and arrives.
// On its way home, a walk ending off its spawn point keeps it on its way
// home with no wander, weight 1000; a walk home then ends the run.
func TestRunAwayToClanWalksOnAfterADetour(t *testing.T) {
	t.Parallel()
	w, h := ind5iDenRunner(t)
	home := script.NPCOf(h).SpawnLocation()
	w.ind5iFlee(t, h, 0)
	w.heard.take()

	w.ind5iDetour(t, h)
	if got := ind1Slot(h, script.SlotIntAI0); got != 1 {
		t.Fatalf("runner i_ai0 after a detour = %d, want 1 (running)", got)
	}
	if d, ok := desire(h, actorai.IntentionMoveTo); !ok || d.Weight != 100000000 || d.Location.X != 240 || d.Location.Y != 20 || d.Location.Z != 30 {
		t.Fatalf("running move desire after a detour = %+v (%v), want 240,20,30 with weight 100000000", d, ok)
	}
	if got := w.heard.take(); len(got) != 0 {
		t.Fatalf("events heard after a detour = %q, want none", got)
	}
	w.ind5iArrive(t, h)

	w.srv.Advance(t, 15*time.Second)
	if got := ind1Slot(h, script.SlotIntAI0); got != 3 {
		t.Fatalf("runner i_ai0 after its timer = %d, want 3 (on its way home)", got)
	}
	w.ind5iDetour(t, h)
	if got := ind1Slot(h, script.SlotIntAI0); got != 3 {
		t.Fatalf("runner i_ai0 after a detour home = %d, want 3 kept", got)
	}
	if d, ok := desire(h, actorai.IntentionWander); ok && d.Weight == 1000 {
		t.Fatalf("runner off its spawn point wander desire = %+v, want none of weight 1000", d)
	}
	w.onQueue(t, h, func() { script.NPCOf(h).AddMoveToDesire(home, 1000000) })
	w.ind5hWalk(t, h)
	if got := ind1Slot(h, script.SlotIntAI0); got != 0 {
		t.Fatalf("runner i_ai0 back home = %d, want 0", got)
	}
}

// TestRunAwayToClanTimerWaitsOutAFight runs the 2001 timer of a runner at
// its flee point that attacks: it changes nothing, so the run neither ends
// nor heads home.
func TestRunAwayToClanTimerWaitsOutAFight(t *testing.T) {
	t.Parallel()
	w, h := ind5iDenRunner(t)
	w.ind5iFlee(t, h, 0)
	w.ind5iArrive(t, h)
	w.srv.Advance(t, 14900*time.Millisecond)
	w.onQueue(t, h, func() {
		sn := script.NPCOf(h)
		sn.AddAttackDesire(sn.Slot(script.SlotCreature0), 1000000)
	})
	for i := 0; script.NPCOf(h).Intention() != script.IntentionAttack; i++ {
		if i == 5 {
			t.Fatal("the runner never turned to attack")
		}
		cycle(t, w.srv)
	}
	w.srv.Advance(t, 100*time.Millisecond)
	if in := script.NPCOf(h).Intention(); in != script.IntentionAttack {
		t.Fatalf("setup: runner intention when its timer fired = %v, want attack", in)
	}
	if got := ind1Slot(h, script.SlotIntAI0); got != 2 {
		t.Fatalf("runner i_ai0 after its timer while attacking = %d, want 2 kept", got)
	}
	if d, ok := desire(h, actorai.IntentionMoveTo); ok {
		t.Fatalf("runner move desire after its timer while attacking = %+v, want none", d)
	}
	if d, ok := desire(h, actorai.IntentionWander); !ok || d.Weight != 50 {
		t.Fatalf("runner wander desire after its timer while attacking = %+v (%v), want its weight 50 kept", d, ok)
	}
}

// TestRunAwayToClanAnswersItsKin ports WarriorRunAwayToClan's script-event
// hook and its empty see-creature hook on a spawn naming no flee point.
// Told of event 10000 while alive and not attacking, it wants to attack
// the creature named, weight 1000000, and clears i_ai3, even when the
// event names nobody; another event, one while it attacks, or one while
// dead changes nothing. Its hits never make it run, and it does not turn
// on the character it sees, while the aggressive one does, weight 200.
func TestRunAwayToClanAnswersItsKin(t *testing.T) {
	t.Parallel()
	w := bootIndividual5i(t, raidBoot{}, ind5iTemplate(ind5iToClanID), ind5iStill(ind5iToClanAggrID))
	me := w.player.ObjectID()
	n := w.spawn(t, ind5iToClanID, 0, 300)
	setAI3 := func(h *npc.Hostile) { w.onQueue(t, h, func() { script.NPCOf(h).SetInt(script.SlotIntAI3, 5) }) }

	setAI3(n)
	w.sendEvent(t, n, 10001, me, 0)
	if got := ind1Slot(n, script.SlotIntAI3); got != 5 {
		t.Fatalf("i_ai3 after event 10001 = %d, want 5 kept", got)
	}
	w.sendEvent(t, n, 10000, 0, 0)
	if got := ind1Slot(n, script.SlotIntAI3); got != 0 {
		t.Fatalf("i_ai3 after event 10000 naming nobody = %d, want 0", got)
	}
	if d, ok := desire(n, actorai.IntentionAttack); ok {
		t.Fatalf("attack desire after events naming nobody or not 10000 = %+v, want none", d)
	}
	setAI3(n)
	w.sendEvent(t, n, 10000, me, 0)
	wantDesire(t, "after event 10000", n, actorai.IntentionAttack, me, 1000000)
	if got := ind1Slot(n, script.SlotIntAI3); got != 0 {
		t.Fatalf("i_ai3 after event 10000 = %d, want 0", got)
	}
	for i := 0; script.NPCOf(n).Intention() != script.IntentionAttack; i++ {
		if i == 5 {
			t.Fatal("the runner never turned to attack")
		}
		cycle(t, w.srv)
	}
	setAI3(n)
	w.sendEvent(t, n, 10000, me, 0)
	if got := ind1Slot(n, script.SlotIntAI3); got != 5 {
		t.Fatalf("i_ai3 after event 10000 while attacking = %d, want 5 kept", got)
	}

	w.draws.set(0)
	for range 3 {
		w.hit(t, n, 1)
	}
	if got := ind1Slot(n, script.SlotIntAI0); got != 0 {
		t.Fatalf("i_ai0 after hits with no flee point = %d, want 0", got)
	}
	w.hit(t, n, int(n.MaxHP()))
	if !n.Dead() {
		t.Fatal("the runner survived the killing hit")
	}
	w.sendEvent(t, n, 10000, me, 0)
	if got := ind1Slot(n, script.SlotIntAI3); got != 5 {
		t.Fatalf("i_ai3 after event 10000 while dead = %d, want 5 kept", got)
	}

	idle := w.spawn(t, ind5iToClanID, 1500, 0)
	aggr := w.spawn(t, ind5iToClanAggrID, 0, 1500)
	decoy := w.spawn(t, unboundID, 100, 1500)
	w.ticks(t, 2)
	if got := ind5gAttackWeight(aggr, decoy.ObjectID()); got != 0 {
		t.Fatalf("aggressive runner attack desire on the decoy = %v, want none", got)
	}
	for _, row := range []struct {
		name    string
		h       *npc.Hostile
		attacks bool
	}{{"runner", idle, false}, {"aggressive runner", aggr, true}} {
		w.moveCharacter(t, row.h, -150, 0)
		w.ticks(t, 2)
		if got := ind5gAttackWeight(row.h, me); (got != 0) != row.attacks {
			t.Fatalf("%s attack desire on the character it sees = %v, want one %v", row.name, got, row.attacks)
		}
	}
}

// TestRunAwayPhysicalSpecialAggressiveAttacksOnlyPlayables ports
// WarriorRunAwayPhysicalSpecialAggressive's see-creature hook, which calls
// no parent: it wants nothing for the decoy monster it sees and attacks
// the character it sees, weight 200.
func TestRunAwayPhysicalSpecialAggressiveAttacksOnlyPlayables(t *testing.T) {
	t.Parallel()
	w := bootIndividual5i(t, raidBoot{}, ind5iStill(ind5iRunnerAggrID))
	n := w.spawn(t, ind5iRunnerAggrID, 1500, 0)
	decoy := w.spawn(t, unboundID, 1600, 0)
	w.ticks(t, 2)
	if got := ind5gAttackWeight(n, decoy.ObjectID()); got != 0 {
		t.Fatalf("attack desire on the decoy = %v, want none", got)
	}
	w.moveCharacter(t, n, -200, 0)
	w.ticks(t, 2)
	if got := ind5gAttackWeight(n, w.player.ObjectID()); got == 0 {
		t.Fatal("seeing the character wants no attack on it")
	}
}

// TestSlowTypeSlowsWhoItHatesMost ports WarriorSlowType's created,
// attacked and timer hooks. Its first hit starts the 5 s 2001 timer
// (i_ai0 set); a later hit by the creature it hates most sets i_ai1. While
// it attacks, a check with i_ai1 set casts nothing and clears it; the next
// one, on a draw under 50, wants its slowing magic on the character,
// weight 1000000. Once it neither attacks nor casts, a check clears both.
func TestSlowTypeSlowsWhoItHatesMost(t *testing.T) {
	t.Parallel()
	w := bootIndividual5i(t, raidBoot{}, ind5iTemplate(ind5iSlowID))
	me := w.player.ObjectID()
	n := w.spawn(t, ind5iSlowID, 100, 0)
	if a0, a1 := ind1Slot(n, script.SlotIntAI0), ind1Slot(n, script.SlotIntAI1); a0 != 0 || a1 != 0 {
		t.Fatalf("created i_ai0, i_ai1 = %d, %d, want 0, 0", a0, a1)
	}
	w.draws.set(0)
	w.hit(t, n, 1)
	if a0, a1 := ind1Slot(n, script.SlotIntAI0), ind1Slot(n, script.SlotIntAI1); a0 != 1 || a1 != 0 {
		t.Fatalf("i_ai0, i_ai1 after the first hit = %d, %d, want 1, 0", a0, a1)
	}
	w.hit(t, n, 1)
	if got := ind1Slot(n, script.SlotIntAI1); got != 1 {
		t.Fatalf("i_ai1 after a hit by its most hated = %d, want 1", got)
	}
	for i := 0; script.NPCOf(n).Intention() != script.IntentionAttack; i++ {
		if i == 5 {
			t.Fatal("it never turned to attack")
		}
		cycle(t, w.srv)
	}
	w.srv.Advance(t, 5*time.Second)
	ind5bNoCast(t, "a check after a hit by its most hated", n, ind5iSlowMagic)
	if got := ind1Slot(n, script.SlotIntAI1); got != 0 {
		t.Fatalf("i_ai1 after a check = %d, want 0", got)
	}
	w.draws.set(50)
	w.srv.Advance(t, 5*time.Second)
	ind5bNoCast(t, "a check on a draw of 50", n, ind5iSlowMagic)
	w.draws.set(49)
	w.srv.Advance(t, 5*time.Second)
	ind5dWant(t, w, "a check on a draw of 49", n, map[modelskill.ID]int32{ind5iSlowMagic: me})

	w.ind5iCalm(t, n)
	w.onQueue(t, n, func() { script.NPCOf(n).SetInt(script.SlotIntAI1, 1) })
	w.srv.Advance(t, 5*time.Second)
	if a0, a1 := ind1Slot(n, script.SlotIntAI0), ind1Slot(n, script.SlotIntAI1); a0 != 0 || a1 != 0 {
		t.Fatalf("i_ai0, i_ai1 after a check while idle = %d, %d, want 0, 0", a0, a1)
	}
}

// TestSlowTypeAnswersItsClan ports WarriorSlowType's clan-attacked hook:
// told of the character's hit on a clan member, it wants its slowing magic
// on the character, weight 1000000, only past its first 7 AI cycles and
// on a draw under 10.
func TestSlowTypeAnswersItsClan(t *testing.T) {
	t.Parallel()
	w := bootIndividual5i(t, raidBoot{}, ind5dClan(ind5iStill(ind5iSlowID)), ind5dClan(warriorTemplate(warriorID, nil)))
	me := w.player.ObjectID()
	victim := w.spawn(t, warriorID, 100, 300)
	n := w.spawn(t, ind5iSlowID, 0, 300)
	w.draws.set(0)
	w.hit(t, victim, 1)
	ind5bNoCast(t, "a clan hit in its first cycles", n, ind5iSlowMagic)
	for i := 0; script.NPCOf(n).LifeTime() <= 7; i++ {
		if i == 20 {
			t.Fatal("its life time never passed 7")
		}
		cycle(t, w.srv)
	}
	if script.NPCOf(n).Intention() == script.IntentionAttack {
		t.Fatal("setup: it attacks")
	}
	w.draws.set(10)
	w.hit(t, victim, 1)
	ind5bNoCast(t, "a clan hit on a draw of 10", n, ind5iSlowMagic)
	w.ind5iCalm(t, n)
	w.draws.set(9)
	w.hit(t, victim, 1)
	ind5dWant(t, w, "a clan hit on a draw of 9", n, map[modelskill.ID]int32{ind5iSlowMagic: me})
}

// TestSlowType1SwitchesAndSlows ports WarriorSlowType1's attacked hook. A
// hit from within 200 wants its slowing magic on the creature it hates
// most on a draw under 10; from 200, on a draw of 10, or on its first hit,
// before it hates anyone, it wants none. Hit
// from within 100 by the character while it attacks a decoy it hates most
// farther than 100, a draw under 80 drops its attack desires and turns it
// on the character: damage / (level + 7) times 30, with the Warrior
// chain's damage / (level + 7) times 100 on top.
func TestSlowType1SwitchesAndSlows(t *testing.T) {
	t.Parallel()
	w := bootIndividual5i(t, raidBoot{}, ind5iTemplate(ind5iSlow1ID))
	me := w.player.ObjectID()
	n := w.spawn(t, ind5iSlow1ID, 199, 0)
	far := w.spawn(t, ind5iSlow1ID, 0, 200)
	w.draws.set(99, 9)
	w.hit(t, n, 1)
	ind5bNoCast(t, "a first hit, hating no one yet, on a draw of 9", n, ind5iSlowMagic)
	w.draws.set(99, 9)
	w.hit(t, n, 1)
	ind5dWant(t, w, "a hit from 199 on a draw of 9", n, map[modelskill.ID]int32{ind5iSlowMagic: me})
	w.ind5dClear(t, n)
	w.draws.set(99, 10)
	w.hit(t, n, 1)
	ind5bNoCast(t, "a hit from 199 on a draw of 10", n, ind5iSlowMagic)
	w.draws.set(0)
	w.hit(t, far, 1)
	w.hit(t, far, 1)
	ind5bNoCast(t, "a hit from 200", far, ind5iSlowMagic)

	near := w.spawn(t, ind5iSlow1ID, 50, 400)
	decoy := w.spawn(t, unboundID, 0, 1500)
	w.onQueue(t, near, func() { script.NPCOf(near).AddAttackDesire(script.NPCOf(decoy), 100000) })
	for i := 0; script.NPCOf(near).Intention() != script.IntentionAttack; i++ {
		if i == 5 {
			t.Fatal("it never turned to attack the decoy")
		}
		cycle(t, w.srv)
	}
	w.moveCharacter(t, near, -50, 0)
	w.draws.set(99, 80)
	w.hit(t, near, 16)
	if got := ind5gAttackWeight(near, me); got != 200 {
		t.Fatalf("attack desire on the character after a draw of 80 = %v, want 200", got)
	}
	w.draws.set(99, 79)
	w.hit(t, near, 16)
	for _, d := range near.AI().Desires().Snapshot() {
		if d.Kind == actorai.IntentionAttack && d.FinalTarget != nil && d.FinalTarget.ObjectID() != me {
			t.Fatalf("attack desire on %d kept after the switch", d.FinalTarget.ObjectID())
		}
	}
	if got := ind5gAttackWeight(near, me); got != 16.0/8*30+16.0/8*100 {
		t.Fatalf("attack desire on the character after the switch = %v, want %v", got, 16.0/8*30+16.0/8*100)
	}
}

// TestSlowType2SlowsFarAttackers ports WarriorSlowType2's attacked hook:
// a hit from farther than 200 wants its slowing magic on the attacker,
// weight 1000000, on a draw under 70; from 200, or on a draw of 70, none.
func TestSlowType2SlowsFarAttackers(t *testing.T) {
	t.Parallel()
	w := bootIndividual5i(t, raidBoot{}, ind5iStill(ind5iSlow2ID))
	me := w.player.ObjectID()
	n := w.spawn(t, ind5iSlow2ID, 201, 0)
	near := w.spawn(t, ind5iSlow2ID, 0, 200)
	w.draws.set(99, 70)
	w.hit(t, n, 1)
	ind5bNoCast(t, "a hit from 201 on a draw of 70", n, ind5iSlowMagic)
	w.draws.set(99, 69)
	w.hit(t, n, 1)
	ind5dWant(t, w, "a hit from 201 on a draw of 69", n, map[modelskill.ID]int32{ind5iSlowMagic: me})
	w.draws.set(0)
	w.hit(t, near, 1)
	ind5bNoCast(t, "a hit from 200", near, ind5iSlowMagic)
}

// TestSlowTypeAggressivesSlowWhatTheySee ports the see-creature hook of
// WarriorSlowType1Aggressive and WarriorSlowType2Aggressive: each attacks
// the character it sees, weight 200; past its first 7 AI cycles, seeing it
// farther than 100 while not attacking, it also wants its slowing magic on
// it on a draw under 10. The decoy monster it sees takes neither.
func TestSlowTypeAggressivesSlowWhatTheySee(t *testing.T) {
	t.Parallel()
	for _, id := range []int32{ind5iSlow1AggrID, ind5iSlow2AggrID} {
		t.Run(fmt.Sprint(id), func(t *testing.T) {
			t.Parallel()
			w := bootIndividual5i(t, raidBoot{}, ind5iStill(id))
			me := w.player.ObjectID()
			young := w.spawn(t, id, 1500, 0)
			old := w.spawn(t, id, 0, 1500)
			older := w.spawn(t, id, 0, -1500)
			decoy := w.spawn(t, unboundID, 100, 1500)
			w.draws.set(0)
			w.moveCharacter(t, young, -150, 0)
			w.ticks(t, 2)
			if got := ind5gAttackWeight(young, me); got != 200 {
				t.Fatalf("young attack desire on the character = %v, want 200", got)
			}
			ind5bNoCast(t, "seen in its first cycles", young, ind5iSlowMagic)
			if got := ind5gAttackWeight(old, decoy.ObjectID()); got != 0 {
				t.Fatalf("attack desire on the decoy = %v, want none", got)
			}

			for i := 0; script.NPCOf(older).LifeTime() <= 7; i++ {
				if i == 20 {
					t.Fatal("its life time never passed 7")
				}
				cycle(t, w.srv)
			}
			w.draws.set(10)
			w.moveCharacter(t, old, -150, 0)
			w.ticks(t, 2)
			ind5bNoCast(t, "seen past its first cycles on a draw of 10", old, ind5iSlowMagic)
			if got := ind5gAttackWeight(old, me); got == 0 {
				t.Fatalf("attack desire on the character after a draw of 10 = %v, want one", got)
			}
			w.draws.set(9)
			w.moveCharacter(t, older, -150, 0)
			w.ticks(t, 1)
			ind5dWant(t, w, "seen past its first cycles on a draw of 9", older, map[modelskill.ID]int32{ind5iSlowMagic: me})
			if got := ind5gAttackWeight(older, me); got == 0 {
				t.Fatalf("attack desire on the character after a draw of 9 = %v, want one", got)
			}
		})
	}
}
