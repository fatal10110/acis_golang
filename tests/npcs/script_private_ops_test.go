package npcs

import (
	"math"
	"slices"
	"testing"
	"time"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/npc"
	"github.com/fatal10110/acis_golang/internal/gameserver/script"
)

// captainWithPrivates spawns the captain by a script and has it create the
// privates its template declares: a private (30 s to respawn) and a scout
// (40 s).
func (w *scriptSpawnWorld) captainWithPrivates(t *testing.T) (*script.NPC, *npc.Hostile) {
	t.Helper()
	master := w.spawner.AddSpawn(spawnCaptainID, script.Loc{X: w.at.X + 50, Y: w.at.Y, Z: w.at.Z}, false, 0)
	w.spawner.CreatePrivates(master)
	w.srv.Settle(t)
	drainFrames(t, w.c)
	return master, spawnedNPC(t, w, spawnCaptainID).(*npc.Hostile)
}

// livePrivate returns one live private of boss's.
func livePrivate(t *testing.T, boss *npc.Hostile) *npc.Hostile {
	t.Helper()
	for _, m := range boss.Minions() {
		if h, ok := m.(*npc.Hostile); ok && !h.Dead() && !h.Decayed() {
			return h
		}
	}
	t.Fatal("the captain has no live private")
	return nil
}

// A decayed private whose spawn declares a respawn delay is brought back at
// once by DoRespawn while its master lives: around the master, again one
// of its privates, as a new life the handle follows. A private whose
// corpse has not decayed only loses the respawn asked for it. Once the
// master is dead DoRespawn brings nothing back.
func TestScriptDoRespawnOfAPrivate(t *testing.T) {
	t.Parallel()
	w := bootScriptSpawns(t)
	_, boss := w.captainWithPrivates(t)

	private := livePrivate(t, boss)
	handle := script.NPCOf(private)
	w.diePrivate(t, private, 0)
	w.spawner.DoRespawn(handle)
	w.srv.Settle(t)
	back := handle.ObjectID()
	if back == private.ObjectID() || handle.Decayed() {
		t.Fatalf("private after DoRespawn: id %d (was %d), decayed %v; want a new live private", back, private.ObjectID(), handle.Decayed())
	}
	live := w.liveObject(t, back).(*npc.Hostile)
	if live.Master() != npc.PartyMember(boss) || !slices.Contains(boss.Minions(), npc.PartyMember(live)) {
		t.Fatal("the respawned private is not the captain's private again")
	}
	bx, by, _ := boss.Position()
	px, py, _ := live.Position()
	if d := math.Hypot(float64(px-bx), float64(py-by)); !inPrivateRing(d, 40, 120) {
		t.Fatalf("respawned private %.2f from its master; want 40 to 120", d)
	}

	corpse := livePrivate(t, boss)
	if !corpse.Die(nil, nil) {
		t.Fatal("the private did not die")
	}
	w.spawner.ScheduleRespawn(script.NPCOf(corpse), 5*time.Second)
	if armed := w.armedPrivates(); len(armed) != 1 {
		t.Fatalf("respawns pending after the request = %q, want one", armed)
	}
	w.spawner.DoRespawn(script.NPCOf(corpse))
	if armed := w.armedPrivates(); len(armed) != 0 {
		t.Fatalf("respawns pending after DoRespawn on the corpse = %q, want none", armed)
	}
	if !corpse.Dead() || corpse.Decayed() {
		t.Fatal("DoRespawn changed the private's undecayed corpse")
	}

	gone := livePrivate(t, boss)
	goneHandle := script.NPCOf(gone)
	w.diePrivate(t, gone, 0)
	if !boss.Die(nil, nil) {
		t.Fatal("the captain did not die")
	}
	w.spawner.DoRespawn(goneHandle)
	w.srv.Settle(t)
	if !goneHandle.Decayed() || goneHandle.ObjectID() != gone.ObjectID() {
		t.Fatal("DoRespawn brought a private back for a dead master")
	}
}

// A private's spawn gives the point the private was created at, or a new
// point around its master facing the master's way.
func TestScriptDrawSpawnLocationOfPrivates(t *testing.T) {
	t.Parallel()
	w := bootScriptSpawns(t)
	master, boss := w.captainWithPrivates(t)
	around := script.NPCOf(livePrivate(t, boss))
	placed := w.spawner.CreateOnePrivateEx(master, spawnWolfID, script.Loc{X: w.at.X + 80, Y: w.at.Y + 10, Z: w.at.Z, Heading: 500}, 0, script.Params{})
	if got := w.spawner.DrawSpawnLocation(placed); got.X != w.at.X+80 || got.Y != w.at.Y+10 || got.Heading != 500 {
		t.Fatalf("placed private spawn location = %+v, want its point, heading 500", got)
	}
	bx, by, _ := boss.Position()
	for range 10 {
		got := w.spawner.DrawSpawnLocation(around)
		if d := math.Hypot(float64(got.X-bx), float64(got.Y-by)); !inPrivateRing(d, 40, 120) || got.Heading != boss.Heading() {
			t.Fatalf("private spawn location %+v: %.2f from its master, heading %d; want 40 to 120, heading %d", got, d, got.Heading, boss.Heading())
		}
	}
}

// A script that sets a private's respawn delay to 0 makes it a one-time
// private: at its decay it leaves its master's privates, while a private
// keeping its declared delay stays one of them. A negative delay reads 0;
// a delay set on an NPC that is no private panics.
func TestScriptSetRespawnDelayOfAPrivate(t *testing.T) {
	t.Parallel()
	w := bootScriptSpawns(t)
	master, boss := w.captainWithPrivates(t)

	kept := livePrivate(t, boss)
	if got := w.spawner.RespawnDelay(script.NPCOf(kept)); got != 30 && got != 40 {
		t.Fatalf("private respawn delay = %d, want its declared 30 or 40", got)
	}
	w.diePrivate(t, kept, 0)
	if !slices.Contains(boss.Minions(), npc.PartyMember(kept)) {
		t.Fatal("a decayed private keeping its respawn delay left its master's privates")
	}

	oneTime := livePrivate(t, boss)
	w.spawner.SetRespawnDelay(script.NPCOf(oneTime), -5)
	if got := w.spawner.RespawnDelay(script.NPCOf(oneTime)); got != 0 {
		t.Fatalf("private respawn delay after setting -5 = %d, want 0", got)
	}
	w.diePrivate(t, oneTime, 0)
	if oneTime.Master() != nil || slices.Contains(boss.Minions(), npc.PartyMember(oneTime)) {
		t.Fatal("a decayed private whose respawn delay a script set to 0 is still its master's private")
	}

	defer func() {
		if recover() == nil {
			t.Fatal("setting the respawn delay of a master that is no private did not panic")
		}
	}()
	w.spawner.SetRespawnDelay(master, 0)
}
