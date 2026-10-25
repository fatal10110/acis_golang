package ai

import (
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"sync/atomic"
	"testing"
	"time"

	gamexml "github.com/fatal10110/acis_golang/internal/gameserver/data/xml"
	actorai "github.com/fatal10110/acis_golang/internal/gameserver/model/actor/ai"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/npc"
	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/script"
	"github.com/fatal10110/acis_golang/internal/gameserver/script/ai/defaultnpc"
	"github.com/fatal10110/acis_golang/internal/gameserver/script/ai/monster"
	"github.com/fatal10110/acis_golang/internal/gameserver/script/ai/raid"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
	"github.com/rs/zerolog"
)

// The batch's NPCs and the fixture skills they list by type.
const (
	rb7AndreasID  = int32(29062) // RaidBossType4AndreasAggressive
	rb7Type5ID    = int32(25109) // RaidBossType5
	rb7ArcherID   = int32(25005) // RaidPrivateArcher
	rb7FighterID  = int32(25003) // RaidPrivateFighter
	rb7Teleport   = modelskill.ID(9950)
	rb7Hold       = modelskill.ID(9951)
	rb7LongRange  = modelskill.ID(9952)
	rb7PhysicalA  = modelskill.ID(9953)
	rb7AltarSkill = modelskill.ID(4183)
)

// Andreas's start and teleport points, its spawn naming none.
const (
	rb7StartX, rb7StartY       = -16385, -53268
	rb7Tel1X, rb7Tel1Y         = -16393, -53433
	rb7Tel2X, rb7Tel2Y         = -17150, -54064
	rb7Tel3X, rb7Tel3Y         = -15690, -54030
	rb7AndreasBroadcast        = 300
	rb7AndreasSeek             = 500
	rb7AndreasLeave            = 5 * time.Hour
	rb7AndreasRecheck          = 30 * time.Minute
	rb7AndreasStageQuietPeriod = 30 * time.Minute
)

// rb7Batch is the raid chain in scripts.xml order with the batch's classes.
var rb7Batch = []struct {
	path string
	ctor func() script.Script
}{
	{"script.ai.individual.DefaultNpc", defaultnpc.New},
	{"script.ai.individual.Monster.MonsterAI", monster.New},
	{"script.ai.individual.Monster.RaidBoss.RaidBossStandard", raid.BossStandard},
	{"script.ai.individual.Monster.RaidBoss.RaidBossParty.RaidBossParty", raid.BossParty},
	{"script.ai.individual.Monster.RaidBoss.RaidBossParty.RaidBossType4", raid.BossType4},
	{"script.ai.individual.Monster.RaidBoss.RaidBossParty.RaidBossType4AndreasAggressive", raid.BossType4AndreasAggressive},
	{"script.ai.individual.Monster.RaidBoss.RaidBossParty.RaidBossType5", raid.BossType5},
	{"script.ai.individual.Monster.RaidPrivate.RaidPrivateArcher", raid.PrivateArcher},
	{"script.ai.individual.Monster.RaidPrivate.RaidPrivateFighter", raid.PrivateFighter},
	{"script.ai.individual.Monster.RaidPrivate.RaidPrivateStandard", raid.PrivateStandard},
}

// rb7Spawnlist declares the altar's six combat npcmakers Andreas talks to,
// all probes.
func rb7Spawnlist() string {
	makers := ""
	for _, name := range rb7AltarMakers() {
		makers += fmt.Sprintf(`	<npcmaker name="%s" territory="rb7_altar" maximumNpcs="1"><ai type="rb5_probe"/></npcmaker>
`, name)
	}
	return `<?xml version="1.0" encoding="utf-8"?>
<list>
	<territory name="rb7_altar" minZ="-100" maxZ="200"><node x="0" y="0"/><node x="400" y="0"/><node x="400" y="400"/><node x="0" y="400"/></territory>
` + makers + `</list>`
}

// rb7AltarMakers are the altar's combat npcmakers, first set then second.
func rb7AltarMakers() []string {
	return []string{"altar_combat1_01", "altar_combat1_02", "altar_combat1_03", "altar_combat2_01", "altar_combat2_02", "altar_combat2_03"}
}

// rb7MakerEvents is event run on each of names, in order.
func rb7MakerEvents(event string, names ...string) []string {
	out := make([]string, len(names))
	for i, name := range names {
		out[i] = name + " " + event
	}
	return out
}

// rb7Skills are the batch's skills beside the raid boss fixture skills:
// Andreas's teleport effect and altar debuff, Type 5's ranged hold, whose
// effect a test lands to close its gate, and the privates' specials.
func rb7Skills() []modelskill.Definition {
	return append(rbSkills(),
		ind1Skill(rb7Teleport, 0, true), ind1Skill(rb7AltarSkill, 0, true),
		rb6Stacked(ind1Skill(rb7Hold, 600, false), "rb7_hold"),
		ind1Skill(rb7LongRange, 900, false), ind1Skill(rb7PhysicalA, 600, false),
	)
}

// bootRaid7 boots the batch beside templates and a listener, every NPC
// casting rb7Skills and the raid curse, with the probe npcmakers of
// rb7Spawnlist.
func bootRaid7(t *testing.T, extra raidBoot, templates ...*npc.Template) *rb5World {
	t.Helper()
	var list []script.Listing
	catalog := script.Catalog{}
	for _, c := range rb7Batch {
		list = append(list, script.Listing{Path: c.path})
		catalog[c.path] = c.ctor
	}
	heard := &ind1Heard{}
	const listener, director = "test.Rb7Listener", "test.Ind1Director"
	catalog[listener] = func() script.Script {
		return script.Script{Behavior: true, NPCs: []int32{rb5ListenerID}, Hooks: script.Hooks{
			OnScriptEvent: func(_ *script.Script, e script.ScriptEvent) {
				heard.mu.Lock()
				defer heard.mu.Unlock()
				heard.events = append(heard.events, fmt.Sprintf("event=%d arg1=%d arg2=%d", e.EventID, e.Arg1, e.Arg2))
			},
		}}
	}
	catalog[director] = func() script.Script { return script.Script{} }
	list = append(list, script.Listing{Path: listener}, script.Listing{Path: director})
	templates = append(templates, warriorTemplate(rb5ListenerID, nil))
	kinds := map[int32]script.NPCKind{}
	for _, tmpl := range templates {
		kinds[int32(tmpl.ID)] = script.KindHostile
	}
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "rb7.xml"), []byte(rb7Spawnlist()), 0o644); err != nil {
		t.Fatalf("write spawnlist: %v", err)
	}
	table, err := gamexml.LoadSpawnlist(dir, zerolog.Nop(), 1)
	if err != nil {
		t.Fatalf("load spawnlist: %v", err)
	}
	makers := &rb5Makers{}
	draws := &ind1Draws{}
	skills := rb7Skills()
	w := bootRaid(t, raidBoot{ready: extra.ready, opts: append([]gameservertest.Option{
		gameservertest.WithScriptRand(draws.rand),
		gameservertest.WithNPCScripts(kinds, list, catalog),
		gameservertest.WithNpcSpawnSkills(modelskill.NewTable(append(skills, raidCurseDefinition()))),
		gameservertest.WithScriptSkills(skills...),
		gameservertest.WithNpcSpawns(table),
		gameservertest.WithMakers(makers.catalog()),
	}, extra.opts...)}, templates...)
	return &rb5World{
		ind7World: &ind7World{ind1World: &ind1World{warriorWorld: w, draws: draws, heard: heard}, spawner: script.NewSpawner(w.srv.NpcSpawns, zerolog.Nop())},
		makers:    makers,
	}
}

// rb7Andreas is Andreas at level 20, standing still and seeing 1000 around
// itself, with its teleport effect, its four privates of rb5PrivateID (the
// third moved onto the first's spot, as the shipped spawn does), and
// broadcast, seek and party point ranges of its own. seer makes it a plain
// monster, which tells its see-creature hook about a playable as it comes
// in sight.
func rb7Andreas(seer bool) *npc.Template {
	private := fmt.Sprint(rb5PrivateID)
	tmpl := rbTemplate(rb7AndreasID, 20, npc.AIParams{
		"private1_silhouette": private, "private2_silhouette": private, "private3_silhouette": private, "private4_silhouette": private,
		"private3_pos_y": "-95", "BroadcastRange": fmt.Sprint(rb7AndreasBroadcast), "SeekRange": fmt.Sprint(rb7AndreasSeek),
		"AttackPartyPointRate": "70",
	}, map[npc.SkillType]modelskill.ID{npc.SkillTypeTeleportEffect: rb7Teleport})
	tmpl.CanMove, tmpl.AggroRange = false, 1000
	if seer {
		tmpl.Type = "Monster"
	}
	return tmpl
}

// rb7AndreasWorld boots the batch with Andreas and its private, spawns
// the listener 50 beyond where Andreas then appears, 400 from the
// character, every draw 99, and returns Andreas and the listener.
func rb7AndreasWorld(t *testing.T, seer bool, opts ...gameservertest.Option) (*rb5World, *npc.Hostile, *npc.Hostile) {
	t.Helper()
	w := bootRaid7(t, raidBoot{opts: opts}, rb7Andreas(seer), rb5PrivateTemplate())
	w.draws.set(99)
	listener := w.spawn(t, rb5ListenerID, 450, 0)
	andreas := w.spawn(t, rb7AndreasID, 400, 0)
	return w, andreas, listener
}

// heardEvent formats the script event the listener hears.
func heardEvent(id, arg1 int32) string { return fmt.Sprintf("event=%d arg1=%d arg2=0", id, arg1) }

// heardOf keeps the events of id the listener heard.
func heardOf(events []string, id int32) []string {
	prefix := fmt.Sprintf("event=%d ", id)
	var out []string
	for _, e := range events {
		if len(e) >= len(prefix) && e[:len(prefix)] == prefix {
			out = append(out, e)
		}
	}
	return out
}

// rb7HitDown hits h from the character down to ratio of its HP and
// returns the damage.
func (w *rb5World) rb7HitDown(t *testing.T, h *npc.Hostile, ratio float64) int {
	t.Helper()
	n := script.NPCOf(h)
	damage := int(n.HP() - ratio*float64(n.MaxHP()))
	w.hit(t, h, damage)
	return damage
}

// andreasFoe returns Andreas's c_ai0, nil for none.
func andreasFoe(h *npc.Hostile) script.Creature { return script.NPCOf(h).Slot(script.SlotCreature0) }

// TestAndreasAppearsWithItsPrivates ports Andreas's created hook: its
// memory starts blank with itself as its foe, it tells the NPCs within
// twice its broadcast range event 0 with its object id, and it places its
// four privates around its start, (-16385, -53268) when its spawn names
// none, at the offsets its spawn gives, else (-91, -95), (-48, -130),
// (-91, 95) and (-48, 130). It runs no parent: no raid music timer.
func TestAndreasAppearsWithItsPrivates(t *testing.T) {
	t.Parallel()
	w, andreas, _ := rb7AndreasWorld(t, false)
	if got, want := w.heard.take(), []string{heardEvent(0, andreas.ObjectID())}; !slices.Equal(got, want) {
		t.Fatalf("events heard as Andreas appeared = %q, want %q", got, want)
	}
	for _, slot := range []script.IntSlot{script.SlotIntAI2, script.SlotIntAI3, script.SlotIntAI4, npc.IntQuest0, npc.IntQuest1} {
		if got := ind1Slot(andreas, slot); got != 0 {
			t.Errorf("slot %d = %d, want 0", slot, got)
		}
	}
	if foe := andreasFoe(andreas); !script.Same(foe, script.NPCOf(andreas)) {
		t.Errorf("c_ai0 = %v, want Andreas itself", foe)
	}
	privates := privatesOf(t, andreas, 4)
	want := [][2]int{{-91, -95}, {-48, -130}, {-91, -95}, {-48, 130}}
	for i, p := range privates {
		at := positionOf(p)
		if at.X != rb7StartX+want[i][0] || at.Y != rb7StartY+want[i][1] {
			t.Errorf("private %d stands at (%d, %d), want (%d, %d)", i+1, at.X, at.Y, rb7StartX+want[i][0], rb7StartY+want[i][1])
		}
	}
}

// TestAndreasLeavesAnUntouchedAltar ports Andreas's 6008 timer. Five hours
// after it appears, at altar stage 2, it stays and checks again 30 minutes
// later; back at stage 0 by then, it runs maker script event 1000 on the
// altar's six combat npcmakers, tells the NPCs within twice its broadcast
// range event 6 and leaves the world.
func TestAndreasLeavesAnUntouchedAltar(t *testing.T) {
	t.Parallel()
	w, andreas, _ := rb7AndreasWorld(t, false)
	w.sendEvent(t, andreas, 2, 0, 0)
	w.srv.Advance(t, rb7AndreasLeave)
	if !w.inWorld(andreas) {
		t.Fatal("Andreas left the world at stage 2")
	}
	if got := w.makers.take(); len(got) != 0 {
		t.Fatalf("maker events at stage 2 = %q, want none", got)
	}
	w.sendEvent(t, andreas, 0, 0, 0)
	w.srv.Advance(t, rb7AndreasRecheck-time.Second)
	if !w.inWorld(andreas) {
		t.Fatal("Andreas left the world before its next check")
	}
	w.heard.take()
	w.srv.Advance(t, time.Second)
	if got, want := w.makers.take(), rb7MakerEvents("1000", rb7AltarMakers()...); !slices.Equal(got, want) {
		t.Fatalf("maker events as Andreas left = %q, want %q", got, want)
	}
	if got, want := heardOf(w.heard.take(), 6), []string{heardEvent(6, andreas.ObjectID())}; !slices.Equal(got, want) {
		t.Fatalf("event 6 heard as Andreas left = %q, want %q", got, want)
	}
	if w.inWorld(andreas) {
		t.Fatal("Andreas stays in the world past its check")
	}
}

// TestAndreasTellsItsFoeWhenHit ports Andreas's attacked hook. A hit runs
// RaidBossType4's attacked hooks before and after Andreas's own, so the
// character's attack desire grows by twice the hate. Its foe becomes its
// top desire target, here the character, and on a draw under its party
// point rate (70) it tells the NPCs within its seek range event 10002 with
// its foe's object id. A hit by an NPC, with no top desire target, makes
// the NPC its foe.
func TestAndreasTellsItsFoeWhenHit(t *testing.T) {
	t.Parallel()
	w, andreas, listener := rb7AndreasWorld(t, false)
	w.heard.take()
	me := w.player.ObjectID()
	w.onQueue(t, listener, func() { andreas.TakeDamage(10, listener) })
	w.srv.Settle(t)
	if foe := andreasFoe(andreas); foe == nil || foe.ObjectID() != listener.ObjectID() {
		t.Fatalf("c_ai0 after a hit by the listener = %v, want the listener", foe)
	}
	w.heard.take()

	w.draws.set(99, 69)
	w.hit(t, andreas, 10)
	if got, want := w.attackWeight(andreas), 2*rbHate(10, 20); got != want {
		t.Fatalf("attack desire after a hit = %v, want %v", got, want)
	}
	if foe := andreasFoe(andreas); foe == nil || foe.ObjectID() != me {
		t.Fatalf("c_ai0 after a hit = %v, want the character", foe)
	}
	if got, want := w.heard.take(), []string{heardEvent(10002, me)}; !slices.Equal(got, want) {
		t.Fatalf("events heard on a draw of 69 = %q, want %q", got, want)
	}
	w.draws.set(99, 70)
	w.hit(t, andreas, 10)
	if got := w.heard.take(); len(got) != 0 {
		t.Fatalf("events heard on a draw of 70 = %q, want none", got)
	}
}

// TestAndreasTeleportsAtItsHPSteps ports Andreas's HP steps and its
// skill-finished hook, with its fixture teleport effect. Its attacked hook
// reads the HP the hit found, so the hit after the one that takes it below
// 60% of its HP has it drop its attack desires and want its teleport effect
// on itself at 1000000, then its parent's hate comes back. The cast done, on
// a draw of 0 in 2 it jumps to its third teleport point and runs maker
// script event 1001 on the first combat makers; 500 ms later it drops its
// desires and hate (i_quest0 1), and 10 s later picks up again (i_quest0
// 0). Below 30%, on 1 in 3, it jumps to its second point and starts the
// second combat makers. Below 10% it jumps to its first point, and 10 s
// later, not before, picks up its fight with its foe (i_quest0 0).
func TestAndreasTeleportsAtItsHPSteps(t *testing.T) {
	t.Parallel()
	w, andreas, _ := rb7AndreasWorld(t, false)
	self := andreas.ObjectID()
	teleport := map[modelskill.ID]ind1Cast{rb7Teleport: {self, 1000000}}
	at := func(when string, x, y int) {
		t.Helper()
		if p := positionOf(andreas); p.X != x || p.Y != y {
			t.Fatalf("%s: Andreas stands at (%d, %d), want (%d, %d)", when, p.X, p.Y, x, y)
		}
	}

	w.rb7HitDown(t, andreas, 0.55)
	w.wantCasts(t, "on the hit that takes it below 60% of its HP", andreas, nil)
	w.hit(t, andreas, 1)
	w.wantCasts(t, "below 60% of its HP", andreas, teleport)
	if got, want := w.attackWeight(andreas), rbHate(1, 20); got != want {
		t.Fatalf("attack desire below 60%% of its HP = %v, want %v", got, want)
	}
	w.makers.take()
	w.draws.set(0)
	w.castQueued(t, andreas)
	at("after the first teleport", rb7Tel3X, rb7Tel3Y)
	if got := ind1Slot(andreas, script.SlotIntAI4); got != 1 {
		t.Fatalf("i_ai4 after the first teleport = %d, want 1", got)
	}
	if got, want := w.makers.take(), rb7MakerEvents("1001", rb7AltarMakers()[:3]...); !slices.Equal(got, want) {
		t.Fatalf("maker events after the first teleport = %q, want %q", got, want)
	}
	w.srv.Advance(t, 500*time.Millisecond)
	if d := andreas.AI().Desires().Snapshot(); len(d) != 0 || ind1Slot(andreas, npc.IntQuest0) != 1 {
		t.Fatalf("500 ms after the teleport: desires %+v (want none), i_quest0 %d (want 1)", d, ind1Slot(andreas, npc.IntQuest0))
	}
	if got := andreas.AggroHate(w.player); got != 0 {
		t.Fatalf("hate on the character 500 ms after the teleport = %v, want none", got)
	}
	w.srv.Advance(t, 9500*time.Millisecond)
	if got := ind1Slot(andreas, npc.IntQuest0); got != 0 {
		t.Fatalf("i_quest0 10 s after the teleport = %d, want 0", got)
	}

	w.rb7HitDown(t, andreas, 0.2)
	w.hit(t, andreas, 1)
	w.wantCasts(t, "below 30% of its HP", andreas, teleport)
	w.draws.set(1)
	w.castQueued(t, andreas)
	at("after the second teleport", rb7Tel2X, rb7Tel2Y)
	if got, want := w.makers.take(), rb7MakerEvents("1001", rb7AltarMakers()[3:]...); !slices.Equal(got, want) {
		t.Fatalf("maker events after the second teleport = %q, want %q", got, want)
	}
	w.srv.Advance(t, 10*time.Second)

	w.rb7HitDown(t, andreas, 0.05)
	w.hit(t, andreas, 1)
	w.wantCasts(t, "below 10% of its HP", andreas, teleport)
	w.castQueued(t, andreas)
	at("after the third teleport", rb7Tel1X, rb7Tel1Y)
	if got := ind1Slot(andreas, script.SlotIntAI4); got != 3 {
		t.Fatalf("i_ai4 after the third teleport = %d, want 3", got)
	}
	if got := w.makers.take(); len(got) != 0 {
		t.Fatalf("maker events after the third teleport = %q, want none", got)
	}
	w.srv.Advance(t, 9900*time.Millisecond)
	if got := ind1Slot(andreas, npc.IntQuest0); got != 1 {
		t.Fatalf("i_quest0 9.9 s after the third teleport = %d, want 1", got)
	}
	w.srv.Advance(t, 100*time.Millisecond)
	if got := ind1Slot(andreas, npc.IntQuest0); got != 0 {
		t.Fatalf("i_quest0 10 s after the third teleport = %d, want 0", got)
	}
}

// TestAndreasFollowsTheAltarStages ports Andreas's script event hook and
// the timers it arms. Event 1 sets the stage and checks every minute for a
// fight: half an hour on from its last hit, not before, it drops its attack
// desires and tells the NPCs within its broadcast range event 7. Event 4
// names a foe: it tells the NPCs within its seek range event 10020 with
// it, and attacks it at 700 when it is a playable. Event 3 has it cast
// 4183 on itself 6.7 s later. Event 7 resets its memory. Event 9 is not a
// stage.
func TestAndreasFollowsTheAltarStages(t *testing.T) {
	t.Parallel()
	// The game ticks count the in-game clock's uptime, moved here a minute
	// at a time, ahead of the world, so that each minute's check, armed on
	// the minute, reads the minute it runs at.
	start := time.Now()
	var elapsed atomic.Int64
	clock := gameservertest.WithGameClockNow(func() time.Time { return start.Add(time.Duration(elapsed.Load())) })
	w, andreas, listener := rb7AndreasWorld(t, false, clock)
	minutes := func(n int) {
		for range n {
			elapsed.Add(int64(time.Minute))
			w.srv.Advance(t, time.Minute)
		}
	}
	me := w.player.ObjectID()
	w.heard.take()

	w.sendEvent(t, andreas, 1, 0, 0)
	if got := ind1Slot(andreas, script.SlotIntAI3); got != 1 {
		t.Fatalf("i_ai3 after event 1 = %d, want 1", got)
	}
	minutes(10)
	w.hit(t, andreas, 10)
	minutes(int(rb7AndreasStageQuietPeriod / time.Minute))
	if got := heardOf(w.heard.take(), 7); len(got) != 0 {
		t.Fatalf("event 7 heard 30 minutes after the hit = %q, want none", got)
	}
	minutes(1)
	if got, want := heardOf(w.heard.take(), 7), []string{heardEvent(7, 0)}; !slices.Equal(got, want) {
		t.Fatalf("event 7 heard 31 minutes after the hit = %q, want %q", got, want)
	}
	if d, ok := desire(andreas, actorai.IntentionAttack); ok {
		t.Fatalf("attack desire after the quiet half hour = %+v, want none", d)
	}

	w.clearDesires(t, andreas)
	w.sendEvent(t, andreas, 4, listener.ObjectID(), 0)
	if got, want := heardOf(w.heard.take(), 10020), []string{heardEvent(10020, listener.ObjectID())}; !slices.Equal(got, want) {
		t.Fatalf("event 10020 heard naming the listener = %q, want %q", got, want)
	}
	if d, ok := desire(andreas, actorai.IntentionAttack); ok {
		t.Fatalf("attack desire on a named NPC = %+v, want none", d)
	}
	w.sendEvent(t, andreas, 4, me, 0)
	if got, want := heardOf(w.heard.take(), 10020), []string{heardEvent(10020, me)}; !slices.Equal(got, want) {
		t.Fatalf("event 10020 heard naming the character = %q, want %q", got, want)
	}
	if d, ok := attackOn(andreas, w.player); !ok || d.Weight != 700 {
		t.Fatalf("attack desire on the named character = %+v (%v), want 700", d, ok)
	}

	w.clearDesires(t, andreas)
	w.sendEvent(t, andreas, 3, 0, 0)
	w.srv.Advance(t, 6700*time.Millisecond-time.Millisecond)
	w.wantCasts(t, "just before 6.7 s after event 3", andreas, nil)
	w.srv.Advance(t, time.Millisecond)
	w.wantCasts(t, "6.7 s after event 3", andreas, map[modelskill.ID]ind1Cast{rb7AltarSkill: {andreas.ObjectID(), 1000000}})

	w.hit(t, andreas, 10)
	w.sendEvent(t, andreas, 7, 0, 0)
	if got := ind1Slot(andreas, script.SlotIntAI3); got != 0 {
		t.Fatalf("i_ai3 after event 7 = %d, want 0", got)
	}
	if foe := andreasFoe(andreas); !script.Same(foe, script.NPCOf(andreas)) {
		t.Fatalf("c_ai0 after event 7 = %v, want Andreas itself", foe)
	}
	w.sendEvent(t, andreas, 2, 0, 0)
	w.sendEvent(t, andreas, 9, 0, 0)
	if got := ind1Slot(andreas, script.SlotIntAI3); got != 2 {
		t.Fatalf("i_ai3 after event 9 = %d, want 2", got)
	}
}

// TestAndreasAttacksWhatItSees ports Andreas's see-creature hook: seeing
// the character with an NPC as its foe, it notes the sight and attacks
// nothing; its own foe again after event 7, it attacks the character at
// 700.
func TestAndreasAttacksWhatItSees(t *testing.T) {
	t.Parallel()
	w, andreas, listener := rb7AndreasWorld(t, true)
	w.onQueue(t, listener, func() { andreas.TakeDamage(10, listener) })
	w.srv.Settle(t)
	if foe := andreasFoe(andreas); foe == nil || foe.ObjectID() != listener.ObjectID() {
		t.Fatalf("c_ai0 after a hit by the listener = %v, want the listener", foe)
	}
	if got := w.sees(t, andreas); got != 0 {
		t.Fatalf("attack desire on the character seen with the listener its foe = %v, want none", got)
	}
	if got := ind1Slot(andreas, script.SlotIntAI2); got != 1 {
		t.Fatalf("i_ai2 after seeing the character = %d, want 1", got)
	}
	w.sendEvent(t, andreas, 7, 0, 0)
	if got := w.sees(t, andreas); got != 700 {
		t.Fatalf("attack desire on the character seen with Andreas its own foe = %v, want 700", got)
	}
}

// TestAndreasWaitsOutAStage ports Andreas's no-desire hook and its 6005
// timer: at a stage, with nothing to do, it forgets the characters it saw
// and 5 s later attacks its foe, the character that hit it, at 700, and
// tells the NPCs within its seek range event 10002 with it.
func TestAndreasWaitsOutAStage(t *testing.T) {
	t.Parallel()
	w, andreas, _ := rb7AndreasWorld(t, false)
	me := w.player.ObjectID()
	w.hit(t, andreas, 10)
	w.sendEvent(t, andreas, 2, 0, 0)
	w.onQueue(t, andreas, func() {
		script.NPCOf(andreas).SetInt(script.SlotIntAI2, 1)
		andreas.AI().Desires().Clear()
	})
	w.heard.take()
	w.srv.AdvanceUntil(t, "i_ai2 forgotten", func() bool {
		w.ticks(t, 1)
		return ind1Slot(andreas, script.SlotIntAI2) == 0
	})
	w.srv.AdvanceUntil(t, "the attack desire on its foe", func() bool {
		_, ok := attackOn(andreas, w.player)
		return ok
	})
	if d, _ := attackOn(andreas, w.player); d.Weight != 700 {
		t.Fatalf("attack desire on its foe = %v, want 700", d.Weight)
	}
	if got := heardOf(w.heard.take(), 10002); len(got) == 0 || got[0] != heardEvent(10002, me) {
		t.Fatalf("event 10002 heard = %q, want %q", got, heardEvent(10002, me))
	}
}

// TestAndreasReportsItsDeath ports Andreas's dying hook: it runs maker
// script event 1000 on the altar's six combat npcmakers, then tells the
// NPCs within twice its broadcast range event 5 with its object id.
func TestAndreasReportsItsDeath(t *testing.T) {
	t.Parallel()
	w, andreas, _ := rb7AndreasWorld(t, false)
	w.makers.take()
	w.heard.take()
	w.die(t, andreas)
	if got, want := w.makers.take(), rb7MakerEvents("1000", rb7AltarMakers()...); !slices.Equal(got, want) {
		t.Fatalf("maker events as Andreas died = %q, want %q", got, want)
	}
	if got, want := heardOf(w.heard.take(), 5), []string{heardEvent(5, andreas.ObjectID())}; !slices.Equal(got, want) {
		t.Fatalf("event 5 heard as Andreas died = %q, want %q", got, want)
	}
}

// TestRaidBossType5HoldsItsPartysAttackers ports RaidBossType5's
// party-attacked and see-spell hooks: on a draw of 0 in 150, a hit on a
// private by the character, then a spell it sees the character cast, wants
// its ranged hold on the character at 1000000; a draw of 1 wants nothing.
// The character holding the hold's effect closes both, draw or not.
func TestRaidBossType5HoldsItsPartysAttackers(t *testing.T) {
	t.Parallel()
	w := bootRaid7(t, raidSpellBoot(t), rb5Leader(rb7Type5ID, 20, map[npc.SkillType]modelskill.ID{npc.SkillTypeRangeHoldA: rb7Hold}), rb5PrivateTemplate())
	w.draws.set(1)
	boss := w.spawn(t, rb7Type5ID, 400, 0)
	privates := privatesOf(t, boss, 2)
	hold := map[modelskill.ID]ind1Cast{rb7Hold: {w.player.ObjectID(), 1000000}}
	spell := func(when string, want map[modelskill.ID]ind1Cast) {
		t.Helper()
		w.castSelfBuff()
		if want == nil {
			w.ticks(t, 2)
		} else {
			w.srv.AdvanceUntil(t, "the ranged hold wanted", func() bool { return len(ind1Casts(boss)) > 0 })
		}
		w.wantCasts(t, when, boss, want)
	}

	w.hit(t, privates[0], 10)
	w.wantCasts(t, "after a hit on a private on a draw of 1", boss, nil)
	w.draws.set(1, 0)
	w.hit(t, privates[0], 10)
	w.wantCasts(t, "after a hit on a private on a draw of 0", boss, hold)

	w.clearDesires(t, boss)
	w.draws.set(1, 0)
	spell("after a spell seen on a draw of 0", hold)
	w.clearDesires(t, boss)
	w.draws.set(1)
	w.srv.Advance(t, 10*time.Second)
	spell("after a spell seen on a draw of 1", nil)

	w.ind5dLand(t, modelskill.Ref{ID: rb7Hold, Level: 1}, w.charOf(t))
	w.clearDesires(t, boss)
	w.draws.set(0)
	w.hit(t, privates[0], 10)
	w.wantCasts(t, "after a hit on a private with the character held", boss, nil)
	w.srv.Advance(t, 10*time.Second)
	spell("after a spell seen with the character held", nil)
}

// rb7Leader is a RaidBossType5 leading two privates of privateID, level
// 20 each; the privates walk, the leader does not.
func rb7Leader(privateID int32, privateSkills map[npc.SkillType]modelskill.ID) []*npc.Template {
	leader := rb5Leader(rb7Type5ID, 20, nil)
	for i := range leader.Privates {
		leader.Privates[i].NpcID = int(privateID)
	}
	private := wusTemplate(privateID, nil, privateSkills)
	private.Level = 20
	return []*npc.Template{leader, private}
}

// TestRaidPrivateArcherRunsFromACloseHit ports RaidPrivateArcher's
// created, attacked and timer hooks: a hit from 400 arms nothing; a hit
// from 50 has it want, 2 s later and not before, to run from the character
// as far as its drift range at 10000.
func TestRaidPrivateArcherRunsFromACloseHit(t *testing.T) {
	t.Parallel()
	w := bootRaid7(t, raidBoot{}, rb7Leader(rb7ArcherID, nil)...)
	w.draws.set(1)
	boss := w.spawn(t, rb7Type5ID, 400, 0)
	archer := privatesOf(t, boss, 2)[0]
	if got := ind1Slot(archer, script.SlotIntAI2); got != 0 {
		t.Fatalf("i_ai2 as the archer appeared = %d, want 0", got)
	}
	flees := func() (actorai.Desire, bool) { return desire(archer, actorai.IntentionFlee) }

	w.hit(t, archer, 10)
	w.srv.Advance(t, 2*time.Second)
	if d, ok := flees(); ok || ind1Slot(archer, script.SlotIntAI2) != 0 {
		t.Fatalf("after a hit from 400: flee desire %+v (%v), i_ai2 %d; want none, 0", d, ok, ind1Slot(archer, script.SlotIntAI2))
	}

	w.onQueue(t, archer, func() {
		script.NPCOf(archer).InstantTeleportTo(script.Loc{X: w.at.X + 50, Y: w.at.Y, Z: w.at.Z})
	})
	w.hit(t, archer, 10)
	if got := ind1Slot(archer, script.SlotIntAI2); got != 1 {
		t.Fatalf("i_ai2 after a hit from 50 = %d, want 1", got)
	}
	w.srv.Advance(t, 1900*time.Millisecond)
	if d, ok := flees(); ok {
		t.Fatalf("flee desire 1.9 s after the hit = %+v, want none", d)
	}
	w.srv.Advance(t, 100*time.Millisecond)
	d, ok := flees()
	if !ok || d.Weight != 10000 || d.FinalTarget == nil || d.FinalTarget.ObjectID() != w.player.ObjectID() {
		t.Fatalf("flee desire 2 s after the hit = %+v (%v), want from the character at 10000", d, ok)
	}
	if got := ind1Slot(archer, script.SlotIntAI2); got != 0 {
		t.Fatalf("i_ai2 after the run = %d, want 0", got)
	}
}

// rb7PrivateAnswers drives a raid private's party-attacked and see-spell
// hooks: with the character its top desire target and every draw 0, a hit
// on its fellow private by the character, then a spell it sees the
// character cast, wants skill at 1000000 on the character; with the
// listener its top desire target, or on draws of 1, nothing.
func rb7PrivateAnswers(t *testing.T, privateID int32, skillType npc.SkillType, skillID modelskill.ID) {
	t.Helper()
	w := bootRaid7(t, raidSpellBoot(t), rb7Leader(privateID, map[npc.SkillType]modelskill.ID{skillType: skillID})...)
	w.draws.set(1)
	boss := w.spawn(t, rb7Type5ID, 400, 0)
	privates := privatesOf(t, boss, 2)
	fellow, called := privates[0], privates[1]
	other := w.spawn(t, rb5ListenerID, -400, 0)
	strike := map[modelskill.ID]ind1Cast{skillID: {w.player.ObjectID(), 1000000}}
	hit := func(when string, top script.Creature, draw int, want map[modelskill.ID]ind1Cast) {
		t.Helper()
		w.setTop(t, called, top)
		w.draws.set(draw)
		w.hit(t, fellow, 10)
		w.wantCasts(t, when, called, want)
	}
	spell := func(when string, top script.Creature, draw int, want map[modelskill.ID]ind1Cast) {
		t.Helper()
		w.srv.Advance(t, 10*time.Second)
		w.setTop(t, called, top)
		w.draws.set(draw)
		w.castSelfBuff()
		if want == nil {
			w.ticks(t, 2)
		} else {
			w.srv.AdvanceUntil(t, "the special wanted", func() bool { return len(ind1Casts(called)) > 0 })
		}
		w.wantCasts(t, when, called, want)
	}

	hit("after a hit by its top desire target on a draw of 0", w.charOf(t), 0, strike)
	hit("after a hit by its top desire target on a draw of 1", w.charOf(t), 1, nil)
	hit("after a hit by another than its top desire target", script.NPCOf(other), 0, nil)
	spell("after a spell by its top desire target on a draw of 0", w.charOf(t), 0, strike)
	spell("after a spell by its top desire target on a draw of 1", w.charOf(t), 1, nil)
	spell("after a spell by another than its top desire target", script.NPCOf(other), 0, nil)
}

// TestRaidPrivateArcherShootsItsTopTarget ports RaidPrivateArcher's
// party-attacked and see-spell hooks: its long range physical special, one
// time in 20.
func TestRaidPrivateArcherShootsItsTopTarget(t *testing.T) {
	t.Parallel()
	rb7PrivateAnswers(t, rb7ArcherID, npc.SkillTypeLongRangePhysicalSpecialA, rb7LongRange)
}

// TestRaidPrivateFighterStrikesItsTopTarget ports RaidPrivateFighter's
// party-attacked and see-spell hooks: its physical special, one time in
// 15.
func TestRaidPrivateFighterStrikesItsTopTarget(t *testing.T) {
	t.Parallel()
	rb7PrivateAnswers(t, rb7FighterID, npc.SkillTypePhysicalSpecialA, rb7PhysicalA)
}
