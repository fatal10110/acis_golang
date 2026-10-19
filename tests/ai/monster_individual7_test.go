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
	"github.com/fatal10110/acis_golang/internal/gameserver/script/ai/defaultnpc"
	"github.com/fatal10110/acis_golang/internal/gameserver/script/ai/monster"
	"github.com/fatal10110/acis_golang/internal/gameserver/skill/effect"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
	"github.com/rs/zerolog"
)

// The seventh individual behavior batch's NPC ids (#192 PR 1): the
// datapack ids of the classes, each a level 1 fixture monster here, the
// quest NPC that raises the LV3 monsters as its privates (it hears their
// script events) and an unbound private of the LV3 party leaders.
const (
	ind7HeroID    = int32(21260) // WarriorHero
	ind7DaggerID  = int32(27299) // LV3Dagger
	ind7KnightID  = int32(27258) // LV3Knight
	ind7OrcID     = int32(27294) // LV3Orc
	ind7DwarfID   = int32(27259) // LV3Dwarf
	ind7HealerID  = int32(27266) // LV3Healer
	ind7MasterID  = int32(29601) // the raising NPC, a script-event listener
	ind7PrivateID = int32(29602) // no behavior
	ind7Stranger  = int32(12345) // a player object id no character has
)

// The skills the batch's fixture NPCs list by type.
const (
	ind7HeroSkill    = modelskill.ID(9601)
	ind7Buff         = modelskill.ID(9602)
	ind7Special1     = modelskill.ID(9603)
	ind7Special2     = modelskill.ID(9604)
	ind7Special3     = modelskill.ID(9605)
	ind7WeakFalse    = modelskill.ID(9606)
	ind7WeakTrue     = modelskill.ID(9607)
	ind7DefenceMode  = modelskill.ID(9608)
	ind7CrazyMode    = modelskill.ID(9609)
	ind7BigBody      = modelskill.ID(9610)
	ind7NormalBody   = modelskill.ID(9611)
	ind7Debuff1      = modelskill.ID(9612)
	ind7Debuff2      = modelskill.ID(9613)
	ind7Debuff3      = modelskill.ID(9614)
	ind7StunSkillID  = modelskill.ID(9615)
	ind7ShieldStun   = modelskill.ID(1245) // the skill that weakens LV3Orc
	ind7KnightBreak1 = modelskill.ID(28)   // the skills that raise LV3Knight's defence
	ind7KnightBreak2 = modelskill.ID(18)
)

// ind7Batch are the batch's classes, in scripts.xml order.
var ind7Batch = []struct {
	path string
	ctor func() script.Script
}{
	{"script.ai.individual.Monster.WarriorHero", monster.WarriorHero},
	{"script.ai.individual.Monster.LV3Monster.LV3Dagger", monster.LV3Dagger},
	{"script.ai.individual.Monster.LV3Monster.LV3Dwarf", monster.LV3Dwarf},
	{"script.ai.individual.Monster.LV3Monster.LV3Healer", monster.LV3Healer},
	{"script.ai.individual.Monster.LV3Monster.LV3Knight", monster.LV3Knight},
	{"script.ai.individual.Monster.LV3Monster.LV3Monster", monster.LV3Monster},
	{"script.ai.individual.Monster.LV3Monster.LV3Orc", monster.LV3Orc},
	{"script.ai.individual.Monster.LV3Monster.LV3PartyLeaderMonster", monster.LV3PartyLeader},
}

// ind7World is ind1World with the batch registered after DefaultNpc and
// MonsterAI, a listener on ind7MasterID and a spawner for privates that
// start with spawn parameters.
type ind7World struct {
	*ind1World
	spawner *script.Spawner
}

// bootIndividual7 boots the batch beside templates, with extra, every NPC
// casting the batch's fixture skills, every draw 0 until a test queues
// others.
func bootIndividual7(t *testing.T, extra raidBoot, templates ...*npc.Template) *ind7World {
	t.Helper()
	list := []script.Listing{{Path: "script.ai.individual.DefaultNpc"}, {Path: "script.ai.individual.Monster.MonsterAI"}}
	catalog := script.Catalog{"script.ai.individual.DefaultNpc": defaultnpc.New, "script.ai.individual.Monster.MonsterAI": monster.New}
	for _, c := range ind7Batch {
		list = append(list, script.Listing{Path: c.path})
		catalog[c.path] = c.ctor
	}
	heard := &ind1Heard{}
	const listener = "test.Ind7Listener"
	catalog[listener] = func() script.Script {
		return script.Script{Behavior: true, NPCs: []int32{ind7MasterID}, Hooks: script.Hooks{
			OnScriptEvent: func(_ *script.Script, e script.ScriptEvent) {
				heard.mu.Lock()
				defer heard.mu.Unlock()
				heard.events = append(heard.events, fmt.Sprintf("event=%d arg1=%d arg2=%d", e.EventID, e.Arg1, e.Arg2))
			},
		}}
	}
	list = append(list, script.Listing{Path: listener})
	kinds := map[int32]script.NPCKind{}
	for _, tmpl := range templates {
		kinds[int32(tmpl.ID)] = script.KindHostile
	}
	draws := &ind1Draws{}
	opts := append([]gameservertest.Option{
		gameservertest.WithScriptRand(draws.rand),
		gameservertest.WithNPCScripts(kinds, list, catalog),
		gameservertest.WithNpcSpawnSkills(ind7Skills()),
	}, extra.opts...)
	w := bootRaid(t, raidBoot{opts: opts, ready: extra.ready}, templates...)
	return &ind7World{ind1World: &ind1World{warriorWorld: w, draws: draws, heard: heard}, spawner: script.NewSpawner(w.srv.NpcSpawns, zerolog.Nop())}
}

// ind7Skills are the batch's fixture skills: the hero skill, buff and
// physical specials strike one target, the modes and debuffs are cast on
// the NPC itself.
func ind7Skills() *modelskill.Table {
	defs := []modelskill.Definition{
		ind1Skill(ind7HeroSkill, 600, false), ind1Skill(ind7Buff, 600, false),
		ind1Skill(ind7Special1, 600, false), ind1Skill(ind7Special2, 600, false), ind1Skill(ind7Special3, 600, false),
	}
	for _, id := range []modelskill.ID{ind7WeakFalse, ind7WeakTrue, ind7DefenceMode, ind7CrazyMode, ind7BigBody, ind7NormalBody, ind7Debuff1, ind7Debuff2, ind7Debuff3} {
		defs = append(defs, ind1Skill(id, 0, true))
	}
	return modelskill.NewTable(defs)
}

// ind7Template is a level 1 fixture monster of id listing
// every skill of the batch by type.
func ind7Template(id int32) *npc.Template {
	tmpl := warriorTemplate(id, nil)
	tmpl.SkillsByType = map[npc.SkillType]modelskill.Ref{
		npc.SkillTypeHeroSkill:        {ID: ind7HeroSkill, Level: 1},
		npc.SkillTypeBuff:             {ID: ind7Buff, Level: 1},
		npc.SkillTypePhysicalSpecial1: {ID: ind7Special1, Level: 1},
		npc.SkillTypePhysicalSpecial2: {ID: ind7Special2, Level: 1},
		npc.SkillTypePhysicalSpecial3: {ID: ind7Special3, Level: 1},
		npc.SkillTypeWeakModeFalse:    {ID: ind7WeakFalse, Level: 1},
		npc.SkillTypeWeakModeTrue:     {ID: ind7WeakTrue, Level: 1},
		npc.SkillTypeDefenceMode:      {ID: ind7DefenceMode, Level: 1},
		npc.SkillTypeCrazyMode:        {ID: ind7CrazyMode, Level: 1},
		npc.SkillTypeBigBodySkill:     {ID: ind7BigBody, Level: 1},
		npc.SkillTypeNormalBodySkill:  {ID: ind7NormalBody, Level: 1},
		npc.SkillTypeDebuff1:          {ID: ind7Debuff1, Level: 1},
		npc.SkillTypeDebuff2:          {ID: ind7Debuff2, Level: 1},
		npc.SkillTypeDebuff3:          {ID: ind7Debuff3, Level: 1},
	}
	return tmpl
}

// ind7Leader is ind7Template declaring two privates of ind7PrivateID.
func ind7Leader(id int32) *npc.Template {
	tmpl := ind7Template(id)
	for range 2 {
		tmpl.Privates = append(tmpl.Privates, npc.PrivateEntry{NpcID: int(ind7PrivateID), Weight: 1})
	}
	return tmpl
}

// ind7PrivateTemplate is the unbound private, 50000 HP, so one hit can deal
// more damage than its leader's whole HP.
func ind7PrivateTemplate() *npc.Template {
	tmpl := warriorTemplate(ind7PrivateID, nil)
	tmpl.HPMax = 50000
	return tmpl
}

// raise spawns an NPC of id dx, dy from the character as master's private,
// starting with params, and returns it.
func (w *ind7World) raise(t *testing.T, master *npc.Hostile, id int32, dx, dy int, params script.Params) *npc.Hostile {
	t.Helper()
	before := w.liveNPCs(id)
	w.onQueue(t, master, func() {
		if w.spawner.CreateOnePrivateEx(script.NPCOf(master), id, script.Loc{X: w.at.X + dx, Y: w.at.Y + dy, Z: w.at.Z}, 0, params) == nil {
			t.Error("the private was not placed")
		}
	})
	w.srv.ReadQueued(t, w.srv.Client)
	for _, h := range w.liveNPCs(id) {
		if !slices.Contains(before, h) {
			return h
		}
	}
	t.Fatalf("no new npc %d", id)
	return nil
}

// skillHit lands a hit of damage by skill id on h from the character.
func (w *ind7World) skillHit(t *testing.T, h *npc.Hostile, id modelskill.ID, damage float64) {
	t.Helper()
	done := make(chan struct{})
	if !w.srv.PlayerQueue(t, w.player.ObjectID()).Post(func() {
		defer close(done)
		h.ReduceHP(damage, w.player, modelskill.Definition{ID: id, Level: 1})
	}) {
		t.Fatal("the character's queue is closed")
	}
	<-done
	w.srv.Settle(t)
}

// stun lands a five minute stun on h and returns it.
func (w *ind7World) stun(t *testing.T, h *npc.Hostile) *effect.Effect {
	t.Helper()
	e, err := effect.New(effect.Skill{ID: ind7StunSkillID, Level: 1, Debuff: true}, modelskill.EffectTemplate{Name: "Stun", Count: 1, Time: 300})
	if err != nil {
		t.Fatalf("effect.New(Stun): %v", err)
	}
	e.Effector, e.Effected = h, h
	w.onQueue(t, h, func() { h.EffectList().Add(e) })
	if !h.Stunned() {
		t.Fatal("setup: the npc is not stunned")
	}
	return e
}

// attackWeight returns h's attack desire weight on the character, 0 for
// none.
func (w *ind7World) attackWeight(h *npc.Hostile) float64 {
	d, _ := attackOn(h, w.player)
	return d.Weight
}

// clearDesires drops every desire h holds.
func (w *ind7World) clearDesires(t *testing.T, h *npc.Hostile) {
	t.Helper()
	w.onQueue(t, h, func() { h.AI().Desires().Clear() })
}

// TestWarriorHeroComesForItsTarget ports WarriorHero's created and timer
// hooks. Raised with the character as its third spawn parameter, it wants
// to cast its hero skill at the character (weight 1000000) and to attack
// it (1000); with no parameter it wants neither. After 8 s it says the
// line 1000434 plus its draw of 7 (draw 3: 1000437), and after 15 s it
// leaves the world.
func TestWarriorHeroComesForItsTarget(t *testing.T) {
	t.Parallel()
	w := bootIndividual7(t, raidBoot{}, ind7Template(ind7HeroID), warriorTemplate(ind7MasterID, nil))
	me := w.player.ObjectID()
	plain := w.spawn(t, ind7HeroID, 300, 200)
	if _, ok := castDesireOf(plain, ind7HeroSkill); ok {
		t.Fatal("a hero with no target wants its hero skill")
	}
	if _, ok := desire(plain, actorai.IntentionAttack); ok {
		t.Fatal("a hero with no target wants to attack")
	}

	master := w.spawn(t, ind7MasterID, 600, 0)
	hero := w.raise(t, master, ind7HeroID, 300, 0, script.Params{P3: me})
	w.wantCasts(t, "after it spawned", hero, map[modelskill.ID]ind1Cast{ind7HeroSkill: {me, 1000000}})
	if got := w.attackWeight(hero); got != 1000 {
		t.Fatalf("hero attack desire on the character = %v, want 1000", got)
	}

	w.draws.set(3)
	w.srv.Advance(t, 8*time.Second)
	frames := w.srv.ReadQueued(t, w.srv.Client)
	if got := says(frames, hero.ObjectID()); !slices.Equal(got, []string{script.NpcString(1000437)}) {
		t.Fatalf("hero lines after 8 s = %q, want the 1000437 line", got)
	}
	if !w.inWorld(hero) {
		t.Fatal("the hero left the world before 15 s")
	}
	w.srv.Advance(t, 7*time.Second)
	if w.inWorld(hero) || w.inWorld(plain) {
		t.Fatal("a hero stays in the world after 15 s")
	}
}

// TestLV3DaggerHuntsForItsPlayer ports LV3Dagger's and LV3Monster's hooks
// for the player the dagger was raised for, every draw 0 unless queued.
// Raised with the character as its first and second spawn parameters and
// its master as its third, it wants to buff the character (1000000) and
// attack it (200), and remembers its master. The character's hit of 10
// adds 10 / 8 * 100 = 125 twice (its own hook, then LV3Monster's) and,
// the character being its top desire target, its first physical special
// (1000). Every 15 s it wants its weak mode off (draws 0, 0) or on (0, 60).
// After 12 minutes it clears its master's quest slot 0 and leaves the
// world, telling nobody.
func TestLV3DaggerHuntsForItsPlayer(t *testing.T) {
	t.Parallel()
	w := bootIndividual7(t, raidBoot{}, ind7Template(ind7DaggerID), warriorTemplate(ind7MasterID, nil))
	me := w.player.ObjectID()
	master := w.spawn(t, ind7MasterID, 600, 0)
	dagger := w.raise(t, master, ind7DaggerID, 300, 0, script.Params{P1: me, P2: me, P3: master.ObjectID()})
	w.wantCasts(t, "after it spawned", dagger, map[modelskill.ID]ind1Cast{ind7Buff: {me, 1000000}})
	if got := w.attackWeight(dagger); got != 200 {
		t.Fatalf("dagger attack desire after it spawned = %v, want 200", got)
	}
	if c1 := script.NPCOf(dagger).Slot(script.SlotCreature1); c1 == nil || c1.ObjectID() != master.ObjectID() {
		t.Fatalf("dagger creature slot 1 = %v, want its master", c1)
	}

	w.hit(t, dagger, 10)
	if got := w.attackWeight(dagger); got != 450 {
		t.Fatalf("dagger attack desire after the hit = %v, want 200 + 125 + 125", got)
	}
	if d, ok := castDesireOf(dagger, ind7Special1); !ok || d.Weight != 1000 || d.FinalTarget.ObjectID() != me {
		t.Fatalf("first physical special desire = %+v (%v), want 1000 on the character", d, ok)
	}
	if !w.inWorld(dagger) {
		t.Fatal("the dagger left the world on its own player's hit")
	}

	w.clearDesires(t, dagger)
	w.srv.Advance(t, 15*time.Second)
	w.wantCasts(t, "after 15 s", dagger, map[modelskill.ID]ind1Cast{ind7WeakFalse: {dagger.ObjectID(), 1000000}})
	w.clearDesires(t, dagger)
	w.draws.set(0, 0, 60)
	w.srv.Advance(t, 15*time.Second)
	w.wantCasts(t, "after 30 s", dagger, map[modelskill.ID]ind1Cast{ind7WeakTrue: {dagger.ObjectID(), 1000000}})

	w.onQueue(t, master, func() { master.Scratch().SetInt(npc.IntQuest0, 7) })
	w.srv.Advance(t, 12*time.Minute-31*time.Second)
	if !w.inWorld(dagger) {
		t.Fatal("the dagger left the world before 12 minutes")
	}
	w.srv.Advance(t, time.Second)
	if w.inWorld(dagger) {
		t.Fatal("the dagger stays in the world after 12 minutes")
	}
	if got := master.Scratch().Int(npc.IntQuest0); got != 0 {
		t.Fatalf("master quest slot 0 = %d, want 0", got)
	}
	if got := w.heard.take(); len(got) != 0 {
		t.Fatalf("events heard = %q, want none", got)
	}
}

// TestLV3MonsterLeavesForAStranger ports the LV3Monster attacked and
// see-spell hooks for a player other than the one its second spawn
// parameter names: a hit or a seen spell tells its master (event 1000),
// clears the master's quest slot 0 and takes it out of the world. The
// spell of its own player leaves it in place.
func TestLV3MonsterLeavesForAStranger(t *testing.T) {
	t.Parallel()
	w := bootIndividual7(t, ind1AggroSpellBoot(t), ind7Template(ind7KnightID), ind7Template(ind7DaggerID), warriorTemplate(ind7MasterID, nil))
	me := w.player.ObjectID()
	master := w.spawn(t, ind7MasterID, 600, 0)
	stranger := script.Params{P1: me, P2: ind7Stranger, P3: master.ObjectID()}
	knight := w.raise(t, master, ind7KnightID, 300, 0, stranger)

	w.onQueue(t, master, func() { master.Scratch().SetInt(npc.IntQuest0, 7) })
	w.hit(t, knight, 10)
	if w.inWorld(knight) {
		t.Fatal("the knight stays in the world after a stranger's hit")
	}
	if got := w.heard.take(); !slices.Equal(got, []string{"event=1000 arg1=0 arg2=0"}) {
		t.Fatalf("events heard after the hit = %q, want event 1000", got)
	}
	if got := master.Scratch().Int(npc.IntQuest0); got != 0 {
		t.Fatalf("master quest slot 0 after the hit = %d, want 0", got)
	}

	own := w.raise(t, master, ind7DaggerID, 150, 50, script.Params{P1: me, P2: me, P3: master.ObjectID()})
	watcher := w.raise(t, master, ind7KnightID, 150, -50, stranger)
	w.onQueue(t, master, func() { master.Scratch().SetInt(npc.IntQuest0, 7) })
	w.castAggroBuff()
	w.srv.AdvanceUntil(t, "the stranger's knight leaving", func() bool { return !w.inWorld(watcher) })
	if !w.inWorld(own) {
		t.Fatal("the dagger left the world on its own player's spell")
	}
	if got := w.heard.take(); !slices.Equal(got, []string{"event=1000 arg1=0 arg2=0"}) {
		t.Fatalf("events heard after the spell = %q, want event 1000", got)
	}
	if got := master.Scratch().Int(npc.IntQuest0); got != 0 {
		t.Fatalf("master quest slot 0 after the spell = %d, want 0", got)
	}
}

// TestLV3KnightDefenceAndCrazyModes ports LV3Knight's hooks, every draw 0.
// It spawns wanting its defence mode on itself, int slots 0 and 1 clear;
// a plain hit taking 60% of its HP changes neither.
// A hit by skill 28 sets slot 0 and wants the defence mode again with a 5
// s timer (5000 + 1000 * draw); under half its HP the same hit also goes
// crazy (slot 0 to 2, slot 1 set, crazy mode wanted, a 5 s timer), and its
// 4001 timer then clears slot 0 and wants both modes. Once crazy, a later
// hit by skill 18 only raises the defence again, which its 4000 timer
// lowers, wanting the defence mode.
func TestLV3KnightDefenceAndCrazyModes(t *testing.T) {
	t.Parallel()
	w := bootIndividual7(t, raidBoot{}, ind7Template(ind7KnightID), warriorTemplate(ind7MasterID, nil))
	me := w.player.ObjectID()
	master := w.spawn(t, ind7MasterID, 600, 0)
	knight := w.raise(t, master, ind7KnightID, 300, 0, script.Params{P2: me, P3: master.ObjectID()})
	self := knight.ObjectID()
	slots := func() [2]int32 {
		return [2]int32{ind1Slot(knight, script.SlotIntAI0), ind1Slot(knight, script.SlotIntAI1)}
	}
	w.wantCasts(t, "after it spawned", knight, map[modelskill.ID]ind1Cast{ind7DefenceMode: {self, 1000000}})
	if got := slots(); got != [2]int32{0, 0} {
		t.Fatalf("slots after it spawned = %v, want 0 0", got)
	}

	maxHP := int(script.NPCOf(knight).MaxHP())
	w.hit(t, knight, maxHP*6/10)
	if got := slots(); got != [2]int32{0, 0} {
		t.Fatalf("slots after a plain hit = %v, want 0 0", got)
	}
	w.clearDesires(t, knight)
	w.skillHit(t, knight, ind7KnightBreak1, 1)
	if got := slots(); got != [2]int32{2, 1} {
		t.Fatalf("slots after skill 28 under half HP = %v, want 2 1", got)
	}
	w.wantCasts(t, "after skill 28", knight, map[modelskill.ID]ind1Cast{ind7DefenceMode: {self, 1000000}, ind7CrazyMode: {self, 1000000}})

	w.clearDesires(t, knight)
	w.srv.Advance(t, 5*time.Second)
	if got := slots(); got != [2]int32{0, 1} {
		t.Fatalf("slots after the 5 s timers = %v, want 0 1", got)
	}
	w.wantCasts(t, "after 4001", knight, map[modelskill.ID]ind1Cast{ind7DefenceMode: {self, 1000000}, ind7CrazyMode: {self, 1000000}})

	w.clearDesires(t, knight)
	w.skillHit(t, knight, ind7KnightBreak2, 1)
	if got := slots(); got != [2]int32{1, 1} {
		t.Fatalf("slots after skill 18 = %v, want 1 1", got)
	}
	w.wantCasts(t, "after skill 18", knight, map[modelskill.ID]ind1Cast{ind7DefenceMode: {self, 1000000}})
	w.clearDesires(t, knight)
	w.srv.Advance(t, 5*time.Second)
	if got := slots(); got != [2]int32{0, 1} {
		t.Fatalf("slots after 4000 = %v, want 0 1", got)
	}
	w.wantCasts(t, "after 4000", knight, map[modelskill.ID]ind1Cast{ind7DefenceMode: {self, 1000000}})
}

// TestLV3OrcWeakensUnderStuns ports LV3Orc's hooks. It spawns glowing at
// +10, wanting its big body on itself. Only a hit by skill 1245 while it
// is stunned moves its int slot 0 on (0 to 1, dropping the glow; 2 to 3;
// 4 to 5). Its 1 s timer, once the stun is gone, acts on 1, 3 and 5: it
// drops its desires, wants the step's debuff on itself (with its normal
// body at 3), moves the slot on and attacks the creature its first spawn
// parameter named (200). Its combat draws are pinned so that no hit
// breaks a stun.
func TestLV3OrcWeakensUnderStuns(t *testing.T) {
	t.Parallel()
	w := bootIndividual7(t, raidBoot{}, ind7Template(ind7OrcID), warriorTemplate(ind7MasterID, nil))
	me := w.player.ObjectID()
	master := w.spawn(t, ind7MasterID, 600, 0)
	orc := w.raise(t, master, ind7OrcID, 300, 0, script.Params{P1: me, P2: me, P3: master.ObjectID()})
	// Every combat draw n-1: a hit never breaks the stun (a draw of 0 of 10).
	w.onQueue(t, orc, func() { orc.SetRollSource(func(n int) int { return n - 1 }) })
	self := orc.ObjectID()
	step := func() int32 { return ind1Slot(orc, script.SlotIntAI0) }
	if got := orc.NPCInfoSnapshot().EnchantEffect; got != 10 {
		t.Fatalf("orc glow after it spawned = %d, want 10", got)
	}
	w.wantCasts(t, "after it spawned", orc, map[modelskill.ID]ind1Cast{ind7BigBody: {self, 1000000}})

	w.skillHit(t, orc, ind7ShieldStun, 1)
	if got := step(); got != 0 {
		t.Fatalf("step after skill 1245 unstunned = %d, want 0", got)
	}
	stun := w.stun(t, orc)
	w.skillHit(t, orc, ind7KnightBreak1, 1)
	if got := step(); got != 0 {
		t.Fatalf("step after another skill stunned = %d, want 0", got)
	}
	w.skillHit(t, orc, ind7ShieldStun, 1)
	if got := step(); got != 1 {
		t.Fatalf("step after skill 1245 stunned = %d, want 1", got)
	}
	if got := orc.NPCInfoSnapshot().EnchantEffect; got != 0 {
		t.Fatalf("orc glow after the first step = %d, want 0", got)
	}
	w.srv.Advance(t, time.Second)
	if got := step(); got != 1 {
		t.Fatalf("step after a stunned second = %d, want 1", got)
	}

	for i, s := range []struct {
		from  int32
		casts []modelskill.ID
	}{
		{1, []modelskill.ID{ind7Debuff1}},
		{3, []modelskill.ID{ind7Debuff2, ind7NormalBody}},
		{5, []modelskill.ID{ind7Debuff3}},
	} {
		if i > 0 {
			stun = w.stun(t, orc)
			w.skillHit(t, orc, ind7ShieldStun, 1)
			if got := step(); got != s.from {
				t.Fatalf("step after skill 1245 stunned = %d, want %d", got, s.from)
			}
		}
		w.onQueue(t, orc, func() { orc.EffectList().Remove(stun) })
		w.skillHit(t, orc, ind7KnightBreak1, 1)
		w.srv.Advance(t, time.Second)
		if got := step(); got != s.from+1 {
			t.Fatalf("step after the timer from %d = %d, want %d", s.from, got, s.from+1)
		}
		want := map[modelskill.ID]ind1Cast{}
		for _, id := range s.casts {
			want[id] = ind1Cast{self, 1000000}
		}
		w.wantCasts(t, fmt.Sprintf("after the timer from %d", s.from), orc, want)
		if got := w.attackWeight(orc); got != 200 {
			t.Fatalf("attack desire after the timer from %d = %v, want 200", s.from, got)
		}
	}
	w.srv.Advance(t, time.Second)
	if got := step(); got != 6 {
		t.Fatalf("step after the last debuff = %d, want 6", got)
	}
}

// TestLV3DwarfGuardsItsParty ports LV3PartyLeaderMonster's and LV3Dwarf's
// hooks. As it spawns it creates its two declared privates (#2081's
// unconditional trigger). Its player's hit on a private of two and a half
// times the leader's maximum HP adds the party formula f0 * damage * the
// private's weight point 1 * 10, f0 = damage / 8, and
// LV3PartyLeaderMonster's (damage / max HP, an integer share: 2) / 0.05 *
// 50. A private's death creates a new one of its kind. A stranger's hit
// on a private tells the leader's master (event 1000) and takes the leader
// out of the world.
func TestLV3DwarfGuardsItsParty(t *testing.T) {
	t.Parallel()
	w := bootIndividual7(t, raidBoot{}, ind7Leader(ind7DwarfID), ind7PrivateTemplate(), warriorTemplate(ind7MasterID, nil))
	me := w.player.ObjectID()
	master := w.spawn(t, ind7MasterID, 600, 0)
	dwarf := w.raise(t, master, ind7DwarfID, 300, 0, script.Params{P2: me, P3: master.ObjectID()})
	privates := privatesOf(t, dwarf, 2)

	// A hit of two and a half times the leader's maximum HP: the share
	// truncates to 2.
	maxHP := script.NPCOf(dwarf).MaxHP()
	damage := maxHP * 5 / 2
	w.hit(t, privates[0], int(damage))
	f0 := float64(damage) / 8
	want := f0*float64(damage)*1*10 + (float64(damage/maxHP)/0.05)*50
	if damage/maxHP != 2 {
		t.Fatalf("setup: share %d / %d = %d, want 2", damage, maxHP, damage/maxHP)
	}
	if got := w.attackWeight(dwarf); got != want {
		t.Fatalf("dwarf attack desire after the hit on its private = %v, want %v", got, want)
	}

	w.kill(t, privates[1])
	live := livePrivates(t, dwarf, 2)
	if slices.Contains(live, privates[1]) {
		t.Fatal("the dead private counts as live")
	}

	other := w.raise(t, master, ind7DwarfID, 300, 300, script.Params{P2: ind7Stranger, P3: master.ObjectID()})
	w.hit(t, privatesOf(t, other, 2)[0], 10)
	if w.inWorld(other) {
		t.Fatal("the dwarf stays in the world after a stranger's hit on its private")
	}
	if got := w.heard.take(); !slices.Equal(got, []string{"event=1000 arg1=0 arg2=0"}) {
		t.Fatalf("events heard = %q, want event 1000", got)
	}
}

// TestLV3HealerGuardsItsParty ports LV3Healer's hooks: it creates its two
// privates as it spawns, its player's hit of 10 on a private adds 10 / 8 *
// 10 * 1 * 10 = 125 (the integer share 10 / 1000 adds 0), a hit of 10 on
// itself adds only LV3Monster's 125 (the party formula skips the leader),
// and a private's death creates a new one of its kind while its own death
// creates none.
func TestLV3HealerGuardsItsParty(t *testing.T) {
	t.Parallel()
	w := bootIndividual7(t, raidBoot{}, ind7Leader(ind7HealerID), ind7PrivateTemplate(), warriorTemplate(ind7MasterID, nil))
	me := w.player.ObjectID()
	master := w.spawn(t, ind7MasterID, 600, 0)
	healer := w.raise(t, master, ind7HealerID, 300, 0, script.Params{P2: me, P3: master.ObjectID()})
	privates := privatesOf(t, healer, 2)

	w.hit(t, privates[0], 10)
	if got := w.attackWeight(healer); got != 125 {
		t.Fatalf("healer attack desire after the hit on its private = %v, want 125", got)
	}
	w.hit(t, healer, 10)
	if got := w.attackWeight(healer); got != 250 {
		t.Fatalf("healer attack desire after the hit on itself = %v, want 250", got)
	}

	w.kill(t, privates[0])
	livePrivates(t, healer, 2)
	before := len(w.liveNPCs(ind7PrivateID))
	w.kill(t, healer)
	if got := len(w.liveNPCs(ind7PrivateID)); got > before {
		t.Fatalf("live privates after the healer's death = %d, want at most %d", got, before)
	}
}
