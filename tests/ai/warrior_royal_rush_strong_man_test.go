package ai

import (
	"testing"
	"time"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/npc"
	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/script"
	"github.com/fatal10110/acis_golang/internal/gameserver/script/ai/warrior"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
)

// The Four Sepulchers strong men the scenarios spawn (#186 PR 1 trailing),
// fixture monsters of level 1, and the Key Box one leaves where it dies.
const (
	strongManID    = int32(18158) // RoyalRushStrongMan1
	strongManTwoID = int32(18160) // RoyalRushStrongMan1
	strongMan2ID   = int32(18159) // RoyalRushStrongMan2
	strongKeyBoxID = int32(31455)
)

// The skills the strong men list by type.
const (
	strongSelfBuffID  = modelskill.ID(9451)
	strongSpecial1ID  = modelskill.ID(9452)
	strongSelfRangeID = modelskill.ID(9453)
	strongDDMagicID   = modelskill.ID(9454)
)

// strongMenBatch are the strong men's classes, in scripts.xml order.
var strongMenBatch = []struct {
	path string
	ctor func() script.Script
}{
	{"script.ai.individual.Monster.WarriorBase.Warrior.RoyalRushStrongMan1", warrior.RoyalRushStrongMan1},
	{"script.ai.individual.Monster.WarriorBase.Warrior.RoyalRushStrongMan2", warrior.RoyalRushStrongMan2},
}

// bootStrongMen boots the strong men's classes after the Warrior chain
// beside hostile templates and the civilian Key Box, every NPC casting the
// strong men's skills, with a director script to read the clock through, every draw 0 until a scenario queues others.
func bootStrongMen(t *testing.T, templates ...*npc.Template) *ind1World {
	t.Helper()
	list, catalog := warriorScripts()
	for _, c := range strongMenBatch {
		list = append(list, script.Listing{Path: c.path})
		catalog[c.path] = c.ctor
	}
	const director = "test.Ind1Director"
	catalog[director] = func() script.Script { return script.Script{} }
	list = append(list, script.Listing{Path: director})
	kinds := map[int32]script.NPCKind{}
	for _, tmpl := range templates {
		kinds[int32(tmpl.ID)] = script.KindHostile
	}
	draws := &ind1Draws{}
	opts := []gameservertest.Option{
		gameservertest.WithScriptRand(draws.rand),
		gameservertest.WithNPCScripts(kinds, list, catalog),
		gameservertest.WithNpcSpawnSkills(strongManSkills()),
	}
	all := append(templates, gameservertest.FolkTemplate("Folk", int(strongKeyBoxID)))
	w := bootRaid(t, raidBoot{opts: opts}, all...)
	return &ind1World{warriorWorld: w, draws: draws, heard: &ind1Heard{}}
}

// strongManTemplate is a fixture strong man of id with ai, listing its
// self buff, first physical special, first self range DD magic and first
// DD magic by type, in the clan the scenario's NPCs share.
func strongManTemplate(id int32, ai npc.AIParams) *npc.Template {
	tmpl := warriorTemplate(id, ai)
	tmpl.SkillsByType = map[npc.SkillType]modelskill.Ref{
		npc.SkillTypeSelfBuff:          {ID: strongSelfBuffID, Level: 1},
		npc.SkillTypePhysicalSpecial1:  {ID: strongSpecial1ID, Level: 1},
		npc.SkillTypeSelfRangeDDMagic1: {ID: strongSelfRangeID, Level: 1},
		npc.SkillTypeDDMagic1:          {ID: strongDDMagicID, Level: 1},
	}
	tmpl.Clans, tmpl.ClanRange = []string{"sepulcher"}, 600
	return tmpl
}

// strongManSkills are the strong men's skills.
func strongManSkills() *modelskill.Table {
	return modelskill.NewTable([]modelskill.Definition{
		ind1Skill(strongSelfBuffID, 0, true), ind1Skill(strongSpecial1ID, 600, false),
		ind1Skill(strongSelfRangeID, 0, true), ind1Skill(strongDDMagicID, 600, false),
	})
}

// now reads the clock the scripts run on.
func (w *ind1World) now(t *testing.T) time.Time {
	t.Helper()
	var now time.Time
	if !w.srv.RunScript(t, w.player.ObjectID(), "Ind1Director", func(s *script.Script, _ *script.Player) { now = s.Now() }) {
		t.Fatal("the clock read panicked")
	}
	return now
}

// keyBoxes returns the Key Boxes in the world.
func (w *ind1World) keyBoxes() []*npc.Folk {
	var out []*npc.Folk
	for _, obj := range w.srv.State.Objects() {
		if f, ok := obj.(*npc.Folk); ok && int32(f.Instance.Template.ID) == strongKeyBoxID && !f.Decayed() {
			out = append(out, f)
		}
	}
	return out
}

// TestRoyalRushStrongManBuffsItselfAsItAppears ports the strong men's
// created hook: each looks around, wants its self buff on itself with
// weight 1000000 and runs the Warrior's created hook (its ShoutMsg1 line).
// The look sees the character in its territory and turns on it, weight 200.
func TestRoyalRushStrongManBuffsItselfAsItAppears(t *testing.T) {
	t.Parallel()
	ai := npc.AIParams{"ShoutMsg1": "1000286"}
	w := bootStrongMen(t, strongManTemplate(strongManID, ai), strongManTemplate(strongMan2ID, ai))
	for _, id := range []int32{strongManID, strongMan2ID} {
		g := w.spawnFrames(t, id, 150, int(id-strongManID)*100)
		n := g.h
		if got := says(g.frames, n.ObjectID()); len(got) != 1 {
			t.Fatalf("%d: created shouts = %q, want the Warrior's one", id, got)
		}
		w.wantCasts(t, "created", n, map[modelskill.ID]ind1Cast{strongSelfBuffID: {n.ObjectID(), 1000000}})
		if got := n.AggroHate(w.player); got != 200 {
			t.Fatalf("%d: hate of the character its look saw = %v, want 200", id, got)
		}
	}
}

// TestRoyalRushStrongManLeavesAtMinute55 ports the strong men's timer
// 6001: 60 s after it appears and every 60 s after that, a strong man
// leaves the world, with no corpse, once the local clock's minute is 55 or
// more. A firing at minute 54 keeps it.
func TestRoyalRushStrongManLeavesAtMinute55(t *testing.T) {
	t.Parallel()
	w := bootStrongMen(t, strongManTemplate(strongManID, nil))
	if !w.srv.DrivesClock() {
		t.Fatal("setup: the test does not drive the clock")
	}
	now := w.now(t)
	at := time.Date(now.Year(), now.Month(), now.Day(), now.Hour(), 53, 30, 0, now.Location())
	if !at.After(now) {
		at = at.Add(time.Hour)
	}
	w.srv.Advance(t, at.Sub(now))
	w.srv.ReadQueued(t, w.srv.Client)
	n := w.spawn(t, strongManID, 1500, 0)
	id := n.ObjectID()

	w.srv.Advance(t, 60*time.Second)
	if got := w.now(t); got.Minute() != 54 {
		t.Fatalf("setup: the first firing's minute = %d, want 54", got.Minute())
	}
	if !w.inWorld(n) {
		t.Fatal("the strong man left the world at minute 54")
	}
	w.srv.ReadQueued(t, w.srv.Client)

	w.srv.Advance(t, 60*time.Second)
	frames := w.srv.ReadQueued(t, w.srv.Client)
	if got := w.now(t); got.Minute() != 55 {
		t.Fatalf("setup: the second firing's minute = %d, want 55", got.Minute())
	}
	if w.inWorld(n) {
		t.Fatal("the strong man is still in the world at minute 55")
	}
	if got := framesOf(frames, serverpackets.OpcodeDeleteObject, id); len(got) != 1 {
		t.Fatalf("DeleteObject frames of the strong man = %d, want 1", len(got))
	}
}

// TestRoyalRushStrongManStrikesWhatItSees ports the strong men's
// see-creature hook. Settled (life time past 7), in its territory and not
// attacking, a strong man seeing its top desire target wants its first
// physical special on it, weight 1000000, on a draw under 33; then, like
// on any sight of a playable in its territory, it turns on it, weight 200.
// On a draw of 33, or with no top desire target, it only turns on it; out
// of its territory it does neither.
func TestRoyalRushStrongManStrikesWhatItSees(t *testing.T) {
	t.Parallel()
	for _, row := range []struct {
		name    string
		draw    int
		top     bool
		outside bool
		casts   bool
	}{
		{"top desire target, draw 0", 0, true, false, true},
		{"top desire target, draw 33", 33, true, false, false},
		{"no top desire target", 0, false, false, false},
		{"out of its territory", 0, true, true, false},
	} {
		t.Run(row.name, func(t *testing.T) {
			t.Parallel()
			w := bootStrongMen(t, strongManTemplate(strongManID, nil))
			n := w.spawn(t, strongManID, 1500, 0)
			w.ticks(t, 9)
			if n.LifeTime() <= 7 {
				t.Fatalf("setup: life time %d, want past 7", n.LifeTime())
			}
			if got := n.AggroHate(w.player); got != 0 {
				t.Fatalf("setup: the strong man saw the character from afar: hate %v", got)
			}
			if row.outside {
				w.teleport(t, n, -1350, 0)
				if script.NPCOf(n).InMyTerritory() {
					t.Fatal("setup: the strong man is still in its territory")
				}
			}
			w.clear(t, n)
			if row.top {
				w.setTop(t, n, w.charOf(t))
			}
			w.draws.set(row.draw)
			w.lookAt(t, n)
			me := w.player.ObjectID()
			var want map[modelskill.ID]ind1Cast
			if row.casts {
				want = map[modelskill.ID]ind1Cast{strongSpecial1ID: {me, 1000000}}
			}
			w.wantCasts(t, "after it saw the character", n, want)
			wantHate := 200.0
			if row.outside {
				wantHate = 0
			}
			if got := n.AggroHate(w.player); got != wantHate {
				t.Fatalf("hate of the character it sees = %v, want %v", got, wantHate)
			}
		})
	}
}

// lookAt moves the character 100 in front of h and runs one AI cycle,
// whose see-creature scan notices it.
func (w *ind1World) lookAt(t *testing.T, h *npc.Hostile) {
	t.Helper()
	obj, ok := w.srv.State.Player(w.player.ObjectID())
	if !ok {
		t.Fatal("the character is not in the world")
	}
	x, y, z := h.Position()
	if err := w.srv.State.Move(obj, x-100, y, z); err != nil {
		t.Fatalf("approach: %v", err)
	}
	w.ticks(t, 1)
}

// TestRoyalRushStrongManStrikesItsAttackers ports the strong men's
// attacked hook. Hit by a playable that is its top desire target, a strong
// man wants its first physical special on it, weight 1000000, on a draw
// under 33. Hit by a playable while its top desire target is another
// creature, on a draw under 33 the first strong man wants its first self
// range DD magic on itself, the second its first DD magic on that
// creature. On draws of 33 neither does anything.
func TestRoyalRushStrongManStrikesItsAttackers(t *testing.T) {
	t.Parallel()
	for _, row := range []struct {
		name  string
		id    int32
		stray modelskill.ID
		self  bool
	}{
		{"first", strongManID, strongSelfRangeID, true},
		{"second", strongMan2ID, strongDDMagicID, false},
	} {
		t.Run(row.name, func(t *testing.T) {
			t.Parallel()
			w := bootStrongMen(t, strongManTemplate(row.id, nil), warriorTemplate(unboundID, nil))
			n := w.spawn(t, row.id, 300, 0)
			foe := w.spawn(t, unboundID, 300, 300)
			me := w.player.ObjectID()

			w.setTop(t, n, w.charOf(t))
			w.hit(t, n, 10)
			w.wantCasts(t, "after a hit by its top desire target", n, map[modelskill.ID]ind1Cast{strongSpecial1ID: {me, 1000000}})

			w.setTop(t, n, w.charOf(t))
			w.draws.set(33)
			w.hit(t, n, 10)
			w.wantCasts(t, "after a hit by its top desire target, draws 33", n, nil)

			w.setTop(t, n, script.NPCOf(foe))
			w.draws.set(33)
			w.hit(t, n, 10)
			w.wantCasts(t, "after a hit while it fights another creature, draws 33", n, nil)

			w.setTop(t, n, script.NPCOf(foe))
			w.draws.set(0)
			w.hit(t, n, 10)
			target := foe.ObjectID()
			if row.self {
				target = n.ObjectID()
			}
			w.wantCasts(t, "after a hit while it fights another creature", n, map[modelskill.ID]ind1Cast{row.stray: {target, 1000000}})
		})
	}
}

// TestRoyalRushStrongManAssistsItsClan ports the strong men's clan-attacked
// hook: a settled clan member that is not attacking wants its first
// physical special on a playable attacking another member, weight 1000000,
// on a draw under 33. On a draw of 33, or young, it does not.
func TestRoyalRushStrongManAssistsItsClan(t *testing.T) {
	t.Parallel()
	for _, row := range []struct {
		name  string
		draw  int
		ticks int
		casts bool
	}{
		{"settled, draw 0", 0, 8, true},
		{"settled, draw 33", 33, 8, false},
		{"young", 0, 0, false},
	} {
		t.Run(row.name, func(t *testing.T) {
			t.Parallel()
			w := bootStrongMen(t, strongManTemplate(strongManID, nil), strongManTemplate(strongManTwoID, nil))
			victim := w.spawn(t, strongManID, 1500, 0)
			member := w.spawn(t, strongManTwoID, 1500, 100)
			w.ticks(t, row.ticks)
			if settled := member.LifeTime() > 7; settled != (row.ticks > 0) {
				t.Fatalf("setup: member life time = %d", member.LifeTime())
			}
			w.clear(t, member)
			w.draws.set(row.draw)
			w.hit(t, victim, 10)
			var want map[modelskill.ID]ind1Cast
			if row.casts {
				want = map[modelskill.ID]ind1Cast{strongSpecial1ID: {w.player.ObjectID(), 1000000}}
			}
			w.wantCasts(t, "member after the hit", member, want)
		})
	}
}

// TestRoyalRushStrongManLeavesAKeyBox ports the strong men's my-dying
// hook: a strong man that dies leaves a Key Box (31455) as its private
// where it died, then runs the Warrior's my-dying hook (its ShoutMsg4
// line on a draw under 30). The Key Box stays once the corpse is gone.
func TestRoyalRushStrongManLeavesAKeyBox(t *testing.T) {
	t.Parallel()
	w := bootStrongMen(t, strongManTemplate(strongMan2ID, npc.AIParams{"ShoutMsg4": "1000288"}))
	n := w.spawn(t, strongMan2ID, 1500, 0)
	where := positionOf(n)
	w.kill(t, n)
	if got := w.keyBoxes(); len(got) != 0 {
		t.Fatalf("Key Boxes as the strong man dies = %d, want none yet", len(got))
	}
	w.srv.Advance(t, 3500*time.Millisecond)
	frames := w.srv.ReadQueued(t, w.srv.Client)
	boxes := w.keyBoxes()
	if len(boxes) != 1 {
		t.Fatalf("Key Boxes after the strong man died = %d, want 1", len(boxes))
	}
	box := boxes[0]
	if at := ind8At(box); at.X != where.X || at.Y != where.Y {
		t.Fatalf("Key Box at %v, want where the strong man died, %v", at, where)
	}
	if m := script.NPCOf(box).Master(); m == nil || m.ObjectID() != n.ObjectID() {
		t.Fatalf("Key Box master = %v, want the strong man %d", m, n.ObjectID())
	}
	if got := says(frames, n.ObjectID()); len(got) != 1 {
		t.Fatalf("dying shouts = %q, want the Warrior's one", got)
	}
	if infos := framesOf(frames, serverpackets.OpcodeNPCInfo, box.ObjectID()); len(infos) != 2 {
		t.Fatalf("NpcInfo frames of the Key Box = %d, want 2 (appears walking, then running)", len(infos))
	}

	w.onQueue(t, n, func() { n.Decay(w.srv.State, w.srv.NpcSpawns.RespawnHook(n.ObjectID())) })
	if w.inWorld(n) {
		t.Fatal("setup: the corpse is still in the world")
	}
	if got := w.keyBoxes(); len(got) != 1 || got[0] != box {
		t.Fatalf("Key Boxes after the corpse decayed = %d, want the one", len(got))
	}
}
