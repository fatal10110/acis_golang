package ai

import (
	"fmt"
	"slices"
	"testing"
	"time"

	actorai "github.com/fatal10110/acis_golang/internal/gameserver/model/actor/ai"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/move"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/npc"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/script"
	"github.com/fatal10110/acis_golang/internal/gameserver/script/ai/warrior"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
)

// The NPC ids of the fifth individual behavior batch's eighth PR the tests
// spawn.
const (
	ind5hHereticID    = int32(22140) // WarriorHereticCorpseZombiePhysicalSpecial
	ind5hHereticTwoID = int32(22149) // WarriorHereticCorpseZombiePhysicalSpecial
	ind5hNoRangeID    = int32(22066) // WarriorPhysicalSpecialShotNoRange
	ind5hShotAggrID   = int32(22053) // WarriorPhysicalSpecialShotRangeAggressive
	ind5hDebuffID     = int32(22054) // WarriorPhysicalSpecialShotRangeDebuffAggressive
	ind5hPriestID     = int32(20925) // WarriorPriest
	ind5hPriestAggrID = int32(21306) // WarriorPriestAggressive
	ind5hRunnerID     = int32(21508) // WarriorRunAwayPhysicalSpecial
	ind5hRunnerTwoID  = int32(21510) // WarriorRunAwayPhysicalSpecial
)

// The skills the batch's fixture NPCs list by type, and the skill whose
// effect kind holds a runner back.
const (
	ind5hLong       = modelskill.ID(9570) // LONG_RANGE_PHYSICAL_SPECIAL
	ind5hStrike     = modelskill.ID(9571) // PHYSICAL_SPECIAL
	ind5hSelfStrike = modelskill.ID(9572) // SELF_RANGE_PHYSICAL_SPECIAL
	ind5hDebuff1    = modelskill.ID(9573) // DEBUFF1
	ind5hDebuff2    = modelskill.ID(9574) // DEBUFF2
	ind5hDebuff3    = modelskill.ID(9575) // DEBUFF3
	ind5hDebuff4    = modelskill.ID(9576) // DEBUFF4
	ind5hDebuff5    = modelskill.ID(9577) // DEBUFF5
	ind5hBuff       = modelskill.ID(9578) // BUFF
	ind5hHold       = modelskill.ID(1201)
)

// ind5hBatch are the classes of the PR, in scripts.xml order, after the
// zombies the heretic derives from.
var ind5hBatch = []struct {
	path string
	ctor func() script.Script
}{
	{"script.ai.individual.Monster.WarriorBase.Warrior.WarriorCorpseZombie.WarriorCorpseZombie", warrior.CorpseZombie},
	{"script.ai.individual.Monster.WarriorBase.Warrior.WarriorCorpseZombie.WarriorCorpseZombiePhysicalSpecial", warrior.CorpseZombiePhysicalSpecial},
	{"script.ai.individual.Monster.WarriorBase.Warrior.WarriorCorpseZombie.WarriorHereticCorpseZombiePhysicalSpecial", warrior.HereticCorpseZombiePhysicalSpecial},
	{"script.ai.individual.Monster.WarriorBase.Warrior.WarriorPhysicalSpecialShotRange.WarriorPhysicalSpecialShotNoRange", warrior.PhysicalSpecialShotNoRange},
	{"script.ai.individual.Monster.WarriorBase.Warrior.WarriorPhysicalSpecialShotRange.WarriorPhysicalSpecialShotRange", warrior.PhysicalSpecialShotRange},
	{"script.ai.individual.Monster.WarriorBase.Warrior.WarriorPhysicalSpecialShotRange.WarriorPhysicalSpecialShotRangeAggressive", warrior.PhysicalSpecialShotRangeAggressive},
	{"script.ai.individual.Monster.WarriorBase.Warrior.WarriorPhysicalSpecialShotRange.WarriorPhysicalSpecialShotRangeDebuffAggressive", warrior.PhysicalSpecialShotRangeDebuffAggressive},
	{"script.ai.individual.Monster.WarriorBase.Warrior.WarriorPriest.WarriorPriest", warrior.Priest},
	{"script.ai.individual.Monster.WarriorBase.Warrior.WarriorPriest.WarriorPriestAggressive", warrior.PriestAggressive},
	{"script.ai.individual.Monster.WarriorBase.Warrior.WarriorRunAwayPhysicalSpecial.WarriorRunAwayPhysicalSpecial", warrior.RunAwayPhysicalSpecial},
}

// ind5hDefs are the batch's skills: one-target strikes and debuffs of
// reach 2000, the self range strike, the buff with an effect of its own
// stack, and the holding skill, a self effect of its own stack.
func ind5hDefs() []modelskill.Definition {
	defs := []modelskill.Definition{
		ind1Skill(ind5hSelfStrike, 0, true),
		ind5dStacked(ind1Skill(ind5hBuff, 2000, false), "ind5h_buff", 1),
		ind5dStacked(ind1Skill(ind5hHold, 0, true), "ind5h_hold", 1),
	}
	for _, id := range []modelskill.ID{ind5hLong, ind5hStrike, ind5hDebuff1, ind5hDebuff2, ind5hDebuff3, ind5hDebuff4, ind5hDebuff5} {
		defs = append(defs, ind1Skill(id, 2000, false))
	}
	return defs
}

// ind5hTemplate is a fixture monster of id listing the batch's skills by
// type.
func ind5hTemplate(id int32) *npc.Template {
	return ind5dTemplate(id, nil, map[npc.SkillType]modelskill.ID{
		npc.SkillTypeLongRangePhysicalSpecial: ind5hLong, npc.SkillTypePhysicalSpecial: ind5hStrike,
		npc.SkillTypeSelfRangePhysicalSpecial: ind5hSelfStrike, npc.SkillTypeBuff: ind5hBuff,
		npc.SkillTypeDebuff1: ind5hDebuff1, npc.SkillTypeDebuff2: ind5hDebuff2, npc.SkillTypeDebuff3: ind5hDebuff3,
		npc.SkillTypeDebuff4: ind5hDebuff4, npc.SkillTypeDebuff5: ind5hDebuff5,
	})
}

// ind5hStill is ind5hTemplate for an NPC that does not move.
func ind5hStill(id int32) *npc.Template {
	tmpl := ind5hTemplate(id)
	tmpl.CanMove = false
	return tmpl
}

// bootIndividual5h boots the PR's classes after the Warrior chain beside
// templates and an unbound decoy monster that records the script events it
// hears, with a director script to send events and land effects through,
// every NPC casting the batch's skills, every draw 0 until a test queues
// others.
func bootIndividual5h(t *testing.T, extra raidBoot, templates ...*npc.Template) *ind1World {
	t.Helper()
	list, catalog := warriorScripts()
	for _, c := range ind5hBatch {
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
		gameservertest.WithNpcSpawnSkills(modelskill.NewTable(ind5hDefs())),
		gameservertest.WithScriptSkills(ind5hDefs()...),
	}, extra.opts...)
	w := bootRaid(t, raidBoot{opts: opts, ready: extra.ready}, templates...)
	return &ind1World{warriorWorld: w, draws: draws, heard: heard}
}

// ind5hWalk runs h's AI and movement until it walks on its move-to
// desire, then until it arrives.
func (w *ind1World) ind5hWalk(t *testing.T, h *npc.Hostile) {
	t.Helper()
	for i := 0; h.AI().CurrentIntention() != actorai.IntentionMoveTo || !h.IsMoving(); i++ {
		if i == 10 {
			t.Fatal("no walk after ten cycles")
		}
		cycle(t, w.srv)
		for h.IsMoving() && h.AI().CurrentIntention() != actorai.IntentionMoveTo {
			w.srv.TickPositions()
		}
	}
	for i := 0; h.IsMoving(); i++ {
		if i >= int(20*time.Second/move.PositionUpdateInterval) {
			t.Fatal("the walk never arrived")
		}
		w.srv.TickPositions()
	}
	w.srv.Settle(t)
	w.srv.ReadQueued(t, w.srv.Client)
}

// ind5hSetHP sets h's HP to ratio of its maximum.
func (w *ind1World) ind5hSetHP(t *testing.T, h *npc.Hostile, ratio float64) {
	t.Helper()
	w.onQueue(t, h, func() { h.SetHP(float64(h.MaxHP()) * ratio) })
}

// TestPhysicalSpecialShotRangeAnswersHits ports
// WarriorPhysicalSpecialShotRange's created and attacked hooks through
// WarriorPhysicalSpecialShotNoRange, whose created hook only passes on. It
// spawns with i_ai3 clear. A hit from farther than 200 keeps the attacker's
// id in i_ai3 and wants its long range physical special skill on it; a
// draw under 33 then wants its physical special, a second its self range
// physical special, each weight 1000000, the self range one aimed at
// itself. From exactly 200 it wants no long range skill.
func TestPhysicalSpecialShotRangeAnswersHits(t *testing.T) {
	t.Parallel()
	w := bootIndividual5h(t, raidBoot{}, ind5hStill(ind5hNoRangeID))
	me := w.player.ObjectID()
	n := w.spawn(t, ind5hNoRangeID, 300, 0)
	if got := ind1Slot(n, script.SlotIntAI3); got != 0 {
		t.Fatalf("created i_ai3 = %d, want 0", got)
	}

	w.draws.set(99, 32, 33)
	w.hit(t, n, 1)
	ind5dWant(t, w, "a hit from 300, draws 32 and 33", n, map[modelskill.ID]int32{ind5hLong: me, ind5hStrike: me})
	if got := ind1Slot(n, script.SlotIntAI3); got != me {
		t.Fatalf("i_ai3 after a hit = %d, want the character %d", got, me)
	}

	w.ind5dClear(t, n)
	w.moveCharacter(t, n, -200, 0)
	w.draws.set(99, 33, 32)
	w.hit(t, n, 1)
	ind5dWant(t, w, "a hit from 200, draws 33 and 32", n, map[modelskill.ID]int32{ind5hSelfStrike: n.ObjectID()})
}

// TestPhysicalSpecialShotRangeAggressiveShootsFromAfarEvery5Seconds ports
// WarriorPhysicalSpecialShotRangeAggressive's see-creature hook and
// WarriorPhysicalSpecialShotRange's timer. Seeing the character 300 away
// it attacks it, wants its long range skill on it and starts its 1002
// timer. Each firing, 5 s apart, wants the long range skill on its last
// attacker while that one stands farther than 200, and forgets it
// otherwise, firing on.
func TestPhysicalSpecialShotRangeAggressiveShootsFromAfarEvery5Seconds(t *testing.T) {
	t.Parallel()
	w := bootIndividual5h(t, raidBoot{}, ind5hStill(ind5hShotAggrID))
	me := w.player.ObjectID()
	w.draws.set(99)
	n := w.spawn(t, ind5hShotAggrID, 800, 0)
	w.ticks(t, 1)
	w.moveCharacter(t, n, -300, 0)
	w.ticks(t, 1)
	if ind5gAttackWeight(n, me) == 0 {
		t.Fatal("seeing the character wants no attack on it")
	}
	ind5dWant(t, w, "seeing the character from 300", n, map[modelskill.ID]int32{ind5hLong: me})

	w.ind5dClear(t, n)
	w.srv.Advance(t, 5*time.Second)
	ind5dWant(t, w, "the first firing, never hit", n, nil)

	w.hit(t, n, 1)
	w.ind5dClear(t, n)
	w.srv.Advance(t, 5*time.Second)
	ind5dWant(t, w, "a firing after a hit from 300", n, map[modelskill.ID]int32{ind5hLong: me})

	w.ind5dClear(t, n)
	w.moveCharacter(t, n, -100, 0)
	w.srv.Advance(t, 5*time.Second)
	ind5dWant(t, w, "a firing with its attacker 100 away", n, nil)
	if got := ind1Slot(n, script.SlotIntAI3); got != 0 {
		t.Fatalf("i_ai3 after a firing with its attacker close = %d, want 0", got)
	}

	w.moveCharacter(t, n, -300, 0)
	w.ind5dClear(t, n)
	w.srv.Advance(t, 5*time.Second)
	ind5dWant(t, w, "a firing after it forgot its attacker", n, nil)
	w.hit(t, n, 1)
	w.ind5dClear(t, n)
	w.srv.Advance(t, 5*time.Second)
	ind5dWant(t, w, "a firing after a new hit from 300", n, map[modelskill.ID]int32{ind5hLong: me})
}

// TestPhysicalSpecialShotRangeDebuffAggressiveDebuffsBelowAThird ports
// WarriorPhysicalSpecialShotRangeDebuffAggressive's attacked hook. Hit
// below a third of its HP, a draw under 60 wants its first debuff on the
// attacker; otherwise a draw under 50 then one under 50 or not wants the
// second or third, and a draw of 50 or more then one under 50 or not the
// fourth or fifth, each weight 1000000. Above a third it draws nothing, the
// first draw going to its parent's physical special.
func TestPhysicalSpecialShotRangeDebuffAggressiveDebuffsBelowAThird(t *testing.T) {
	t.Parallel()
	w := bootIndividual5h(t, raidBoot{}, ind5hStill(ind5hDebuffID))
	me := w.player.ObjectID()
	w.draws.set(99)
	n := w.spawn(t, ind5hDebuffID, 100, 0)
	for _, row := range []struct {
		draws []int
		want  modelskill.ID
	}{
		{[]int{59}, ind5hDebuff1},
		{[]int{60, 49, 49}, ind5hDebuff2},
		{[]int{60, 49, 50}, ind5hDebuff3},
		{[]int{60, 50, 49}, ind5hDebuff4},
		{[]int{60, 50, 50}, ind5hDebuff5},
	} {
		w.ind5hSetHP(t, n, 0.3)
		w.ind5dClear(t, n)
		w.draws.set(99, row.draws...)
		w.hit(t, n, 1)
		ind5dWant(t, w, fmt.Sprintf("a hit below a third, draws %v", row.draws), n, map[modelskill.ID]int32{row.want: me})
	}

	w.ind5hSetHP(t, n, 0.5)
	w.ind5dClear(t, n)
	w.draws.set(99, 0)
	w.hit(t, n, 1)
	ind5dWant(t, w, "a hit at half its HP, draw 0", n, map[modelskill.ID]int32{ind5hStrike: me})
}

// TestPriestBuffsTheMonstersAroundIt ports WarriorPriest's created,
// see-creature and timer hooks. Its 5001 timer, every 20 s, looks around:
// the first look, 20 s after it appears, sees the decoy monster beside it,
// which holds no effect of its buff's kind, and wants its buff on it,
// weight 1000000. A look seeing the decoy holding the buff wants nothing.
// Seeing the character wants no buff on it.
func TestPriestBuffsTheMonstersAroundIt(t *testing.T) {
	t.Parallel()
	w := bootIndividual5h(t, raidBoot{}, ind5hStill(ind5hPriestID))
	n := w.spawn(t, ind5hPriestID, 1500, 0)
	decoy := w.spawn(t, unboundID, 1600, 0)
	w.srv.Advance(t, 19900*time.Millisecond)
	ind5bNoCast(t, "before the first look", n, ind5hBuff)
	w.srv.Advance(t, 100*time.Millisecond)
	ind5bWantCast(t, "at the first look", n, ind5hBuff, decoy.ObjectID())

	w.ind5dLand(t, modelskill.Ref{ID: ind5hBuff, Level: 1}, script.NPCOf(decoy))
	w.ind5dClear(t, n)
	w.srv.Advance(t, 40*time.Second)
	ind5bNoCast(t, "a look at the buffed decoy", n, ind5hBuff)

	w.moveCharacter(t, n, -200, 0)
	w.ticks(t, 2)
	ind5bNoCast(t, "seeing the character", n, ind5hBuff)
}

// TestPriestAggressiveAttacksOnlyPlayables ports WarriorPriestAggressive's
// see-creature hook: seeing the decoy monster it returns before its
// parent, wanting no buff on it; seeing the character it attacks it.
func TestPriestAggressiveAttacksOnlyPlayables(t *testing.T) {
	t.Parallel()
	w := bootIndividual5h(t, raidBoot{}, ind5hStill(ind5hPriestAggrID))
	n := w.spawn(t, ind5hPriestAggrID, 1500, 0)
	w.spawn(t, unboundID, 1600, 0)
	w.srv.Advance(t, 20*time.Second)
	ind5bNoCast(t, "a look at the decoy", n, ind5hBuff)

	w.moveCharacter(t, n, -200, 0)
	w.ticks(t, 2)
	if ind5gAttackWeight(n, w.player.ObjectID()) == 0 {
		t.Fatal("seeing the character wants no attack on it")
	}
	ind5bNoCast(t, "seeing the character", n, ind5hBuff)
}

// TestHereticCorpseZombiePhysicalSpecialFollowsScriptEvents ports
// WarriorHereticCorpseZombiePhysicalSpecial's created, attacked and
// script-event hooks. It spawns with i_ai4 0, and a hit while it neither
// attacks nor casts sets i_ai4. Event 10035 clears i_ai4, drops its attack
// desires and wants to walk to its spawn point (weight 100). Event 10002
// drops its attack desires and wants to attack the named creature (weight
// 200); 10033 does the same and sets i_ai4. An event naming no creature
// does nothing. A hit while it attacks leaves i_ai4 clear.
func TestHereticCorpseZombiePhysicalSpecialFollowsScriptEvents(t *testing.T) {
	t.Parallel()
	w := bootIndividual5h(t, raidBoot{}, ind5hTemplate(ind5hHereticID))
	me := w.player.ObjectID()
	w.draws.set(99)
	n := w.spawn(t, ind5hHereticID, 300, 0)
	if got := ind1Slot(n, script.SlotIntAI4); got != 0 {
		t.Fatalf("created i_ai4 = %d, want 0", got)
	}
	w.hit(t, n, 10)
	if got := ind1Slot(n, script.SlotIntAI4); got != 1 {
		t.Fatalf("i_ai4 after a hit while idle = %d, want 1", got)
	}

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
	wantDesire(t, "after event 10033", n, actorai.IntentionAttack, me, 200)
	if got := ind1Slot(n, script.SlotIntAI4); got != 1 {
		t.Fatalf("i_ai4 after event 10033 = %d, want 1", got)
	}

	w.onQueue(t, n, func() { script.NPCOf(n).SetInt(script.SlotIntAI4, 0) })
	for i := 0; script.NPCOf(n).Intention() != script.IntentionAttack; i++ {
		if i == 5 {
			t.Fatal("the zombie never turned to attack")
		}
		cycle(t, w.srv)
	}
	w.hit(t, n, 10)
	if got := ind1Slot(n, script.SlotIntAI4); got != 0 {
		t.Fatalf("i_ai4 after a hit while attacking = %d, want 0", got)
	}
}

// TestHereticCorpseZombiePhysicalSpecialAnswersItsClan ports the
// clan-attacked hook: told of the character's hit on a clan member while
// it neither attacks nor casts, the zombie gives up its territory (i_ai4
// set).
func TestHereticCorpseZombiePhysicalSpecialAnswersItsClan(t *testing.T) {
	t.Parallel()
	w := bootIndividual5h(t, raidBoot{}, ind5dClan(ind5hTemplate(ind5hHereticTwoID)), ind5dClan(warriorTemplate(warriorID, nil)))
	w.draws.set(99)
	victim := w.spawn(t, warriorID, 100, 300)
	n := w.spawn(t, ind5hHereticTwoID, 0, 300)
	w.hit(t, victim, 1)
	if got := ind1Slot(n, script.SlotIntAI4); got != 1 {
		t.Fatalf("i_ai4 after a hit on its clan member = %d, want 1", got)
	}
}

// ind5hHereticDens are two npcmakers, each spawning one heretic zombie in
// a 400 square.
const ind5hHereticDens = `<?xml version="1.0" encoding="utf-8"?>
<list>
	<territory name="zombie_den" minZ="-1000" maxZ="1000"><node x="0" y="0"/><node x="400" y="0"/><node x="400" y="400"/><node x="0" y="400"/></territory>
	<npcmaker name="zombie_den" territory="zombie_den" maximumNpcs="1">
		<ai type="default_maker"/>
		<npc id="22140" total="1" pos="40;20;30;0" respawn="60sec"/>
	</npcmaker>
	<territory name="zombie_den2" minZ="-1000" maxZ="1000"><node x="1000" y="0"/><node x="1400" y="0"/><node x="1400" y="400"/><node x="1000" y="400"/></territory>
	<npcmaker name="zombie_den2" territory="zombie_den2" maximumNpcs="1">
		<ai type="default_maker"/>
		<npc id="22149" total="1" pos="1040;20;30;0" respawn="60sec"/>
	</npcmaker>
</list>`

// TestHereticCorpseZombiePhysicalSpecialHeadsHomeOutOfTerritory ports the
// out-of-territory hook, which calls no parent: arriving outside its
// territory, a zombie still bound to it (i_ai4 clear) holds no attack
// desire and wants to walk back to its spawn point, weight 100; one that
// gave the territory up (i_ai4 set) wants no walk home.
func TestHereticCorpseZombiePhysicalSpecialHeadsHomeOutOfTerritory(t *testing.T) {
	t.Parallel()
	w := bootIndividual5h(t, raidBoot{opts: []gameservertest.Option{gameservertest.WithNpcSpawns(ind3bSpawnTable(t, ind5hHereticDens))}},
		ind5hTemplate(ind5hHereticID), ind5hTemplate(ind5hHereticTwoID))
	ow := &ootRoamWorld{srv: w.srv}
	obj, ok := w.srv.State.Player(w.player.ObjectID())
	if !ok {
		t.Fatal("the character is not in the world")
	}
	if err := w.srv.State.Move(obj, 700, -1300, 30); err != nil {
		t.Fatalf("move the character: %v", err)
	}
	for _, row := range []struct {
		id    int32
		home  location.Location
		freed bool
	}{{ind5hHereticID, location.Location{X: 40, Y: 20, Z: 30}, false}, {ind5hHereticTwoID, location.Location{X: 1040, Y: 20, Z: 30}, true}} {
		live := w.liveNPCs(row.id)
		if len(live) != 1 {
			t.Fatalf("setup: %d zombies of %d in the world, want 1", len(live), row.id)
		}
		n := live[0]
		if row.freed {
			w.onQueue(t, n, func() { script.NPCOf(n).SetInt(script.SlotIntAI4, 1) })
		}
		w.clearDesires(t, n)
		ow.walkTo(t, n, location.Location{X: row.home.X, Y: 700, Z: 30})
		if n.InTerritory() {
			t.Fatalf("%d: the walk ended inside the territory, want outside", row.id)
		}
		home := slices.ContainsFunc(n.AI().Desires().Snapshot(), func(d actorai.Desire) bool {
			return d.Kind == actorai.IntentionMoveTo && d.Weight == 100 && d.Location.X == row.home.X && d.Location.Y == row.home.Y
		})
		if home == row.freed {
			t.Fatalf("%d (i_ai4 set %v): desires outside its territory = %+v, want a walk home %v", row.id, row.freed, n.AI().Desires().Snapshot(), !row.freed)
		}
	}
}

// ind5hRunnerDen is an npcmaker spawning one runner whose spawn names a
// flee point 200 east of it.
const ind5hRunnerDen = `<?xml version="1.0" encoding="utf-8"?>
<list>
	<territory name="runner_den" minZ="-1000" maxZ="1000"><node x="0" y="0"/><node x="400" y="0"/><node x="400" y="400"/><node x="0" y="400"/></territory>
	<npcmaker name="runner_den" territory="runner_den" maximumNpcs="1">
		<ai type="default_maker"/>
		<npc id="21508" total="1" pos="40;20;30;0" respawn="60sec"><ai><set name="flee_x" val="240"/><set name="flee_y" val="20"/><set name="flee_z" val="30"/></ai></npc>
	</npcmaker>
</list>`

// ind5hDenRunner boots the den with the listener 100 from the flee point
// and returns the runner.
func ind5hDenRunner(t *testing.T) (*ind1World, *npc.Hostile) {
	t.Helper()
	w := bootIndividual5h(t, raidBoot{opts: []gameservertest.Option{gameservertest.WithNpcSpawns(ind3bSpawnTable(t, ind5hRunnerDen))}}, ind5hTemplate(ind5hRunnerID))
	live := w.liveNPCs(ind5hRunnerID)
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

// ind5hFlee lands a hit of 60% of the runner's HP on a draw of 99, then one
// of 10 on a draw of 0 below half HP, and checks the runner runs to its
// flee point, weight 2000, wanting no strike.
func (w *ind1World) ind5hFlee(t *testing.T, h *npc.Hostile) {
	t.Helper()
	w.draws.set(99)
	w.hit(t, h, int(h.MaxHP())*6/10)
	if got := ind1Slot(h, script.SlotIntAI0); got != 0 {
		t.Fatalf("runner i_ai0 after a hit on a draw of 99 = %d, want 0", got)
	}
	w.draws.set(99, 0)
	w.hit(t, h, 10)
	if got := ind1Slot(h, script.SlotIntAI0); got != 1 {
		t.Fatalf("runner i_ai0 after a hit below half HP = %d, want 1 (running)", got)
	}
	if d, ok := desire(h, actorai.IntentionMoveTo); !ok || d.Weight != 2000 || d.Location.X != 240 || d.Location.Y != 20 || d.Location.Z != 30 {
		t.Fatalf("running runner move desire = %+v (%v), want 240,20,30 with weight 2000", d, ok)
	}
	ind5bNoCast(t, "the run", h, ind5hStrike)
}

// ind5hArrive walks the running runner to its flee point with every draw
// arrivalDraw and checks it wanders there.
func (w *ind1World) ind5hArrive(t *testing.T, h *npc.Hostile, arrivalDraw int) {
	t.Helper()
	w.draws.set(arrivalDraw)
	w.ind5hWalk(t, h)
	if got := ind1Slot(h, script.SlotIntAI0); got != 2 {
		t.Fatalf("runner i_ai0 at its flee point = %d, want 2", got)
	}
	if d, ok := desire(h, actorai.IntentionWander); !ok || d.Weight != 50 || d.Timer != 5 {
		t.Fatalf("runner at its flee point wander desire = %+v (%v), want timer 5 and weight 50", d, ok)
	}
}

// TestRunAwayPhysicalSpecialRunsAndCallsItsKin ports
// WarriorRunAwayPhysicalSpecial's created, attacked, move-finished and
// timer hooks: below half HP a hit, on a draw under 20, sends it to its
// flee point. There, on a draw under 50, its 2001 timer 1 s later wants to
// attack the creature it last fought, weight 1000, and broadcasts script
// event 10000 with that creature within 400. Walking home then ends the
// run.
func TestRunAwayPhysicalSpecialRunsAndCallsItsKin(t *testing.T) {
	t.Parallel()
	w, h := ind5hDenRunner(t)
	me := w.player.ObjectID()
	home := script.NPCOf(h).SpawnLocation()
	w.ind5hFlee(t, h)
	w.ind5hArrive(t, h, 0)
	w.heard.take()

	w.srv.Advance(t, time.Second)
	if got, want := w.heard.take(), []string{fmt.Sprintf("event=10000 arg1=%d arg2=0", me)}; !slices.Equal(got, want) {
		t.Fatalf("events heard 1 s after the run = %q, want %q", got, want)
	}
	if got := ind1Slot(h, script.SlotIntAI0); got != 3 {
		t.Fatalf("runner i_ai0 after its call = %d, want 3", got)
	}
	if got := ind5gAttackWeight(h, me); got < 1000 {
		t.Fatalf("runner attack desire on the character after its call = %v, want at least 1000", got)
	}

	w.onQueue(t, h, func() { script.NPCOf(h).RemoveAllDesire() })
	if !h.AI().AddMoveToDesire(location.Location{X: home.X, Y: home.Y, Z: home.Z}, 1e6) {
		t.Fatal("the walk home was not queued")
	}
	w.ind5hWalk(t, h)
	if got := ind1Slot(h, script.SlotIntAI0); got != 0 {
		t.Fatalf("runner i_ai0 back home = %d, want 0", got)
	}
}

// TestRunAwayPhysicalSpecialHealsBeforeGoingHome runs the runner's 2002
// timer: arriving at its flee point on a draw of 50 or more starts it 1 s
// later; it checks every second until the runner is at full HP, then wants
// its spawn point, weight 50000.
func TestRunAwayPhysicalSpecialHealsBeforeGoingHome(t *testing.T) {
	t.Parallel()
	w, h := ind5hDenRunner(t)
	home := script.NPCOf(h).SpawnLocation()
	w.ind5hFlee(t, h)
	w.ind5hArrive(t, h, 99)
	w.heard.take()

	w.srv.Advance(t, time.Second)
	if got := w.heard.take(); len(got) != 0 {
		t.Fatalf("events heard after a run on a draw of 99 = %q, want none", got)
	}
	if d, ok := desire(h, actorai.IntentionMoveTo); ok && d.Weight == 50000 {
		t.Fatal("wounded runner wants to walk home")
	}
	w.ind5hSetHP(t, h, 1)
	w.srv.Advance(t, time.Second)
	if d, ok := desire(h, actorai.IntentionMoveTo); !ok || d.Weight != 50000 || d.Location.X != home.X || d.Location.Y != home.Y {
		t.Fatalf("healed runner move desire = %+v (%v), want its spawn point %d,%d with weight 50000", d, ok, home.X, home.Y)
	}
}

// TestRunAwayPhysicalSpecialHeldGivesUpTheRun runs the attacked hook of a
// running runner farther than 40 from the creature it hates most: without
// an effect of the holding skill's kind it keeps running; holding one, a
// hit ends the run (i_ai0 set to 3), drops its walk to the flee point and
// wants to attack the character.
func TestRunAwayPhysicalSpecialHeldGivesUpTheRun(t *testing.T) {
	t.Parallel()
	w, h := ind5hDenRunner(t)
	me := w.player.ObjectID()
	w.ind5hFlee(t, h)
	w.teleport(t, h, 200, 0)
	hate := func() { w.onQueue(t, h, func() { h.AddDamageHate(w.player, 0, 100000) }) }

	hate()
	w.draws.set(99)
	w.hit(t, h, 10)
	if got := ind1Slot(h, script.SlotIntAI0); got != 1 {
		t.Fatalf("runner i_ai0 after a hit holding nothing = %d, want 1 (still running)", got)
	}

	w.ind5dLand(t, modelskill.Ref{ID: ind5hHold, Level: 1}, script.NPCOf(h))
	hate()
	w.hit(t, h, 10)
	if got := ind1Slot(h, script.SlotIntAI0); got != 3 {
		t.Fatalf("held runner i_ai0 after a hit = %d, want 3 (run given up)", got)
	}
	if d, ok := desire(h, actorai.IntentionMoveTo); ok {
		t.Fatalf("held runner still wants to walk: %+v", d)
	}
	if ind5gAttackWeight(h, me) == 0 {
		t.Fatal("held runner wants no attack on the character")
	}
}

// TestRunAwayPhysicalSpecialStrikesAndAnswersItsKin ports the runner's
// strike and script-event hook on a spawn naming no flee point: a hit by
// the creature it hates most wants its physical special on it on a draw
// under 33, weight 1000000, and none on 33. Told of event 10000 while it
// does not attack, it wants to attack the creature named, weight 1000; an
// event naming nobody, another event, or one while it attacks changes
// nothing.
func TestRunAwayPhysicalSpecialStrikesAndAnswersItsKin(t *testing.T) {
	t.Parallel()
	w := bootIndividual5h(t, raidBoot{}, ind5hStill(ind5hRunnerTwoID))
	me := w.player.ObjectID()
	n := w.spawn(t, ind5hRunnerTwoID, 100, 0)
	w.draws.set(99)
	w.hit(t, n, 10)
	w.ind5dClear(t, n)
	w.draws.set(99, 33)
	w.hit(t, n, 10)
	ind5bNoCast(t, "a hit by its most hated, draw 33", n, ind5hStrike)
	w.draws.set(99, 32)
	w.hit(t, n, 10)
	ind5dWant(t, w, "a hit by its most hated, draw 32", n, map[modelskill.ID]int32{ind5hStrike: me})

	idle := w.spawn(t, ind5hRunnerTwoID, 0, 300)
	w.sendEvent(t, idle, 10000, 0, 0)
	w.sendEvent(t, idle, 10001, me, 0)
	if d, ok := desire(idle, actorai.IntentionAttack); ok {
		t.Fatalf("attack desire after events naming nobody or not 10000 = %+v, want none", d)
	}
	w.sendEvent(t, idle, 10000, me, 0)
	wantDesire(t, "after event 10000", idle, actorai.IntentionAttack, me, 1000)
	for i := 0; script.NPCOf(idle).Intention() != script.IntentionAttack; i++ {
		if i == 5 {
			t.Fatal("the runner never turned to attack")
		}
		cycle(t, w.srv)
	}
	before := ind5gAttackWeight(idle, me)
	w.sendEvent(t, idle, 10000, me, 0)
	if got := ind5gAttackWeight(idle, me); got != before {
		t.Fatalf("attack desire after event 10000 while attacking = %v, want %v kept", got, before)
	}
}
