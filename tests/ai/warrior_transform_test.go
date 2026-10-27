package ai

import (
	"testing"

	"github.com/rs/zerolog"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/npc"
	"github.com/fatal10110/acis_golang/internal/gameserver/script"
)

// liveNPCs returns the NPCs of template id in the world.
func (w *warriorWorld) liveNPCs(id int32) []*npc.Hostile {
	var out []*npc.Hostile
	for _, obj := range w.srv.State.Objects() {
		if h, ok := obj.(*npc.Hostile); ok && int32(h.Instance.Template.ID) == id && !h.Decayed() {
			out = append(out, h)
		}
	}
	return out
}

// deleteMe deletes h on its own queue, as a script's DeleteMe does.
func (w *warriorWorld) deleteMe(t *testing.T, h *npc.Hostile) {
	t.Helper()
	if !h.Queue().Post(h.DeleteNow) {
		t.Fatalf("npc %d's queue is closed", h.ObjectID())
	}
	w.srv.Settle(t)
}

// TestWarriorTransformSuccessorOutlivesMaster runs the Warrior base's
// transformation: a hit that finds a Warrior with IsTransform between 70%
// and 50% of its HP (the draw under 30 wins with every draw 0) spawns its
// step1 NPC as its private and deletes itself. The successor stays in the
// world after its master leaves, and its created hook turns it on the
// attacker with weight 500.
func TestWarriorTransformSuccessorOutlivesMaster(t *testing.T) {
	t.Parallel()
	w := bootWarriors(t,
		warriorTemplate(warriorID, npc.AIParams{"IsTransform": "1", "step1": "20003"}),
		warriorTemplate(warriorTwoID, nil),
	)
	master := w.spawn(t, warriorID, 150, 0)

	// The hook reads the HP the hit found, as the reference raises it
	// before the damage lands: a first hit leaves the warrior at 60%, the
	// second one finds it there.
	w.hit(t, master, master.MaxHP()*2/5)
	if master.Decayed() {
		t.Fatal("the warrior transformed on a hit that found it at full HP")
	}
	w.hit(t, master, 1)
	if !master.Decayed() {
		t.Fatal("the transforming warrior is still in the world")
	}
	successors := w.liveNPCs(warriorTwoID)
	if len(successors) != 1 {
		t.Fatalf("successors in the world = %d, want 1", len(successors))
	}
	successor := successors[0]
	if _, ok := w.srv.State.Object(successor.ObjectID()); !ok {
		t.Fatal("the successor left the world with its master")
	}
	if got := successor.Scratch().Int(npc.IntParam3); got != 1 {
		t.Fatalf("successor's step = %d, want 1", got)
	}
	if got := successor.AggroHate(w.player); got < 500 {
		t.Fatalf("successor's hate of the attacker = %v, want at least 500", got)
	}
}

// TestPrivatesOutliveTheirMastersDeleteMe pins the privates' despawn: a
// master's deleteMe leaves its privates in the world, whether a behavior
// binds the master or not.
func TestPrivatesOutliveTheirMastersDeleteMe(t *testing.T) {
	t.Parallel()
	const unboundPrivateID = int32(29991)
	w := bootWarriors(t,
		warriorTemplate(warriorID, nil), warriorTemplate(warriorTwoID, nil),
		warriorTemplate(unboundID, nil), warriorTemplate(unboundPrivateID, nil),
	)
	spawner := script.NewSpawner(w.srv.NpcSpawns, zerolog.Nop())
	bound := w.spawn(t, warriorID, 150, 0)
	unbound := w.spawn(t, unboundID, -150, 0)
	at := func(dx int) script.Loc { return script.Loc{X: w.at.X + dx, Y: w.at.Y + 100, Z: w.at.Z} }
	boundPrivate := spawner.CreateOnePrivateEx(script.NPCOf(bound), warriorTwoID, at(150), 0, script.Params{})
	unboundPrivate := spawner.CreateOnePrivateEx(script.NPCOf(unbound), unboundPrivateID, at(-150), 0, script.Params{})
	if boundPrivate == nil || unboundPrivate == nil {
		t.Fatalf("private handles = %v, %v; want both", boundPrivate, unboundPrivate)
	}
	w.srv.Settle(t)
	if len(bound.Minions()) != 1 || len(unbound.Minions()) != 1 {
		t.Fatalf("masters' privates = %d, %d; want one each", len(bound.Minions()), len(unbound.Minions()))
	}

	w.deleteMe(t, unbound)
	w.deleteMe(t, bound)
	if unboundPrivate.Decayed() {
		t.Fatal("the unbound master's private left the world with it")
	}
	if boundPrivate.Decayed() {
		t.Fatal("the bound master's private left the world with it")
	}
}
