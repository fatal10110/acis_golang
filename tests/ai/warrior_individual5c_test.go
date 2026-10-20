package ai

import (
	"encoding/binary"
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
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/script"
	"github.com/fatal10110/acis_golang/internal/gameserver/script/ai/warrior"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
	"github.com/rs/zerolog"
)

// The NPC ids of the fifth individual behavior batch's third PR the tests
// spawn.
const (
	ind5cHereticBombID    = int32(22145) // RoyalRushHereticBomb
	ind5cHereticBombTwoID = int32(22154) // RoyalRushHereticBomb
	ind5cHereticBomb2ID   = int32(22195) // RoyalRushHereticBomb2
	ind5cRoomBoss1ID      = int32(18121) // RoyalRushRoomBoss1
	ind5cRoomBoss2ID      = int32(18120) // RoyalRushRoomBoss2
	ind5cRoomBoss4ID      = int32(18141) // RoyalRushRoomBoss4
	ind5cRoomBoss5ID      = int32(18122) // RoyalRushRoomBoss5
	ind5cTyrannoID        = int32(22215) // TyrannoPrimeval
)

// The skills the batch's fixture NPCs list by type.
const (
	ind5cSpecial1    = modelskill.ID(9360) // PHYSICAL_SPECIAL1
	ind5cRangeDD     = modelskill.ID(9361) // RANGE_DD_MAGIC1
	ind5cSelfDebuff1 = modelskill.ID(9362) // SELF_DEBUFF1
	ind5cSelfDebuff2 = modelskill.ID(9363) // SELF_DEBUFF2
	ind5cSelfDebuff3 = modelskill.ID(9364) // SELF_DEBUFF3
	ind5cClanBuff    = modelskill.ID(9365) // CLAN_BUFF1
	ind5cSelfRange   = modelskill.ID(9366) // SELF_RANGE_DD_MAGIC1
	ind5cDebuff      = modelskill.ID(9367) // DEBUFF
	ind5cDDMagic     = modelskill.ID(9368) // DD_MAGIC
	ind5cSelfBuff1   = modelskill.ID(9370) // SELF_BUFF1
	ind5cSelfBuff2   = modelskill.ID(9371) // SELF_BUFF2
	ind5cLongRange   = modelskill.ID(9372) // LONG_RANGE_DD_MAGIC1
	ind5cDebuff1     = modelskill.ID(9376) // DEBUFF1
	ind5cDebuff2     = modelskill.ID(9377) // DEBUFF2
	ind5cCancelA     = modelskill.ID(9378) // CAPTURE_CANCEL_A
	ind5cCancelB     = modelskill.ID(9379) // CAPTURE_CANCEL_B
	ind5cCancelC     = modelskill.ID(9380) // CAPTURE_CANCEL_C
	ind5cCancelAll   = modelskill.ID(9381) // CAPTURE_CANCEL_ALL
)

// The capture skills a player casts on a tyrannosaurus.
const (
	ind5cCaptureA = modelskill.ID(5106)
	ind5cCaptureB = modelskill.ID(5107)
)

// ind5cTrapFailed is the system message of a failed capture.
const ind5cTrapFailed = 2002

// ind5cBatch are the classes of the PR, in scripts.xml order.
var ind5cBatch = []struct {
	path string
	ctor func() script.Script
}{
	{"script.ai.individual.Monster.WarriorBase.Warrior.RoyalRushBomb.RoyalRushHereticBomb", warrior.RoyalRushHereticBomb},
	{"script.ai.individual.Monster.WarriorBase.Warrior.RoyalRushBomb.RoyalRushHereticBomb2", warrior.RoyalRushHereticBomb2},
	{"script.ai.individual.Monster.WarriorBase.Warrior.RoyalRushRoomBossBasic.RoyalRushRoomBoss1", warrior.RoyalRushRoomBoss1},
	{"script.ai.individual.Monster.WarriorBase.Warrior.RoyalRushRoomBossBasic.RoyalRushRoomBoss2", warrior.RoyalRushRoomBoss2},
	{"script.ai.individual.Monster.WarriorBase.Warrior.RoyalRushRoomBossBasic.RoyalRushRoomBoss4", warrior.RoyalRushRoomBoss4},
	{"script.ai.individual.Monster.WarriorBase.Warrior.RoyalRushRoomBossBasic.RoyalRushRoomBoss5", warrior.RoyalRushRoomBoss5},
	{"script.ai.individual.Monster.WarriorBase.Warrior.RoyalRushRoomBossBasic.RoyalRushRoomBossBasic", warrior.RoyalRushRoomBossBasic},
	{"script.ai.individual.Monster.WarriorBase.Warrior.TyrannoPrimeval.TyrannoPrimeval", warrior.TyrannoPrimeval},
}

// ind5cCaptureDef is capture skill id at level: an effect of its own stack
// type lasting a minute, its stack order the level.
func ind5cCaptureDef(id modelskill.ID, level int) modelskill.Definition {
	d := ind1Skill(id, 600, false)
	d.Level = level
	d.Effects = []modelskill.EffectTemplate{{Name: "Debuff", Time: 60, Icon: true, StackType: fmt.Sprintf("ind5c_capture_%d", id), StackOrder: float64(level)}}
	return d
}

// ind5cSkills are the skills the batch's fixture NPCs cast: the self
// skills self buffs, the clan buff a one-target buff and the others
// one-target magic, both of reach 600.
func ind5cSkills() *modelskill.Table {
	var defs []modelskill.Definition
	for _, id := range []modelskill.ID{ind5cSpecial1, ind5cRangeDD, ind5cDebuff, ind5cDDMagic, ind5cLongRange} {
		defs = append(defs, ind1Skill(id, 600, false))
	}
	for _, id := range []modelskill.ID{
		ind5cSelfDebuff1, ind5cSelfDebuff2, ind5cSelfDebuff3, ind5cSelfRange, ind5cSelfBuff1, ind5cSelfBuff2,
		ind5cDebuff1, ind5cDebuff2, ind5cCancelA, ind5cCancelB, ind5cCancelC, ind5cCancelAll,
	} {
		defs = append(defs, ind1Skill(id, 0, true))
	}
	clanBuff := ind1Skill(ind5cClanBuff, 600, false)
	clanBuff.Offensive, clanBuff.Magic, clanBuff.SkillType, clanBuff.Power = false, false, "BUFF", 0
	return modelskill.NewTable(append(defs, clanBuff))
}

// ind5cRoomBossTemplate is a fixture room boss of id with ai, listing the
// room bosses' skills by type, in the scenario clan of range 500.
func ind5cRoomBossTemplate(id int32, ai npc.AIParams) *npc.Template {
	tmpl := warriorTemplate(id, ai)
	tmpl.SkillsByType = map[npc.SkillType]modelskill.Ref{
		npc.SkillTypePhysicalSpecial1:  {ID: ind5cSpecial1, Level: 1},
		npc.SkillTypeRangeDDMagic1:     {ID: ind5cRangeDD, Level: 1},
		npc.SkillTypeSelfDebuff1:       {ID: ind5cSelfDebuff1, Level: 1},
		npc.SkillTypeSelfDebuff2:       {ID: ind5cSelfDebuff2, Level: 1},
		npc.SkillTypeSelfDebuff3:       {ID: ind5cSelfDebuff3, Level: 1},
		npc.SkillTypeClanBuff1:         {ID: ind5cClanBuff, Level: 1},
		npc.SkillTypeSelfRangeDDMagic1: {ID: ind5cSelfRange, Level: 1},
		npc.SkillTypeDebuff:            {ID: ind5cDebuff, Level: 1},
		npc.SkillTypeDDMagic:           {ID: ind5cDDMagic, Level: 1},
	}
	return ind5cClan(tmpl)
}

// ind5cClan puts tmpl in the scenario clan, of range 500.
func ind5cClan(tmpl *npc.Template) *npc.Template {
	tmpl.Clans, tmpl.ClanRange = []string{"ind5c_clan"}, 500
	return tmpl
}

// ind5cTyrannoTemplate is a fixture tyrannosaurus with ai listing the
// skills of types by type.
func ind5cTyrannoTemplate(ai npc.AIParams, types map[npc.SkillType]modelskill.ID) *npc.Template {
	tmpl := warriorTemplate(ind5cTyrannoID, ai)
	tmpl.SkillsByType = map[npc.SkillType]modelskill.Ref{}
	for t, id := range types {
		tmpl.SkillsByType[t] = modelskill.Ref{ID: id, Level: 1}
	}
	return tmpl
}

// bootIndividual5c boots the PR's classes after the Warrior chain beside
// templates and a Key Box, every NPC casting the batch's skills, with the
// listener behavior on ind1ListenerID and the director script, every draw
// 0 until a scenario queues others.
func bootIndividual5c(t *testing.T, templates ...*npc.Template) *ind1World {
	t.Helper()
	list, catalog := warriorScripts()
	for _, c := range ind5cBatch {
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
	kinds := map[int32]script.NPCKind{}
	for _, tmpl := range templates {
		kinds[int32(tmpl.ID)] = script.KindHostile
	}
	draws := &ind1Draws{}
	w := bootRaid(t, raidBoot{opts: []gameservertest.Option{
		gameservertest.WithScriptRand(draws.rand),
		gameservertest.WithNPCScripts(kinds, list, catalog),
		gameservertest.WithNpcSpawnSkills(ind5cSkills()),
		gameservertest.WithScriptSkills(ind5cCaptureDef(ind5cCaptureA, 1), ind5cCaptureDef(ind5cCaptureB, 1)),
	}}, append(templates, gameservertest.FolkTemplate("Folk", int(strongKeyBoxID)))...)
	return &ind1World{warriorWorld: w, draws: draws, heard: heard}
}

// ind5cLand applies the effects of skill ref on target from the
// character.
func (w *ind1World) ind5cLand(t *testing.T, ref modelskill.Ref, target *npc.Hostile) {
	t.Helper()
	if !w.srv.RunScript(t, w.player.ObjectID(), "Ind1Director", func(s *script.Script, p *script.Player) {
		s.ApplyEffects(ref, p, script.NPCOf(target))
	}) {
		t.Fatal("the effect landing panicked")
	}
	w.srv.Settle(t)
}

// ind5cSystemMessages returns the ids of the SystemMessage frames.
func ind5cSystemMessages(frames [][]byte) []int32 {
	var out []int32
	for _, f := range frames {
		if len(f) >= 5 && f[0] == serverpackets.OpcodeSystemMessage {
			out = append(out, int32(binary.LittleEndian.Uint32(f[1:5])))
		}
	}
	return out
}

// TestRoyalRushHereticBombsFollowScriptEvents ports RoyalRushHereticBomb's
// created, attacked, clan-attacked and script-event hooks, every draw 0.
// Each heretic bomb, RoyalRushHereticBomb2 included, spawns with i_ai4 0;
// a hit, or a hit on its clan, while it neither attacks nor casts sets
// i_ai4. Event 10035 clears i_ai4, drops its attack desires and wants to
// walk to its spawn point (weight 100). Event 10002 drops its attack
// desires and wants to attack the named creature (weight 200); 10033 does
// the same and sets i_ai4. An event naming no creature does nothing.
func TestRoyalRushHereticBombsFollowScriptEvents(t *testing.T) {
	t.Parallel()
	w := bootIndividual5c(t, ind5cClan(warriorTemplate(ind5cHereticBombID, nil)), ind5cClan(warriorTemplate(ind5cHereticBombTwoID, nil)),
		warriorTemplate(ind5cHereticBomb2ID, nil), ind5cClan(warriorTemplate(unboundID, nil)))
	n := w.spawn(t, ind5cHereticBombID, 300, 0)
	two := w.spawn(t, ind5cHereticBomb2ID, 1500, 300)
	me := w.player.ObjectID()
	for _, h := range []*npc.Hostile{n, two} {
		if got := ind1Slot(h, script.SlotIntAI4); got != 0 {
			t.Fatalf("%d: created i_ai4 = %d, want 0", h.NpcID(), got)
		}
		w.hit(t, h, 10)
		if got := ind1Slot(h, script.SlotIntAI4); got != 1 {
			t.Fatalf("%d: i_ai4 after a hit while idle = %d, want 1", h.NpcID(), got)
		}
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

	clanBomb := w.spawn(t, ind5cHereticBombTwoID, -1500, 0)
	member := w.spawn(t, unboundID, -1500, 100)
	w.hit(t, member, 10)
	if got := ind1Slot(clanBomb, script.SlotIntAI4); got != 1 {
		t.Fatalf("i_ai4 after a hit on its clan while idle = %d, want 1", got)
	}
}

// ind5cHereticDen is an npcmaker spawning one heretic bomb in a 400 by 400
// territory.
const ind5cHereticDen = `<?xml version="1.0" encoding="utf-8"?>
<list>
	<territory name="heretic_bomb_den" minZ="-1000" maxZ="1000"><node x="0" y="0"/><node x="400" y="0"/><node x="400" y="400"/><node x="0" y="400"/></territory>
	<npcmaker name="heretic_bomb_den" territory="heretic_bomb_den" maximumNpcs="1">
		<ai type="default_maker"/>
		<npc id="22145" total="1" pos="40;20;30;0" respawn="60sec"/>
	</npcmaker>
</list>`

// TestRoyalRushHereticBombWalksHomeFromOutside ports RoyalRushHereticBomb's
// out-of-territory hook on a heretic bomb its npcmaker spawned. Arriving
// outside its territory with i_ai4 clear, it wants to walk to its spawn
// point with weight 100. Released from the territory (i_ai4 set), its
// arrival outside wants no walk home.
func TestRoyalRushHereticBombWalksHomeFromOutside(t *testing.T) {
	t.Parallel()
	for _, row := range []struct {
		name     string
		released bool
	}{{"bound", false}, {"released", true}} {
		t.Run(row.name, func(t *testing.T) {
			t.Parallel()
			dir := t.TempDir()
			if err := os.WriteFile(filepath.Join(dir, "den.xml"), []byte(ind5cHereticDen), 0o644); err != nil {
				t.Fatalf("write spawnlist: %v", err)
			}
			table, err := gamexml.LoadSpawnlist(dir, zerolog.Nop(), 1)
			if err != nil {
				t.Fatalf("load spawnlist: %v", err)
			}
			list, catalog := warriorScripts()
			for _, c := range ind5cBatch {
				list = append(list, script.Listing{Path: c.path})
				catalog[c.path] = c.ctor
			}
			draws := &ind1Draws{}
			ww := bootRaid(t, raidBoot{opts: []gameservertest.Option{
				gameservertest.WithScriptRand(draws.rand),
				gameservertest.WithNPCScripts(map[int32]script.NPCKind{ind5cHereticBombID: script.KindHostile}, list, catalog),
				gameservertest.WithNpcSpawns(table),
			}}, warriorTemplate(ind5cHereticBombID, nil))
			w := &ind1World{warriorWorld: ww, draws: draws, heard: &ind1Heard{}}
			var bomb *npc.Hostile
			for _, obj := range w.srv.State.Objects() {
				if h, ok := obj.(*npc.Hostile); ok && h.NpcID() == int(ind5cHereticBombID) {
					bomb = h
				}
			}
			if bomb == nil {
				t.Fatal("the den spawned no heretic bomb")
			}
			if row.released {
				w.onQueue(t, bomb, func() { script.NPCOf(bomb).SetInt(script.SlotIntAI4, 1) })
			}
			home := script.NPCOf(bomb).SpawnLocation()

			ind4WalkTo(t, w, bomb, location.Location{X: 40, Y: 700, Z: 30})
			if bomb.InTerritory() {
				t.Fatal("setup: the walk ended inside the territory")
			}
			d, ok := desire(bomb, actorai.IntentionMoveTo)
			if row.released {
				if ok {
					t.Fatalf("released, move desire after arriving outside = %+v, want none", d)
				}
				return
			}
			if !ok || d.Weight != 100 || d.Location.X != home.X || d.Location.Y != home.Y {
				t.Fatalf("move desire after arriving outside = %+v (%v), want weight 100 to its spawn point %d,%d", d, ok, home.X, home.Y)
			}
		})
	}
}

// TestRoyalRushHereticBomb2WalksHomeWhenIdle ports RoyalRushHereticBomb2's
// no-desire hook: moved off its spawn point with nothing to do, it wants
// to walk back there with weight 30 instead of wandering.
func TestRoyalRushHereticBomb2WalksHomeWhenIdle(t *testing.T) {
	t.Parallel()
	w := bootIndividual5c(t, warriorTemplate(ind5cHereticBomb2ID, nil))
	n := w.spawn(t, ind5cHereticBomb2ID, 1500, 0)
	home := script.NPCOf(n).SpawnLocation()
	w.teleport(t, n, 200, 0)
	w.ticks(t, 2)
	if d, ok := desire(n, actorai.IntentionMoveTo); !ok || d.Weight != 30 || d.Location.X != home.X || d.Location.Y != home.Y {
		t.Fatalf("move desire = %+v (%v), want weight 30 to its spawn point %d,%d", d, ok, home.X, home.Y)
	}
	if d, ok := desire(n, actorai.IntentionWander); ok {
		t.Fatalf("wander desire = %+v, want none", d)
	}
}

// TestRoyalRushRoomBoss1WeakensItselfAsItsClanDies ports
// RoyalRushRoomBoss1's created and clan-died hooks and RoyalRushRoomBoss2's
// created hook. RoyalRushRoomBoss1 appears with i_ai0 0 wanting its first
// self debuff on itself (weight 1000000); RoyalRushRoomBoss2 wants
// nothing. Each death in its clan counts in i_ai0; a death met with i_ai0
// at 2, 4 or 6 wants its first, second or third self debuff on itself,
// one met at 0 none.
func TestRoyalRushRoomBoss1WeakensItselfAsItsClanDies(t *testing.T) {
	t.Parallel()
	w := bootIndividual5c(t, ind5cRoomBossTemplate(ind5cRoomBoss1ID, nil), ind5cRoomBossTemplate(ind5cRoomBoss2ID, nil), ind5cClan(warriorTemplate(unboundID, nil)))
	b := w.spawn(t, ind5cRoomBoss1ID, 1500, 0)
	ind5bWantCast(t, "as it appears", b, ind5cSelfDebuff1, b.ObjectID())
	if got := ind1Slot(b, script.SlotIntAI0); got != 0 {
		t.Fatalf("created i_ai0 = %d, want 0", got)
	}
	b2 := w.spawn(t, ind5cRoomBoss2ID, 1500, 200)
	w.wantCasts(t, "RoyalRushRoomBoss2 as it appears", b2, nil)

	for _, row := range []struct {
		count int32
		want  modelskill.ID
	}{{0, 0}, {2, ind5cSelfDebuff1}, {4, ind5cSelfDebuff2}, {6, ind5cSelfDebuff3}} {
		member := w.spawn(t, unboundID, 1500, -100)
		w.onQueue(t, b, func() {
			b.AI().Desires().Clear()
			script.NPCOf(b).SetInt(script.SlotIntAI0, row.count)
		})
		w.kill(t, member)
		want := map[modelskill.ID]ind1Cast{}
		if row.want != 0 {
			want[row.want] = ind1Cast{b.ObjectID(), 1000000}
		}
		w.wantCasts(t, fmt.Sprintf("a death met at %d", row.count), b, want)
		if got := ind1Slot(b, script.SlotIntAI0); got != row.count+1 {
			t.Fatalf("i_ai0 after a death met at %d = %d, want %d", row.count, got, row.count+1)
		}
	}
	if got := ind1Slot(b2, script.SlotIntAI0); got != 4 {
		t.Fatalf("RoyalRushRoomBoss2 i_ai0 after 4 deaths = %d, want 4", got)
	}
}

// TestRoyalRushRoomBossesStrikeTheirTopDesireTarget ports the attacked
// hooks of RoyalRushRoomBossBasic, RoyalRushRoomBoss1, RoyalRushRoomBoss4
// and RoyalRushRoomBoss5. Hit by the character while it is their top
// desire target, RoyalRushRoomBoss1 and 4 want their first range magic on
// it on a draw under 33, then their parent its first physical special on
// another; with no top desire target nothing is drawn or wanted.
// RoyalRushRoomBoss5 draws three times: under 33 on the second and third,
// with the character its top desire target, it wants its debuff and its
// magic on it (beside its parent's strike); with another top desire
// target, under 33 on the first, its self range magic on itself.
func TestRoyalRushRoomBossesStrikeTheirTopDesireTarget(t *testing.T) {
	t.Parallel()
	var templates []*npc.Template
	for _, id := range []int32{ind5cRoomBoss1ID, ind5cRoomBoss4ID, ind5cRoomBoss5ID} {
		// Out of any clan, so a hit on one is no clan hit on another.
		tmpl := ind5cRoomBossTemplate(id, nil)
		tmpl.Clans = nil
		templates = append(templates, tmpl)
	}
	w := bootIndividual5c(t, append(templates, warriorTemplate(unboundID, nil))...)
	me := w.player.ObjectID()
	other := w.spawn(t, unboundID, -1500, 0)
	b1 := w.spawn(t, ind5cRoomBoss1ID, 1500, 0)
	b4 := w.spawn(t, ind5cRoomBoss4ID, 1500, 300)
	b5 := w.spawn(t, ind5cRoomBoss5ID, 1500, -300)

	for _, b := range []*npc.Hostile{b1, b4} {
		w.clear(t, b)
		w.draws.set(0)
		w.hit(t, b, 10)
		w.wantCasts(t, fmt.Sprintf("%d hit with no top desire target", b.NpcID()), b, nil)

		w.setTop(t, b, script.PlayerOf(w.player))
		w.hit(t, b, 10)
		w.wantCasts(t, fmt.Sprintf("%d hit by its top desire target", b.NpcID()), b, map[modelskill.ID]ind1Cast{
			ind5cRangeDD: {me, 1000000}, ind5cSpecial1: {me, 1000000},
		})

		w.setTop(t, b, script.PlayerOf(w.player))
		w.draws.set(0, 33, 0)
		w.hit(t, b, 10)
		w.wantCasts(t, fmt.Sprintf("%d hit by its top desire target, its own draw lost", b.NpcID()), b, map[modelskill.ID]ind1Cast{
			ind5cSpecial1: {me, 1000000},
		})
	}

	w.setTop(t, b5, script.PlayerOf(w.player))
	w.draws.set(0)
	w.hit(t, b5, 10)
	w.wantCasts(t, "RoyalRushRoomBoss5 hit by its top desire target", b5, map[modelskill.ID]ind1Cast{
		ind5cDebuff: {me, 1000000}, ind5cDDMagic: {me, 1000000}, ind5cSpecial1: {me, 1000000},
	})
	w.setTop(t, b5, script.NPCOf(other))
	w.hit(t, b5, 10)
	w.wantCasts(t, "RoyalRushRoomBoss5 hit by another", b5, map[modelskill.ID]ind1Cast{
		ind5cSelfRange: {b5.ObjectID(), 1000000},
	})
	w.setTop(t, b5, script.NPCOf(other))
	w.draws.set(0, 33)
	w.hit(t, b5, 10)
	w.wantCasts(t, "RoyalRushRoomBoss5 hit by another, its first draw lost", b5, nil)
}

// TestRoyalRushRoomBoss4BuffsItsClan ports RoyalRushRoomBoss4's created
// and clan-attacked hooks. It appears wanting its clan buff on itself
// (weight 1000000). Told of the character's hit on a clan member, on two
// draws under 33 and 10 it wants its first range magic on the character,
// not its top desire target, and its clan buff on the member; the
// character its top desire target, only the buff; draws of 33 and 10, nothing.
func TestRoyalRushRoomBoss4BuffsItsClan(t *testing.T) {
	t.Parallel()
	w := bootIndividual5c(t, ind5cRoomBossTemplate(ind5cRoomBoss4ID, nil), ind5cClan(warriorTemplate(unboundID, nil)))
	me := w.player.ObjectID()
	b := w.spawn(t, ind5cRoomBoss4ID, 1500, 0)
	ind5bWantCast(t, "as it appears", b, ind5cClanBuff, b.ObjectID())
	member := w.spawn(t, unboundID, 1500, 100)

	w.clear(t, b)
	w.draws.set(0)
	w.hit(t, member, 10)
	w.wantCasts(t, "a hit on its clan", b, map[modelskill.ID]ind1Cast{
		ind5cRangeDD: {me, 1000000}, ind5cClanBuff: {member.ObjectID(), 1000000},
	})

	w.setTop(t, b, script.PlayerOf(w.player))
	w.hit(t, member, 10)
	w.wantCasts(t, "a hit on its clan by its top desire target", b, map[modelskill.ID]ind1Cast{
		ind5cClanBuff: {member.ObjectID(), 1000000},
	})

	w.clear(t, b)
	w.draws.set(0, 33, 10)
	w.hit(t, member, 10)
	w.wantCasts(t, "a hit on its clan, both draws lost", b, nil)
}

// TestRoyalRushRoomBossLeavesItsKeyBox ports RoyalRushRoomBossBasic's
// my-dying hook on a RoyalRushRoomBoss5: dying, it leaves its KeyBox NPC
// as its private at the KeyBox_X, KeyBox_Y point it names, or where it
// died when one of the three is 0.
func TestRoyalRushRoomBossLeavesItsKeyBox(t *testing.T) {
	t.Parallel()
	for _, row := range []struct {
		name string
		at   bool
	}{{"at its point", true}, {"where it died", false}} {
		t.Run(row.name, func(t *testing.T) {
			t.Parallel()
			// A point of the world away from the character.
			point := location.Location{X: 83000, Y: 148000, Z: -3400}
			ai := npc.AIParams{"KeyBox": fmt.Sprint(strongKeyBoxID)}
			if row.at {
				ai["KeyBox_X"], ai["KeyBox_Y"], ai["KeyBox_Z"] = fmt.Sprint(point.X), fmt.Sprint(point.Y), fmt.Sprint(point.Z)
			}
			w := bootIndividual5c(t, ind5cRoomBossTemplate(ind5cRoomBoss5ID, ai))
			n := w.spawn(t, ind5cRoomBoss5ID, 1500, 0)
			if !row.at {
				point = positionOf(n)
			}
			w.die(t, n)
			boxes := w.keyBoxes()
			if len(boxes) != 1 {
				t.Fatalf("Key Boxes after the boss died = %d, want 1", len(boxes))
			}
			if at := ind8At(boxes[0]); at.X != point.X || at.Y != point.Y {
				t.Fatalf("Key Box at %v, want %v", at, point)
			}
			if m := script.NPCOf(boxes[0]).Master(); m == nil || m.ObjectID() != n.ObjectID() {
				t.Fatalf("Key Box master = %v, want the boss %d", m, n.ObjectID())
			}
		})
	}
}

// TestTyrannoPrimevalBuffsItselfAtAnyHitPoints ports TyrannoPrimeval's
// created, attacked and skill-finished hooks. It appears with a chance
// multiplier (i_ai3) of 1. Its hit point rates are percents compared with
// a ratio of at most 1, so even at full hit points a hit drops its attack
// desires, remembers its top desire target and wants its second self buff
// (weight 1000000); farther than LongRangeSkillDist (0) from the
// attacker, a draw at most ProbLongRangeDDMagic1 x i_ai3 also wants its
// long range magic on the attacker. Its second self buff finished in
// combat sets i_ai3 to ProbMultiplier2 (5) and forgets the remembered
// target, wanting to attack it again.
func TestTyrannoPrimevalBuffsItselfAtAnyHitPoints(t *testing.T) {
	t.Parallel()
	w := bootIndividual5c(t, ind5cTyrannoTemplate(npc.AIParams{"ProbLongRangeDDMagic1": "10"}, map[npc.SkillType]modelskill.ID{
		npc.SkillTypeSelfBuff1: ind5cSelfBuff1, npc.SkillTypeSelfBuff2: ind5cSelfBuff2, npc.SkillTypeLongRangeDDMagic1: ind5cLongRange,
	}))
	me := w.player.ObjectID()
	n := w.spawn(t, ind5cTyrannoID, 300, 0)
	if a3 := ind1Slot(n, script.SlotIntAI3); a3 != 1 {
		t.Fatalf("created i_ai3 = %d, want 1", a3)
	}
	w.hit(t, n, 10)
	w.wantCasts(t, "after the first hit", n, map[modelskill.ID]ind1Cast{
		ind5cSelfBuff2: {n.ObjectID(), 1000000}, ind5cLongRange: {me, 1000000},
	})
	if c1 := script.NPCOf(n).Slot(script.SlotCreature1); c1 != nil {
		t.Fatalf("c_ai1 after the first hit = %v, want none: it had no top desire target", c1)
	}
	w.ind5cIntoCombat(t, n)
	w.setTop(t, n, script.PlayerOf(w.player))
	w.draws.set(0, 11)
	w.hit(t, n, 10)
	w.wantCasts(t, "after a hit by its top desire target, its draw lost", n, map[modelskill.ID]ind1Cast{
		ind5cSelfBuff2: {n.ObjectID(), 1000000},
	})
	if c1 := script.NPCOf(n).Slot(script.SlotCreature1); c1 == nil || c1.ObjectID() != me {
		t.Fatalf("c_ai1 after the second hit = %v, want the character", c1)
	}
	w.draws.set(99)
	ind5bRunUntil(t, w, "the second self buff's end", func() bool { return ind1Slot(n, script.SlotIntAI3) == 5 })
	if c1 := script.NPCOf(n).Slot(script.SlotCreature1); c1 != nil {
		t.Fatalf("c_ai1 after the buff = %v, want forgotten", c1)
	}
	if d, ok := desire(n, actorai.IntentionAttack); !ok || d.FinalTarget == nil || d.FinalTarget.ObjectID() != me {
		t.Fatalf("attack desire after the buff = %+v (%v), want on the character", d, ok)
	}
}

// ind5cIntoCombat drops h's cast desires and gives h the attack stance
// of a fight, on its queue.
func (w *ind1World) ind5cIntoCombat(t *testing.T, h *npc.Hostile) {
	t.Helper()
	w.onQueue(t, h, func() {
		h.AI().Desires().RemoveKind(actorai.IntentionCast)
		h.SetInCombat(true)
	})
}

// TestTyrannoPrimevalCaptureCycle ports TyrannoPrimeval's abnormal-status,
// skill-finished and timer hooks. In combat, the first capture skill at
// level 1 matching the social animation it last played (0) wants its first
// debuff on itself (weight 10000000), sets the capture stage and arms the
// capture timer for CaptureCycle (10) plus a CaptureCycleRand (4) draw
// seconds. The second capture skill, not matching, wants its matching
// capture cancel on itself and remembers the caster, who is told the trap
// failed once that cancel lands. At the timer, still in combat and the
// stage set, a draw under ProbTimer (100) wants its cancel-all on itself,
// clears the stage and plays the social animation a draw of 3 picks.
func TestTyrannoPrimevalCaptureCycle(t *testing.T) {
	t.Parallel()
	w := bootIndividual5c(t, ind5cTyrannoTemplate(nil, map[npc.SkillType]modelskill.ID{
		npc.SkillTypeDebuff1: ind5cDebuff1, npc.SkillTypeDebuff2: ind5cDebuff2, npc.SkillTypeCaptureCancelA: ind5cCancelA,
		npc.SkillTypeCaptureCancelB: ind5cCancelB, npc.SkillTypeCaptureCancelC: ind5cCancelC, npc.SkillTypeCaptureCancelAll: ind5cCancelAll,
	}))
	me := w.player.ObjectID()
	n := w.spawn(t, ind5cTyrannoID, 300, 0)
	w.hit(t, n, 10)
	w.ind5cIntoCombat(t, n)
	w.draws.set(2)
	w.ind5cLand(t, modelskill.Ref{ID: ind5cCaptureA, Level: 1}, n)
	if d, ok := ind1Casts(n)[ind5cDebuff1]; !ok || d.Weight != 10000000 || d.FinalTarget == nil || d.FinalTarget.ObjectID() != n.ObjectID() {
		t.Fatalf("debuff 1 desire after the matching capture = %+v (%v), want on itself, weight 10000000", d, ok)
	}
	if got := ind1Slot(n, npc.IntQuest0); got != 1 {
		t.Fatalf("capture stage after the matching capture = %d, want 1", got)
	}
	w.ind5cLand(t, modelskill.Ref{ID: ind5cCaptureB, Level: 1}, n)
	if d, ok := ind1Casts(n)[ind5cCancelB]; !ok || d.Weight != 10000000 || d.FinalTarget == nil || d.FinalTarget.ObjectID() != n.ObjectID() {
		t.Fatalf("cancel B desire after the wrong capture = %+v (%v), want on itself, weight 10000000", d, ok)
	}
	if c0 := script.NPCOf(n).Slot(script.SlotCreature0); c0 == nil || c0.ObjectID() != me {
		t.Fatalf("c_ai0 after the wrong capture = %v, want the character", c0)
	}
	var frames [][]byte
	for i := 0; !slices.Contains(ind5cSystemMessages(frames), ind5cTrapFailed); i++ {
		if i == 40 {
			t.Fatal("the character was not told the trap failed within 40 AI cycles")
		}
		frames = append(frames, w.ticks(t, 1)...)
		w.srv.Advance(t, 500*time.Millisecond)
		frames = append(frames, w.srv.ReadQueued(t, w.srv.Client)...)
	}
	if got := castsOf(frames, n, ind5cCancelB); len(got) != 1 {
		t.Fatalf("casts of cancel B = %v, want one", got)
	}

	w.srv.Advance(t, 12*time.Second)
	ind5bRunUntil(t, w, "the capture timer", func() bool { return ind1Slot(n, npc.IntQuest0) == 0 })
	if got := ind1Slot(n, npc.IntQuest1); got != 2 {
		t.Fatalf("social animation after the capture timer = %d, want the drawn 2", got)
	}
}

// TestTyrannoPrimevalReportsAndShoutsAsItDies ports TyrannoPrimeval's
// script-event, timer and my-dying hooks. Event 11049 makes it report and
// arms the 600 s timer "9002": with more than 60 s since its last fight or
// sighting and not attacking, the timer broadcasts event 11051 to the
// NPCs within 8000. Dying while reporting it shouts and broadcasts event
// 11044 within 5000.
func TestTyrannoPrimevalReportsAndShoutsAsItDies(t *testing.T) {
	t.Parallel()
	w := bootIndividual5c(t, ind5cTyrannoTemplate(nil, nil), warriorTemplate(ind1ListenerID, nil))
	n := w.spawn(t, ind5cTyrannoID, 1500, 0)
	w.spawn(t, ind1ListenerID, 1500, 3000)
	w.sendEvent(t, n, 11049, 0, 0)
	if got := ind1Slot(n, npc.IntQuest2); got != 1 {
		t.Fatalf("i_quest2 after event 11049 = %d, want 1", got)
	}
	w.onQueue(t, n, func() { script.NPCOf(n).SetInt(script.SlotIntAI4, ind1Slot(n, script.SlotIntAI4)-61) })
	w.srv.Advance(t, 599*time.Second)
	if got := w.heard.take(); len(got) != 0 {
		t.Fatalf("the listener heard %q before the timer", got)
	}
	w.srv.Advance(t, time.Second)
	if got := w.heard.take(); !slices.Equal(got, []string{"event=11051 arg1=0 arg2=0"}) {
		t.Fatalf("the listener heard %q at the timer, want event 11051 once", got)
	}

	w.kill(t, n)
	w.srv.Advance(t, 3500*time.Millisecond)
	frames := w.srv.ReadQueued(t, w.srv.Client)
	if got := shouts(t, frames, n.ObjectID()); len(got) != 1 {
		t.Fatalf("dying shouts = %q, want one", got)
	}
	if got := w.heard.take(); !slices.Equal(got, []string{"event=11044 arg1=0 arg2=0"}) {
		t.Fatalf("the listener heard %q as it died, want event 11044 once", got)
	}
}

// TestTyrannoPrimevalGhostLeavesOutOfCombat ports TyrannoPrimeval's
// created and timer hooks with CollectGhost set: out of combat, it leaves
// the world CollectGhostDespawnTime minutes after it appeared. Without
// CollectGhost it stays.
func TestTyrannoPrimevalGhostLeavesOutOfCombat(t *testing.T) {
	t.Parallel()
	w := bootIndividual5c(t, ind5cTyrannoTemplate(npc.AIParams{"CollectGhost": "1", "CollectGhostDespawnTime": "1"}, nil), ind5cClan(warriorTemplate(ind5cTyrannoID+1, nil)))
	ghost := w.spawn(t, ind5cTyrannoID, 1500, 0)
	w.srv.Advance(t, 59*time.Second)
	if !w.inWorld(ghost) {
		t.Fatal("the ghost left before its despawn time")
	}
	w.srv.Advance(t, time.Second)
	if w.inWorld(ghost) {
		t.Fatal("the ghost is still in the world after its despawn time")
	}
}

// TestTyrannoPrimevalWalksHomeAndRebuffs ports TyrannoPrimeval's
// no-desire hook: out of combat with a chance multiplier above 1 it wants
// its first self buff on itself (weight 1000000) and resets the multiplier
// to 1; with mobile_type 0 it wants to walk back to its spawn point
// (weight 30).
func TestTyrannoPrimevalWalksHomeAndRebuffs(t *testing.T) {
	t.Parallel()
	w := bootIndividual5c(t, ind5cTyrannoTemplate(nil, map[npc.SkillType]modelskill.ID{npc.SkillTypeSelfBuff1: ind5cSelfBuff1}))
	n := w.spawn(t, ind5cTyrannoID, 1500, 0)
	home := script.NPCOf(n).SpawnLocation()
	w.teleport(t, n, 200, 0)
	w.onQueue(t, n, func() { script.NPCOf(n).SetInt(script.SlotIntAI3, 5) })
	w.ticks(t, 2)
	if got := ind1Slot(n, script.SlotIntAI3); got != 1 {
		t.Fatalf("i_ai3 after an idle cycle = %d, want 1", got)
	}
	ind5bWantCast(t, "after an idle cycle", n, ind5cSelfBuff1, n.ObjectID())
	if d, ok := desire(n, actorai.IntentionMoveTo); !ok || d.Weight != 30 || d.Location.X != home.X || d.Location.Y != home.Y {
		t.Fatalf("move desire = %+v (%v), want weight 30 to its spawn point %d,%d", d, ok, home.X, home.Y)
	}
}

// TestTyrannoPrimevalSeesAndAnswersShouts ports TyrannoPrimeval's
// see-creature and script-event hooks. With ag_type 1, seeing the
// character out of combat it wants its long range magic on the character
// (weight 1000000) and attacks it. Event 10016 naming the character with
// BroadCastReception 1 drops its attack desires and wants to attack the
// character with weight 100; its parent, on a lost draw, adds nothing.
func TestTyrannoPrimevalSeesAndAnswersShouts(t *testing.T) {
	t.Parallel()
	t.Run("sees", func(t *testing.T) {
		t.Parallel()
		w := bootIndividual5c(t, ind5cTyrannoTemplate(npc.AIParams{"ag_type": "1"}, map[npc.SkillType]modelskill.ID{npc.SkillTypeLongRangeDDMagic1: ind5cLongRange}))
		n := w.spawn(t, ind5cTyrannoID, 150, 0)
		w.ticks(t, 1)
		ind5bWantCast(t, "after seeing the character", n, ind5cLongRange, w.player.ObjectID())
		if n.AggroHate(w.player) == 0 {
			t.Fatal("the tyrannosaurus does not attack the character it sees")
		}
	})
	t.Run("shout", func(t *testing.T) {
		t.Parallel()
		w := bootIndividual5c(t, ind5cTyrannoTemplate(npc.AIParams{"BroadCastReception": "1"}, nil))
		n := w.spawn(t, ind5cTyrannoID, 300, 0)
		w.draws.set(99)
		w.sendEvent(t, n, 10016, w.player.ObjectID(), 0)
		wantDesire(t, "after event 10016", n, actorai.IntentionAttack, w.player.ObjectID(), 100)
	})
}
