package ai

import (
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"testing"
	"time"

	gamexml "github.com/fatal10110/acis_golang/internal/gameserver/data/xml"
	actorai "github.com/fatal10110/acis_golang/internal/gameserver/model/actor/ai"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/npc"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/script"
	"github.com/fatal10110/acis_golang/internal/gameserver/script/ai/defaultnpc"
	"github.com/fatal10110/acis_golang/internal/gameserver/script/ai/monster"
	"github.com/fatal10110/acis_golang/internal/gameserver/script/ai/raid"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
	"github.com/rs/zerolog"
)

// The batch's NPCs and the fixture skills its bosses list by type.
const (
	rb6SirraID        = int32(29056) // IceFairySirra
	rb6QueenGuardID   = int32(29057) // no behavior
	rb6Type3ID        = int32(25269) // RaidBossType3
	rb6AnaisID        = int32(29096) // RaidBossType3AnaisAggressive
	rb6Type4ID        = int32(25319) // RaidBossType4
	rb6PhysicalB      = modelskill.ID(9946)
	rb6Debuff         = modelskill.ID(9947)
	rb6DebuffAnother  = modelskill.ID(9948)
	rb6DDMagic        = modelskill.ID(9949)
	rb6SirraDebuff1   = modelskill.ID(4480)
	rb6SirraDebuff2   = modelskill.ID(4481)
	rb6SirraDebuff3   = modelskill.ID(4482)
	rb6AnaisHomeX     = 112800
	rb6AnaisHomeY     = -76160
	rb6CountdownMs    = 10000
	rb6SirraLifetime  = 34*time.Minute + 30*time.Second
	rb6SirraFirstNote = 5 * time.Minute
)

// rb6Batch is the raid chain in scripts.xml order with the batch's classes.
var rb6Batch = []struct {
	path string
	ctor func() script.Script
}{
	{"script.ai.individual.DefaultNpc", defaultnpc.New},
	{"script.ai.individual.Monster.MonsterAI", monster.New},
	{"script.ai.individual.Monster.RaidBoss.RaidBossStandard", raid.BossStandard},
	{"script.ai.individual.Monster.RaidBoss.RaidBossParty.IceFairySirra", raid.IceFairySirra},
	{"script.ai.individual.Monster.RaidBoss.RaidBossParty.RaidBossParty", raid.BossParty},
	{"script.ai.individual.Monster.RaidBoss.RaidBossParty.RaidBossType2", raid.BossType2},
	{"script.ai.individual.Monster.RaidBoss.RaidBossParty.RaidBossType3", raid.BossType3},
	{"script.ai.individual.Monster.RaidBoss.RaidBossParty.RaidBossType3AnaisAggressive", raid.BossType3AnaisAggressive},
	{"script.ai.individual.Monster.RaidBoss.RaidBossParty.RaidBossType4", raid.BossType4},
}

// rb6Spawnlist declares the two npcmakers of the Ice Queen's garden the
// Ice Fairy Sirra talks to, both probes.
const rb6Spawnlist = `<?xml version="1.0" encoding="utf-8"?>
<list>
	<territory name="rb6_garden" minZ="-100" maxZ="200"><node x="0" y="0"/><node x="400" y="0"/><node x="400" y="400"/><node x="0" y="400"/></territory>
	<npcmaker name="schuttgart13_npc2314_1m1" territory="rb6_garden" maximumNpcs="1">
		<ai type="rb5_probe"/>
	</npcmaker>
	<npcmaker name="schuttgart13_npc2314_3m1" territory="rb6_garden" maximumNpcs="1">
		<ai type="rb5_probe"/>
	</npcmaker>
</list>`

// rb6Skills are the skills the batch's bosses cast beside the raid boss
// fixture skills: Type 3's physical special, Type 4's two ranged debuffs
// around itself and its DD magic, the ranged DD magic of Type 2 and Sirra,
// and Sirra's three debuffs. Type 4's first debuff and Sirra's third carry
// an effect of their own stack, which a test lands to close their gates.
func rb6Skills() []modelskill.Definition {
	return append(rbSkills(),
		ind1Skill(rb5RangeDD, 600, false), ind1Skill(rb6PhysicalB, 600, false),
		rb6Stacked(ind1Skill(rb6Debuff, 0, true), "rb6_debuff"), ind1Skill(rb6DebuffAnother, 0, true), ind1Skill(rb6DDMagic, 600, false),
		ind1Skill(rb6SirraDebuff1, 0, true), ind1Skill(rb6SirraDebuff2, 0, true), rb6Stacked(ind1Skill(rb6SirraDebuff3, 600, false), "rb6_sirra_debuff3"),
		rb6SirraDebuff3Order0(),
	)
}

// rb6SirraDebuff3Order0 is a level 2 of Sirra's third debuff whose effect
// stacks at order 0: a guard holding it holds the debuff at level 0.
func rb6SirraDebuff3Order0() modelskill.Definition {
	d := rb6Stacked(ind1Skill(rb6SirraDebuff3, 600, false), "rb6_sirra_debuff3")
	d.Level, d.Effects[0].StackOrder = 2, 0
	return d
}

// rb6Stacked gives d one 60 s effect of stack.
func rb6Stacked(d modelskill.Definition, stack string) modelskill.Definition {
	d.Effects = []modelskill.EffectTemplate{{Name: "Debuff", Time: 60, Icon: true, StackType: stack, StackOrder: 1}}
	return d
}

// bootRaid6 boots the batch beside templates and a listener, every NPC
// casting rb6Skills and the raid curse, every draw 0 until a test queues
// others, with the probe npcmakers of rb6Spawnlist.
func bootRaid6(t *testing.T, extra raidBoot, templates ...*npc.Template) *rb5World {
	t.Helper()
	var list []script.Listing
	catalog := script.Catalog{}
	for _, c := range rb6Batch {
		list = append(list, script.Listing{Path: c.path})
		catalog[c.path] = c.ctor
	}
	heard := &ind1Heard{}
	const listener, director = "test.Rb6Listener", "test.Ind1Director"
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
	if err := os.WriteFile(filepath.Join(dir, "rb6.xml"), []byte(rb6Spawnlist), 0o644); err != nil {
		t.Fatalf("write spawnlist: %v", err)
	}
	table, err := gamexml.LoadSpawnlist(dir, zerolog.Nop(), 1)
	if err != nil {
		t.Fatalf("load spawnlist: %v", err)
	}
	makers := &rb5Makers{}
	draws := &ind1Draws{}
	skills := rb6Skills()
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

// TestRaidBossType3StrikesItsTopTarget ports RaidBossType3's party-attacked
// and see-spell hooks. The first hit on a private finds the boss with no
// top desire target: only RaidBossParty's 7407 follows. The next, by the
// character now its top desire target, on a draw of 0 in 15 wants its
// physical special on the character at 1000000. A spell cast by its top
// desire target does the same; one cast while another creature is its top
// desire target does nothing.
func TestRaidBossType3StrikesItsTopTarget(t *testing.T) {
	t.Parallel()
	skills := map[npc.SkillType]modelskill.ID{npc.SkillTypePhysicalSpecialB: rb6PhysicalB}
	w := bootRaid6(t, raidSpellBoot(t), rb5Leader(rb6Type3ID, 20, skills), rb5PrivateTemplate())
	w.draws.set(1, 0)
	boss := w.spawn(t, rb6Type3ID, 400, 0)
	privates := privatesOf(t, boss, 2)
	me := w.player.ObjectID()

	w.hit(t, privates[0], 10)
	if got := w.attackWeight(boss); got != 7407 {
		t.Fatalf("attack desire after the first hit = %v, want 7407", got)
	}
	w.wantCasts(t, "after the first hit, no top desire target", boss, nil)
	w.draws.set(1, 0)
	w.hit(t, privates[0], 10)
	w.wantCasts(t, "after a hit by its top desire target", boss, map[modelskill.ID]ind1Cast{rb6PhysicalB: {me, 1000000}})

	w.setTop(t, boss, w.charOf(t))
	w.draws.set(0)
	w.castSelfBuff()
	w.srv.AdvanceUntil(t, "the physical special wanted", func() bool {
		_, ok := desire(boss, actorai.IntentionCast)
		return ok
	})
	w.wantCasts(t, "after a spell by its top desire target", boss, map[modelskill.ID]ind1Cast{rb6PhysicalB: {me, 1000000}})

	other := w.spawn(t, rb5ListenerID, -400, 0)
	w.srv.Advance(t, time.Second)
	w.setTop(t, boss, script.NPCOf(other))
	w.castSelfBuff()
	w.ticks(t, 2)
	w.wantCasts(t, "after a spell by another than its top desire target", boss, nil)
}

// TestRaidBossType4DebuffsAroundItself ports RaidBossType4's party-attacked
// and see-spell hooks, with the boss 100 from the character and the
// listener, its top desire target, 100 on its other side. With no top
// desire target a hit on a private wants nothing of its own. A hit by the
// character, not its top desire target, on draws of 0 wants both its
// ranged debuffs around itself and its DD magic on the character, each at
// 1000000, neither creature holding a debuff's effect; a spell the
// character casts does the same. A hit by its top desire target wants only
// the DD magic.
func TestRaidBossType4DebuffsAroundItself(t *testing.T) {
	t.Parallel()
	skills := map[npc.SkillType]modelskill.ID{
		npc.SkillTypeSelfRangeDebuffA: rb6Debuff, npc.SkillTypeSelfRangeDebuffAnotherA: rb6DebuffAnother, npc.SkillTypeDDMagicA: rb6DDMagic,
	}
	w := bootRaid6(t, raidSpellBoot(t), rb5Leader(rb6Type4ID, 20, skills), rb5PrivateTemplate())
	w.draws.set(0)
	boss := w.spawn(t, rb6Type4ID, 100, 0)
	privates := privatesOf(t, boss, 2)
	top := w.spawn(t, rb5ListenerID, 200, 0)
	me := w.player.ObjectID()
	all := map[modelskill.ID]ind1Cast{
		rb6Debuff: {boss.ObjectID(), 1000000}, rb6DebuffAnother: {boss.ObjectID(), 1000000}, rb6DDMagic: {me, 1000000},
	}

	w.hit(t, privates[0], 10)
	w.wantCasts(t, "after a hit with no top desire target", boss, nil)

	w.setTop(t, boss, script.NPCOf(top))
	w.hit(t, privates[0], 10)
	w.wantCasts(t, "after a hit by another than its top desire target", boss, all)

	w.setTop(t, boss, script.NPCOf(top))
	w.castSelfBuff()
	w.srv.AdvanceUntil(t, "the debuffs wanted", func() bool { return len(ind1Casts(boss)) == 3 })
	w.wantCasts(t, "after a spell by another than its top desire target", boss, all)

	w.setTop(t, boss, w.charOf(t))
	w.hit(t, privates[0], 10)
	w.wantCasts(t, "after a hit by its top desire target", boss, map[modelskill.ID]ind1Cast{rb6DDMagic: {me, 1000000}})
}

// TestRaidBossType4DebuffGates ports the gates of RaidBossType4's
// debuffs, the boss 100 from the character, a hit on a private by the
// character, its top desire target another listener. A draw of 1 in 2 and
// 0 in 5 wants only the other debuff (and the DD magic on 0 in 75); 0 in 2
// and 1 in 5 only the first, with no DD magic on 1 in 75. A top desire
// target 300 from the boss leaves only the DD magic. The first debuff's
// effect held by the top desire target, then by the character, closes that
// debuff alone, without its draw.
func TestRaidBossType4DebuffGates(t *testing.T) {
	t.Parallel()
	skills := map[npc.SkillType]modelskill.ID{
		npc.SkillTypeSelfRangeDebuffA: rb6Debuff, npc.SkillTypeSelfRangeDebuffAnotherA: rb6DebuffAnother, npc.SkillTypeDDMagicA: rb6DDMagic,
	}
	w := bootRaid6(t, raidBoot{}, rb5Leader(rb6Type4ID, 20, skills), rb5PrivateTemplate())
	w.draws.set(1)
	boss := w.spawn(t, rb6Type4ID, 100, 0)
	privates := privatesOf(t, boss, 2)
	near := w.spawn(t, rb5ListenerID, 200, 0)
	other := w.spawn(t, rb5ListenerID, 200, 50)
	far := w.spawn(t, rb5ListenerID, 400, 0)
	me, self := w.player.ObjectID(), boss.ObjectID()
	hitWith := func(top *npc.Hostile, draws ...int) {
		w.setTop(t, boss, script.NPCOf(top))
		w.draws.set(1, draws...)
		w.hit(t, privates[0], 10)
	}

	hitWith(near, 1, 0, 0)
	w.wantCasts(t, "on draws 1, 0, 0", boss, map[modelskill.ID]ind1Cast{rb6DebuffAnother: {self, 1000000}, rb6DDMagic: {me, 1000000}})
	hitWith(near, 0, 1, 1)
	w.wantCasts(t, "on draws 0, 1, 1", boss, map[modelskill.ID]ind1Cast{rb6Debuff: {self, 1000000}})
	hitWith(far, 0, 0, 0)
	w.wantCasts(t, "with the top desire target 300 away", boss, map[modelskill.ID]ind1Cast{rb6DDMagic: {me, 1000000}})

	held := modelskill.Ref{ID: rb6Debuff, Level: 1}
	w.ind5dLand(t, held, script.NPCOf(near))
	hitWith(near, 0, 0)
	w.wantCasts(t, "with the top desire target holding the first debuff", boss, map[modelskill.ID]ind1Cast{rb6DebuffAnother: {self, 1000000}, rb6DDMagic: {me, 1000000}})
	w.ind5dLand(t, held, w.charOf(t))
	hitWith(other, 0, 0)
	w.wantCasts(t, "with the character holding the first debuff", boss, map[modelskill.ID]ind1Cast{rb6DebuffAnother: {self, 1000000}, rb6DDMagic: {me, 1000000}})
}

// TestAnaisAttacksWhatItSeesInItsTerritory ports Anais's see-creature hook:
// seeing the character while it stands in its territory, it wants to attack
// it at 200; out of its territory, nothing.
func TestAnaisAttacksWhatItSeesInItsTerritory(t *testing.T) {
	t.Parallel()
	w := bootRaid6(t, raidBoot{}, rb5Seer(rb6AnaisID))
	w.draws.set(1)
	anais := w.spawn(t, rb6AnaisID, 400, 0)
	if got := w.sees(t, anais); got != 200 {
		t.Fatalf("attack desire on the character seen in territory = %v, want 200", got)
	}
	w.teleport(t, anais, 3400, 0)
	if script.NPCOf(anais).InMyTerritory() {
		t.Fatal("setup: Anais is still in its territory")
	}
	if got := w.sees(t, anais); got != 0 {
		t.Fatalf("attack desire on the character seen out of territory = %v, want none", got)
	}
}

// atAnaisHome reports whether h stands at Anais's home.
func atAnaisHome(h *npc.Hostile) bool {
	at := positionOf(h)
	return at.X == rb6AnaisHomeX && at.Y == rb6AnaisHomeY
}

// TestAnaisGoesHomeWhenHitOutOfItsTerritory ports Anais's attacked and
// timer hooks: hit in its territory, it stays; hit out of it, it goes back
// to its home and arms its 1 s timer, which then drops its attack desires
// and its hate, here a desire on the listener standing beside its home.
func TestAnaisGoesHomeWhenHitOutOfItsTerritory(t *testing.T) {
	t.Parallel()
	w := bootRaid6(t, raidBoot{}, rb5Boss(rb6AnaisID, nil))
	w.draws.set(1)
	anais := w.spawn(t, rb6AnaisID, 400, 0)
	w.hit(t, anais, 10)
	if got, want := w.attackWeight(anais), rbHate(10, 20); got != want || atAnaisHome(anais) {
		t.Fatalf("hit in territory: attack desire %v (want %v), at home %v (want not)", got, want, atAnaisHome(anais))
	}
	beside := w.spawnAt(t, rb5ListenerID, location.Location{X: rb6AnaisHomeX + 100, Y: rb6AnaisHomeY, Z: w.at.Z})

	w.teleport(t, anais, 3400, 0)
	w.hit(t, anais, 10)
	if !atAnaisHome(anais) {
		t.Fatalf("Anais stands at %v after a hit out of territory, want its home", positionOf(anais))
	}
	w.onQueue(t, anais, func() { script.NPCOf(anais).AddAttackDesire(script.NPCOf(beside), 1000) })
	w.srv.Advance(t, 900*time.Millisecond)
	if _, ok := attackOn(anais, beside); !ok {
		t.Fatal("attack desire on the listener dropped before the timer, want it kept")
	}
	w.srv.Advance(t, 100*time.Millisecond)
	if d, ok := desire(anais, actorai.IntentionAttack); ok {
		t.Fatalf("attack desire 1 s after the hit = %+v, want none", d)
	}
	if got := anais.AggroHate(beside); got != 0 {
		t.Fatalf("hate on the listener 1 s after the hit = %v, want none", got)
	}
}

// TestAnaisGoesHomeWithNothingToDo ports Anais's no-desire hook: left with
// nothing to do out of its territory, it goes back to its home.
func TestAnaisGoesHomeWithNothingToDo(t *testing.T) {
	t.Parallel()
	w := bootRaid6(t, raidBoot{}, rb5Boss(rb6AnaisID, nil))
	w.draws.set(1)
	anais := w.spawn(t, rb6AnaisID, 400, 0)
	w.ticks(t, 2)
	if atAnaisHome(anais) {
		t.Fatal("Anais went home from inside its territory")
	}
	w.teleport(t, anais, 3400, 0)
	w.clearDesires(t, anais)
	w.srv.AdvanceUntil(t, "Anais at home", func() bool {
		w.ticks(t, 1)
		return atAnaisHome(anais)
	})
}

// TestAnaisGoesHomeAfterACastOutOfItsTerritory ports Anais's skill-finished
// hook: a cast of its own that ends out of its territory sends it back to
// its home, as it ends; one that ends inside does not.
func TestAnaisGoesHomeAfterACastOutOfItsTerritory(t *testing.T) {
	t.Parallel()
	w := bootRaid6(t, raidBoot{}, rb5Boss(rb6AnaisID, map[npc.SkillType]modelskill.ID{npc.SkillTypeSelfBuffA: rbSelfBuff}))
	w.draws.set(1)
	anais := w.spawn(t, rb6AnaisID, 400, 0)
	w.castOnce(t, anais, rbSelfBuff)
	if atAnaisHome(anais) {
		t.Fatal("Anais went home after a cast inside its territory")
	}
	// Out of its territory, a do-nothing desire under the cast keeps it
	// from idling, so its no-desire hook stays out of the way.
	w.teleport(t, anais, 3400, 0)
	w.onQueue(t, anais, func() {
		n := script.NPCOf(anais)
		n.AddDoNothingDesire(60, 500000)
		n.AddCastDesire(n, modelskill.Ref{ID: rbSelfBuff, Level: 1}, 1000000)
	})
	if atAnaisHome(anais) {
		t.Fatal("setup: Anais went home before its cast")
	}
	w.castQueued(t, anais)
	if !atAnaisHome(anais) {
		t.Fatalf("Anais stands at %v after a cast out of territory, want its home", positionOf(anais))
	}
}

// TestAnaisAnswersItsPartyOnlyFromItsTerritory ports Anais's party-attacked
// hook. A private stands in its master's territory exactly when its master
// does. A hit on a private with Anais in its territory goes on to
// RaidBossParty (7407). With Anais out of it, caller and called both out,
// Anais goes back to its home and the hit gets no further answer.
func TestAnaisAnswersItsPartyOnlyFromItsTerritory(t *testing.T) {
	t.Parallel()
	w := bootRaid6(t, raidBoot{}, rb5Leader(rb6AnaisID, 20, nil), rb5PrivateTemplate())
	w.draws.set(1)
	anais := w.spawn(t, rb6AnaisID, 400, 0)
	privates := privatesOf(t, anais, 2)
	w.hit(t, privates[0], 10)
	if got := w.attackWeight(anais); got != 7407 || atAnaisHome(anais) {
		t.Fatalf("hit on a private in territory: attack desire %v (want 7407), at home %v (want not)", got, atAnaisHome(anais))
	}

	w.clearDesires(t, anais)
	w.teleport(t, anais, 3400, 0)
	if script.NPCOf(privates[1]).InMyTerritory() || script.NPCOf(anais).InMyTerritory() {
		t.Fatal("setup: want Anais and its private out of the territory")
	}
	w.hit(t, privates[1], 10)
	if !atAnaisHome(anais) {
		t.Fatalf("Anais stands at %v after a hit on its party out of territory, want its home", positionOf(anais))
	}
	if d, ok := desire(anais, actorai.IntentionAttack); ok {
		t.Fatalf("attack desire after a hit on its party out of territory = %+v, want none", d)
	}
}

// TestIceFairySirraLeavesOnItsClock ports IceFairySirra's created and timer
// hooks: 34.5 minutes after it appears, not before, it runs maker script
// event 10005 on schuttgart13_npc2314_1m1 and leaves the world.
func TestIceFairySirraLeavesOnItsClock(t *testing.T) {
	t.Parallel()
	w := bootRaid6(t, raidBoot{}, rb5Boss(rb6SirraID, nil))
	w.draws.set(1)
	sirra := w.spawn(t, rb6SirraID, 400, 0)
	w.makers.take()
	w.srv.Advance(t, rb6SirraLifetime-time.Second)
	if !w.inWorld(sirra) {
		t.Fatal("Sirra left the world before its time")
	}
	if got := w.makers.take(); len(got) != 0 {
		t.Fatalf("maker events before its time = %q, want none", got)
	}
	w.srv.Advance(t, time.Second)
	if got, want := w.makers.take(), []string{"schuttgart13_npc2314_1m1 10005"}; !slices.Equal(got, want) {
		t.Fatalf("maker events at its time = %q, want %q", got, want)
	}
	if w.inWorld(sirra) {
		t.Fatal("Sirra stays in the world past its time")
	}
}

// TestIceFairySirraFollowsTheQueensPhases ports IceFairySirra's event 11036
// and see-creature hooks, with the Ice Queen's guard beside it. Phase 1
// wants its first debuff on itself, phase 2 its second, each at 1000000;
// seeing the guard in phase 2 wants nothing of it. Phase 3 has it look
// around: it wants its third debuff on the guard, which holds none, and to
// attack the character in its territory at 200. Seen again holding the
// third debuff at level 0, the guard still draws it; at level 1, nothing.
// Phase 0 ends it.
func TestIceFairySirraFollowsTheQueensPhases(t *testing.T) {
	t.Parallel()
	w := bootRaid6(t, raidBoot{}, rb5Boss(rb6SirraID, nil), rb5PrivateTemplate(), rb5Boss(rb6QueenGuardID, nil))
	w.draws.set(1)
	sirra := w.spawn(t, rb6SirraID, 100, 0)
	guard := w.spawn(t, rb6QueenGuardID, 200, 0)
	self := sirra.ObjectID()
	// look runs Sirra's look around half a minute after its last one, its
	// desires dropped first.
	look := func() {
		w.srv.Advance(t, 30*time.Second)
		w.clearDesires(t, sirra)
		w.onQueue(t, sirra, func() { script.NPCOf(sirra).LookNeighbor() })
	}

	w.sendEvent(t, sirra, 11036, 1, 0)
	if got := ind1Slot(sirra, script.SlotIntAI0); got != 1 {
		t.Fatalf("i_ai0 after phase 1 = %d, want 1", got)
	}
	w.wantCasts(t, "in phase 1", sirra, map[modelskill.ID]ind1Cast{rb6SirraDebuff1: {self, 1000000}})
	w.clearDesires(t, sirra)
	w.sendEvent(t, sirra, 11036, 2, 0)
	w.wantCasts(t, "in phase 2", sirra, map[modelskill.ID]ind1Cast{rb6SirraDebuff2: {self, 1000000}})
	look()
	w.wantCasts(t, "seeing the guard in phase 2", sirra, nil)

	w.srv.Advance(t, 30*time.Second)
	w.clearDesires(t, sirra)
	w.sendEvent(t, sirra, 11036, 3, 0)
	if got := ind1Slot(sirra, script.SlotIntAI0); got != 3 {
		t.Fatalf("i_ai0 after phase 3 = %d, want 3", got)
	}
	w.wantCasts(t, "in phase 3", sirra, map[modelskill.ID]ind1Cast{rb6SirraDebuff3: {guard.ObjectID(), 1000000}})
	if d, ok := attackOn(sirra, w.player); !ok || d.Weight != 200 {
		t.Fatalf("attack desire on the character seen in phase 3 = %+v (%v), want 200", d, ok)
	}

	w.ind5dLand(t, modelskill.Ref{ID: rb6SirraDebuff3, Level: 2}, script.NPCOf(guard))
	look()
	w.wantCasts(t, "seeing the guard holding the third debuff at level 0 in phase 3", sirra, map[modelskill.ID]ind1Cast{rb6SirraDebuff3: {guard.ObjectID(), 1000000}})
	w.ind5dLand(t, modelskill.Ref{ID: rb6SirraDebuff3, Level: 1}, script.NPCOf(guard))
	look()
	w.wantCasts(t, "seeing the guard holding the third debuff at level 1 in phase 3", sirra, nil)

	w.sendEvent(t, sirra, 11036, 0, 0)
	if got := ind1Slot(sirra, script.SlotIntAI0); got != 0 {
		t.Fatalf("i_ai0 after phase 0 = %d, want 0", got)
	}
}

// TestIceFairySirraCountsDownToTheParty ports IceFairySirra's event 11040
// and its countdown timers: named the character, whose party holds Mate,
// it shows that party the time left 5 minutes later (NpcString 1010643,
// 30), 10 minutes after that (20), and 15 minutes after that the end
// (1121002), each for 10 s.
func TestIceFairySirraCountsDownToTheParty(t *testing.T) {
	t.Parallel()
	w := bootRaid6(t, raidBoot{}, rb5Boss(rb6SirraID, nil))
	w.draws.set(1)
	sirra := w.spawn(t, rb6SirraID, 400, 0)
	w.joinMate(t)
	w.sendEvent(t, sirra, 11040, w.player.ObjectID(), 0)
	if got := script.NPCOf(sirra).Slot(script.SlotCreature0); got == nil || got.ObjectID() != w.player.ObjectID() {
		t.Fatalf("c_ai0 after event 11040 = %v, want the character", got)
	}
	note := func(text string) string { return fmt.Sprintf("%s@%d", text, rb6CountdownMs) }
	for _, step := range []struct {
		after time.Duration
		want  string
	}{
		{rb6SirraFirstNote, note(script.NpcString(1010643, "30"))},
		{10 * time.Minute, note(script.NpcString(1010643, "20"))},
		{15 * time.Minute, note(script.NpcString(1121002))},
	} {
		frames := w.advanceFrames(t, step.after-time.Second)
		if got := screenTexts(frames); len(got) != 0 {
			t.Fatalf("screen messages a second before %q = %q, want none", step.want, got)
		}
		frames = w.advanceFrames(t, time.Second)
		if got := screenTexts(frames); !slices.Equal(got, []string{step.want}) {
			t.Fatalf("screen messages = %q, want %q", got, step.want)
		}
	}
}

// TestIceFairySirraTellsTheGardenAsItDies ports IceFairySirra's dying hook:
// it runs maker script event 10025 on schuttgart13_npc2314_1m1, then on
// schuttgart13_npc2314_3m1.
func TestIceFairySirraTellsTheGardenAsItDies(t *testing.T) {
	t.Parallel()
	w := bootRaid6(t, raidBoot{}, rb5Boss(rb6SirraID, nil))
	w.draws.set(1)
	sirra := w.spawn(t, rb6SirraID, 400, 0)
	w.makers.take()
	w.die(t, sirra)
	if got, want := w.makers.take(), []string{"schuttgart13_npc2314_1m1 10025", "schuttgart13_npc2314_3m1 10025"}; !slices.Equal(got, want) {
		t.Fatalf("maker events as Sirra died = %q, want %q", got, want)
	}
}

// TestIceFairySirraStrikesItsPartysAttackers ports IceFairySirra's
// party-attacked and see-spell hooks: on a draw of 0 in 50 a hit on a
// private by the character, then a spell it sees the character cast,
// wants its ranged DD magic on the character at 1000000; RaidBossType2's
// own one time in 150 draws 1.
func TestIceFairySirraStrikesItsPartysAttackers(t *testing.T) {
	t.Parallel()
	skills := map[npc.SkillType]modelskill.ID{npc.SkillTypeRangeDDMagicA: rb5RangeDD}
	w := bootRaid6(t, raidSpellBoot(t), rb5Leader(rb6SirraID, 20, skills), rb5PrivateTemplate())
	w.draws.set(1)
	sirra := w.spawn(t, rb6SirraID, 400, 0)
	privates := privatesOf(t, sirra, 2)
	me := w.player.ObjectID()

	w.draws.set(1, 0)
	w.hit(t, privates[0], 10)
	w.wantCasts(t, "after a hit on a private", sirra, map[modelskill.ID]ind1Cast{rb5RangeDD: {me, 1000000}})

	w.clearDesires(t, sirra)
	w.draws.set(1, 0)
	w.castSelfBuff()
	w.srv.AdvanceUntil(t, "the ranged DD magic wanted", func() bool {
		_, ok := desire(sirra, actorai.IntentionCast)
		return ok
	})
	w.wantCasts(t, "after the spell seen", sirra, map[modelskill.ID]ind1Cast{rb5RangeDD: {me, 1000000}})
}
