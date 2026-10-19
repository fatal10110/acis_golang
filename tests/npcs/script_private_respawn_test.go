package npcs

import (
	"fmt"
	"math"
	"slices"
	"testing"
	"time"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/npc"
	"github.com/fatal10110/acis_golang/internal/gameserver/script"
)

// diePrivate kills the private h, asks its spawn to bring it back after
// request when request is positive (as a party-died hook does at the
// death), then decays its corpse.
func (w *scriptSpawnWorld) diePrivate(t *testing.T, h *npc.Hostile, request time.Duration) {
	t.Helper()
	if !h.Die(nil, nil) {
		t.Fatalf("private %d did not die", h.ObjectID())
	}
	w.spawner.ScheduleRespawn(script.NPCOf(h), request)
	onQueueOf(t, h, func() { h.Decay(w.srv.State, w.srv.NpcSpawns.RespawnHook(h.ObjectID())) })
	w.srv.Settle(t)
}

// armedPrivates returns the slot keys of the script privates with a
// respawn pending. The harness's respawn task never fires on its own, and
// runs on the wall clock: the delay itself is pinned by the population's
// own test.
func (w *scriptSpawnWorld) armedPrivates() []string {
	var armed []string
	for i := 1; i <= 64; i++ {
		if key := fmt.Sprintf("private#%d", i); w.srv.NpcRespawns.Tracked(key) {
			armed = append(armed, key)
		}
	}
	return armed
}

// respawnArmed brings the one armed script private's respawn due.
func (w *scriptSpawnWorld) respawnArmed(t *testing.T) {
	t.Helper()
	armed := w.armedPrivates()
	if len(armed) != 1 {
		t.Fatalf("script privates with a respawn pending = %q, want one", armed)
	}
	w.srv.NpcRespawns.Cancel(armed[0])
	w.srv.NpcSpawns.Respawn(armed[0])
	w.srv.Settle(t)
}

// livePups returns the pups in the world.
func (w *scriptSpawnWorld) livePups() []*npc.Hostile {
	var out []*npc.Hostile
	for _, obj := range w.srv.State.Objects() {
		if h, ok := obj.(*npc.Hostile); ok && h.Instance.Template.ID == spawnPupID {
			out = append(out, h)
		}
	}
	return out
}

// A private created at a point of its own with no respawn delay, asked
// back at its death, comes back at that point once the request is due:
// facing the asked way, on the ground, with its spawn parameters. Its
// decay let it go, so it comes back as no private of its master's, yet
// still spawned for it. The handle the script kept acts on the new life.
func TestScriptPlacedOneTimePrivateComesBackAtItsPoint(t *testing.T) {
	t.Parallel()
	w := bootScriptSpawns(t)
	master := w.spawner.AddSpawn(spawnWolfID, script.Loc{X: w.at.X + 50, Y: w.at.Y, Z: w.at.Z}, false, 0)
	boss := spawnedNPC(t, w, spawnWolfID).(*npc.Hostile)
	placed := w.spawner.CreateOnePrivateEx(master, spawnPupID, script.Loc{X: w.at.X + 80, Y: w.at.Y + 10, Z: w.at.Z, Heading: 500}, 0, script.Params{P1: 7, P2: 8, P3: 9})
	pup := spawnedNPC(t, w, spawnPupID).(*npc.Hostile)
	drainFrames(t, w.c)

	w.diePrivate(t, pup, 5*time.Second)
	if pup.Master() != nil || slices.Contains(boss.Minions(), npc.PartyMember(pup)) {
		t.Fatal("the decayed one-time private is still its master's")
	}
	w.respawnArmed(t)
	frames := drainFrames(t, w.c)

	back := spawnedNPC(t, w, spawnPupID).(*npc.Hostile)
	if back.ObjectID() == pup.ObjectID() {
		t.Fatal("the respawn kept the decayed pup's object id")
	}
	if x, y, z, heading := npcInfoAt(t, frames, back.ObjectID()); x != int32(w.at.X+80) || y != int32(w.at.Y+10) || z != int32(w.at.Z-groundBelow) || heading != 500 {
		t.Fatalf("respawned pup shown at %d,%d,%d heading %d; want its own point on the ground, heading 500", x, y, z, heading)
	}
	if back.Master() != nil || slices.Contains(boss.Minions(), npc.PartyMember(back)) || back.SpawnMaster() != boss {
		t.Fatalf("respawned pup: master %v, among the master's privates %v, spawned for %v; want no master, not listed, spawned for the boss",
			back.Master(), slices.Contains(boss.Minions(), npc.PartyMember(back)), back.SpawnMaster())
	}
	s := back.Scratch()
	if a, b, c := s.Int(npc.IntParam1), s.Int(npc.IntParam2), s.Int(npc.IntParam3); a != 7 || b != 8 || c != 9 {
		t.Fatalf("respawned pup params = %d, %d, %d; want 7, 8, 9", a, b, c)
	}
	if placed.Decayed() || placed.ObjectID() != back.ObjectID() {
		t.Fatalf("kept handle: decayed %v, id %d; want the respawned pup %d", placed.Decayed(), placed.ObjectID(), back.ObjectID())
	}
}

// A one-time private created around its master and asked back comes back
// around its live master. One nobody asks back is gone for good: its slot
// goes at its decay, and a request made after that asks nothing. A request
// that comes due once the master has died brings nothing back either.
func TestScriptOneTimePrivateRespawnRules(t *testing.T) {
	t.Parallel()
	w := bootScriptSpawns(t)
	master := w.spawner.AddSpawn(spawnWolfID, script.Loc{X: w.at.X + 50, Y: w.at.Y, Z: w.at.Z}, false, 0)
	boss := spawnedNPC(t, w, spawnWolfID).(*npc.Hostile)

	asked := w.spawner.CreateOnePrivate(master, spawnPupID, 0)
	pup := spawnedNPC(t, w, spawnPupID).(*npc.Hostile)
	w.diePrivate(t, pup, 3*time.Second)
	w.respawnArmed(t)
	back := spawnedNPC(t, w, spawnPupID).(*npc.Hostile)
	if back.ObjectID() == pup.ObjectID() || asked.ObjectID() != back.ObjectID() {
		t.Fatalf("asked pup back as %d (was %d, handle %d), want a new life the handle follows", back.ObjectID(), pup.ObjectID(), asked.ObjectID())
	}
	bx, by, _ := boss.Position()
	px, py, _ := back.Position()
	if d := math.Hypot(float64(px-bx), float64(py-by)); !inPrivateRing(d, 40, 120) {
		t.Fatalf("respawned pup %.2f from its master; want 40 to 120", d)
	}

	w.diePrivate(t, back, 0)
	w.spawner.ScheduleRespawn(asked, time.Second)
	if armed := w.armedPrivates(); len(armed) != 0 {
		t.Fatalf("a request after the decay of a pup nobody asked back armed %q", armed)
	}
	if got := len(w.livePups()); got != 0 {
		t.Fatalf("pups after a death nobody asked back = %d, want none", got)
	}

	w.spawner.CreateOnePrivate(master, spawnPupID, 0)
	orphan := spawnedNPC(t, w, spawnPupID).(*npc.Hostile)
	w.diePrivate(t, orphan, 3*time.Second)
	if !boss.Die(nil, nil) {
		t.Fatal("the master did not die")
	}
	w.respawnArmed(t)
	if got := len(w.livePups()); got != 0 {
		t.Fatalf("pups after the master died = %d, want none", got)
	}
}
