package ai

import (
	"encoding/binary"
	"fmt"
	"testing"
	"time"

	actorai "github.com/fatal10110/acis_golang/internal/gameserver/model/actor/ai"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/npc"
	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/script"
	"github.com/fatal10110/acis_golang/internal/gameserver/script/ai/defaultnpc"
	"github.com/fatal10110/acis_golang/internal/gameserver/script/ai/royalrush"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
)

// The Royal Rush NPCs the tests spawn and the afflict skills the
// watchmen list.
const (
	afflictID     = int32(18244)
	afflictTwoID  = int32(18245)
	treasureBoxID = int32(18256)

	afflictSkill1ID = modelskill.ID(9411)
	afflictSkill2ID = modelskill.ID(9412)
	afflictSkill3ID = modelskill.ID(9413)
)

// bootRoyalRush boots one character beside the templates, the Royal Rush
// chain registered in scripts.xml order, every NPC casting the afflict
// skills, with a director script to read the clock through, every draw 0
// until a test queues others.
func bootRoyalRush(t *testing.T, templates ...*npc.Template) *ind1World {
	t.Helper()
	paths := []string{
		"script.ai.individual.DefaultNpc",
		"script.ai.individual.RoyalRushDefaultNpc.RoyalRushAfflict",
		"script.ai.individual.RoyalRushDefaultNpc.RoyalRushDefaultNpc",
		"script.ai.individual.RoyalRushDefaultNpc.RoyalRushTrasureBox",
	}
	catalog := script.Catalog{
		paths[0]: defaultnpc.New,
		paths[1]: royalrush.Afflict,
		paths[2]: royalrush.DefaultNpc,
		paths[3]: royalrush.TreasureBox,
	}
	list := make([]script.Listing, 0, len(paths)+1)
	for _, path := range paths {
		list = append(list, script.Listing{Path: path})
	}
	const director = "test.Ind1Director"
	catalog[director] = func() script.Script { return script.Script{} }
	list = append(list, script.Listing{Path: director})
	kinds := map[int32]script.NPCKind{}
	for _, tmpl := range templates {
		kinds[int32(tmpl.ID)] = script.KindHostile
	}
	draws := &ind1Draws{}
	w := bootRaid(t, raidBoot{opts: []gameservertest.Option{
		gameservertest.WithScriptRand(draws.rand),
		gameservertest.WithNPCScripts(kinds, list, catalog),
		gameservertest.WithNpcSpawnSkills(modelskill.NewTable([]modelskill.Definition{
			ind1Skill(afflictSkill1ID, 0, true), ind1Skill(afflictSkill2ID, 0, true), ind1Skill(afflictSkill3ID, 0, true),
		})),
	}}, templates...)
	if !w.srv.DrivesClock() {
		t.Fatal("setup: the test does not drive the clock")
	}
	return &ind1World{warriorWorld: w, draws: draws, heard: &ind1Heard{}}
}

// afflictTemplate is a fixture watchman of the graves of id, listing its
// three afflict skills by type.
func afflictTemplate(id int32) *npc.Template {
	tmpl := warriorTemplate(id, nil)
	tmpl.SkillsByType = map[npc.SkillType]modelskill.Ref{
		npc.SkillTypeAfflictSkill1: {ID: afflictSkill1ID, Level: 1},
		npc.SkillTypeAfflictSkill2: {ID: afflictSkill2ID, Level: 1},
		npc.SkillTypeAfflictSkill3: {ID: afflictSkill3ID, Level: 1},
	}
	return tmpl
}

// treasureBoxTemplate is the fixture Halisha's treasure box, of the kind
// the datapack gives it and listing no afflict skill.
func treasureBoxTemplate() *npc.Template {
	tmpl := warriorTemplate(treasureBoxID, nil)
	tmpl.Type, tmpl.Name = "HalishaChest", "Halisha's Treasure Box"
	return tmpl
}

// toMinute advances the clock to second sec of the next minute min of an
// hour and returns that instant.
func (w *ind1World) toMinute(t *testing.T, minute, sec int) time.Time {
	t.Helper()
	now := w.now(t)
	at := time.Date(now.Year(), now.Month(), now.Day(), now.Hour(), minute, sec, 0, now.Location())
	if !at.After(now) {
		at = at.Add(time.Hour)
	}
	w.srv.Advance(t, at.Sub(now))
	w.srv.ReadQueued(t, w.srv.Client)
	return at
}

// skillUses returns the skill ids of the MagicSkillUse frames objectID
// sent.
func skillUses(frames [][]byte, objectID int32) []modelskill.ID {
	var out []modelskill.ID
	for _, f := range framesOf(frames, serverpackets.OpcodeMagicSkillUse, objectID) {
		// opcode, caster, target, skill id, then the rest of the cast.
		out = append(out, modelskill.ID(binary.LittleEndian.Uint32(f[9:13])))
	}
	return out
}

// TestRoyalRushAfflictBombCastsAndVanishes ports RoyalRushAfflict's plan 0:
// created on a draw of 0, a watchman sets its first int slot to 0, looks
// around and, seeing the character on a follow draw under 50, wants to
// follow it with weight 100. 5 s on, timer 3001 makes it want its first
// afflict skill on itself, weight 1000000; once that cast ends, it leaves
// the world.
func TestRoyalRushAfflictBombCastsAndVanishes(t *testing.T) {
	t.Parallel()
	w := bootRoyalRush(t, afflictTemplate(afflictID))
	w.toMinute(t, 10, 0)
	n := w.spawn(t, afflictID, 150, 0)
	if got := ind8Slot(n, script.SlotIntAI0); got != 0 {
		t.Fatalf("first int slot = %d, want plan 0", got)
	}
	if d, ok := desire(n, actorai.IntentionFollow); !ok || d.Weight != 100 || d.FinalTarget.ObjectID() != w.player.ObjectID() {
		t.Fatalf("follow desire after its look = %+v (%v), want weight 100 on the character", d, ok)
	}

	w.srv.Advance(t, 4999*time.Millisecond)
	w.wantCasts(t, "before timer 3001", n, nil)
	w.srv.Advance(t, time.Millisecond)
	w.wantCasts(t, "timer 3001", n, map[modelskill.ID]ind1Cast{afflictSkill1ID: {n.ObjectID(), 1000000}})

	frames := w.ticks(t, 2)
	frames = append(frames, w.advanceFrames(t, 2*time.Second)...)
	if got := skillUses(frames, n.ObjectID()); len(got) != 1 || got[0] != afflictSkill1ID {
		t.Fatalf("watchman casts = %v, want its first afflict skill once", got)
	}
	if w.inWorld(n) {
		t.Fatal("the watchman is still in the world after its bomb")
	}
	if got := framesOf(frames, serverpackets.OpcodeDeleteObject, n.ObjectID()); len(got) != 1 {
		t.Fatalf("DeleteObject frames of the watchman = %d, want 1", len(got))
	}
}

// TestRoyalRushAfflictRecastsItsCurse ports RoyalRushAfflict's plans 1 and
// 2: created on a draw of 1 or 2, a watchman keeps the plan in its first
// int slot; 5 s on, timer 3002 makes it want its second or third afflict
// skill on itself, weight 1000000, and each time that cast ends it wants it
// again; it stays in the world. A look-around draw of 50 or more leaves it
// with no wish to follow the character.
func TestRoyalRushAfflictRecastsItsCurse(t *testing.T) {
	t.Parallel()
	for _, row := range []struct {
		plan  int
		skill modelskill.ID
	}{{1, afflictSkill2ID}, {2, afflictSkill3ID}} {
		t.Run(fmt.Sprint("plan ", row.plan), func(t *testing.T) {
			w := bootRoyalRush(t, afflictTemplate(afflictID))
			w.toMinute(t, 10, 0)
			w.draws.set(0, row.plan, 50)
			n := w.spawn(t, afflictID, 150, 0)
			if got := ind8Slot(n, script.SlotIntAI0); got != int32(row.plan) {
				t.Fatalf("plan %d: first int slot = %d", row.plan, got)
			}
			if d, ok := desire(n, actorai.IntentionFollow); ok {
				t.Fatalf("plan %d: follow desire on a draw of 50 = %+v, want none", row.plan, d)
			}

			w.srv.Advance(t, 5*time.Second)
			w.wantCasts(t, "timer 3002", n, map[modelskill.ID]ind1Cast{row.skill: {n.ObjectID(), 1000000}})
			frames := w.ticks(t, 2)
			frames = append(frames, w.advanceFrames(t, 2*time.Second)...)
			if got := skillUses(frames, n.ObjectID()); len(got) == 0 || got[0] != row.skill {
				t.Fatalf("plan %d: watchman casts = %v, want skill %d", row.plan, got, row.skill)
			}
			if !w.inWorld(n) {
				t.Fatalf("plan %d: the watchman left the world after its cast", row.plan)
			}
			w.wantCasts(t, "after the cast", n, map[modelskill.ID]ind1Cast{row.skill: {n.ObjectID(), 1000000}})
		})
	}
}

// TestRoyalRushAfflictLeavesAtMinute54 ports RoyalRushDefaultNpc's 3000
// timer, which RoyalRushAfflict and RoyalRushTrasureBox run through their
// created hooks: every second the NPC reads the local clock and leaves the
// world, with no corpse, at minute 54. Seconds of minute 53 keep it.
func TestRoyalRushAfflictLeavesAtMinute54(t *testing.T) {
	t.Parallel()
	w := bootRoyalRush(t, afflictTemplate(afflictID), treasureBoxTemplate())
	w.toMinute(t, 53, 57)
	watchman := w.spawn(t, afflictID, 150, 0)
	box := w.spawn(t, treasureBoxID, -150, 0)

	w.srv.Advance(t, 2*time.Second)
	if got := w.now(t); got.Minute() != 53 || got.Second() != 59 {
		t.Fatalf("setup: clock = %v, want 53:59", got)
	}
	if !w.inWorld(watchman) || !w.inWorld(box) {
		t.Fatal("an NPC left the world in minute 53")
	}
	w.srv.ReadQueued(t, w.srv.Client)

	w.srv.Advance(t, time.Second)
	frames := w.srv.ReadQueued(t, w.srv.Client)
	for _, h := range []*npc.Hostile{watchman, box} {
		if w.inWorld(h) {
			t.Fatalf("npc %d is still in the world at minute 54", h.ObjectID())
		}
		if got := framesOf(frames, serverpackets.OpcodeDeleteObject, h.ObjectID()); len(got) != 1 {
			t.Fatalf("DeleteObject frames of npc %d = %d, want 1", h.ObjectID(), len(got))
		}
	}
}

// TestRoyalRushAfflictFleesAndWanders ports RoyalRushAfflict's attacked and
// no-desire hooks: a hit makes a watchman want to flee its attacker by its
// drift range, weight 100, and nothing more (no hate, no attack); with
// nothing to do, another wants to wander every 5 s with weight 5.
func TestRoyalRushAfflictFleesAndWanders(t *testing.T) {
	t.Parallel()
	w := bootRoyalRush(t, afflictTemplate(afflictID), afflictTemplate(afflictTwoID))
	w.toMinute(t, 10, 0)
	w.draws.set(0, 0, 50)
	hit := w.spawn(t, afflictID, 150, 0)
	w.draws.set(0, 0, 50)
	idle := w.spawn(t, afflictTwoID, 0, 600)

	w.hit(t, hit, 10)
	d, ok := desire(hit, actorai.IntentionFlee)
	if !ok || d.Weight != 100 || d.Distance != hit.Instance.MaxDriftRange() || d.FinalTarget.ObjectID() != w.player.ObjectID() {
		t.Fatalf("flee desire after a hit = %+v (%v), want weight 100 over drift range %d from the character", d, ok, hit.Instance.MaxDriftRange())
	}
	if got := hit.AggroHate(w.player); got != 0 {
		t.Fatalf("watchman hate after a hit = %v, want none", got)
	}
	if _, ok := desire(hit, actorai.IntentionAttack); ok {
		t.Fatal("the hit watchman wants to attack")
	}

	w.ticks(t, 2)
	if d, ok := desire(idle, actorai.IntentionWander); !ok || d.Weight != 5 || d.Timer != 5 {
		t.Fatalf("idle watchman wander desire = %+v (%v), want timer 5, weight 5", d, ok)
	}
}

// TestRoyalRushTreasureBoxLeavesAfterFiveMinutes ports RoyalRushTrasureBox's
// created and attacked hooks: the box, which cannot move, ignores hits (its
// flee of 150 is refused, and it adds no hate), casts nothing as it lists no
// afflict skill, and leaves the world 300 s after it spawns.
func TestRoyalRushTreasureBoxLeavesAfterFiveMinutes(t *testing.T) {
	t.Parallel()
	w := bootRoyalRush(t, treasureBoxTemplate())
	w.toMinute(t, 10, 0)
	box := w.spawn(t, treasureBoxID, 150, 0)
	at := position(box)

	w.hit(t, box, 10)
	if _, ok := desire(box, actorai.IntentionFlee); ok {
		t.Fatal("the treasure box wants to flee")
	}
	if got := box.AggroHate(w.player); got != 0 {
		t.Fatalf("treasure box hate after a hit = %v, want none", got)
	}

	frames := w.advanceFrames(t, 299*time.Second)
	if !w.inWorld(box) {
		t.Fatal("the treasure box left the world before 300 s")
	}
	if got := skillUses(frames, box.ObjectID()); len(got) != 0 {
		t.Fatalf("treasure box casts = %v, want none", got)
	}
	if got := position(box); got != at {
		t.Fatalf("treasure box stands at %+v, want where it spawned, %+v", got, at)
	}
	frames = w.advanceFrames(t, time.Second)
	if w.inWorld(box) {
		t.Fatal("the treasure box is still in the world at 300 s")
	}
	if got := framesOf(frames, serverpackets.OpcodeDeleteObject, box.ObjectID()); len(got) != 1 {
		t.Fatalf("DeleteObject frames of the treasure box = %d, want 1", len(got))
	}
}

// advanceFrames advances the clock by d and returns the frames sent on the
// way.
func (w *ind1World) advanceFrames(t *testing.T, d time.Duration) [][]byte {
	t.Helper()
	w.srv.Advance(t, d)
	return w.srv.ReadQueued(t, w.srv.Client)
}
