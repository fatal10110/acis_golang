package npcs

import (
	"fmt"
	"slices"
	"testing"
	"time"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/npc"
	"github.com/fatal10110/acis_golang/internal/gameserver/script"
)

// A private that creates a private through the API: the new NPC joins the
// top master's privates, linked to the private that asked for it, and that
// private stays no master. A hit on the asking private calls the new NPC's
// party-attacked hook, and its death the party-died one without letting the
// new NPC go.
func TestScriptPrivateOfPrivateJoinsMasterParty(t *testing.T) {
	t.Parallel()
	log := &partyLog{}
	w := bootCivilianParty(t, log)
	master := w.spawner.AddSpawn(spawnWolfID, script.Loc{X: w.at.X + 50, Y: w.at.Y, Z: w.at.Z}, false, 0)
	wolf := spawnedNPC(t, w, spawnWolfID).(*npc.Hostile)
	asker := w.spawner.CreateOnePrivate(master, spawnTabletID, 0)
	tablet := spawnedNPC(t, w, spawnTabletID).(*npc.Folk)

	knight := w.spawner.CreateOnePrivate(asker, spawnPupID, 0)
	pup := spawnedNPC(t, w, spawnPupID).(*npc.Hostile)

	if got := wolf.Minions(); len(got) != 2 || !slices.Contains(got, npc.PartyMember(tablet)) || !slices.Contains(got, npc.PartyMember(pup)) {
		t.Fatalf("master's privates = %v, want the asking private and the new one", got)
	}
	if got := tablet.Minions(); len(got) != 2 || !slices.Contains(got, npc.PartyMember(pup)) {
		t.Fatalf("asking private's party privates = %v, want its master's", got)
	}
	if tablet.IsMaster() || asker.IsMaster() || !asker.HasMaster() {
		t.Fatalf("asking private master %v (handle %v), has master %v; want a linked non-master", tablet.IsMaster(), asker.IsMaster(), asker.HasMaster())
	}
	if pup.Master() != npc.PartyMember(tablet) || pup.SpawnMaster() != npc.PartyMember(tablet) || !script.Same(knight.Master(), asker) {
		t.Fatalf("new private's master = %v, want the asking private", pup.Master())
	}
	if handles := master.Minions(); len(handles) != 2 || !slices.ContainsFunc(handles, func(h *script.NPC) bool { return script.Same(h, knight) }) {
		t.Fatalf("master handle privates = %v, want the new private among 2", handles)
	}
	if pup.IsMaster() {
		t.Fatal("the new private became a master")
	}

	p := w.liveObject(t, w.player)
	onPlayerQueue(t, w, func() { tablet.TakeDamage(10, p) })
	onPlayerQueue(t, w, func() { tablet.TakeDamage(1_000_000, p) })
	w.srv.Settle(t)
	want := []string{
		fmt.Sprintf("PARTY_ATTACKED caller=%d called=%d damage=10", spawnTabletID, spawnPupID),
		fmt.Sprintf("PARTY_ATTACKED caller=%d called=%d damage=1000000", spawnTabletID, spawnPupID),
		fmt.Sprintf("PARTY_DIED caller=%d called=%d", spawnTabletID, spawnPupID),
	}
	if got := log.take(); !slices.Equal(got, want) {
		t.Fatalf("party hooks = %q, want %q", got, want)
	}
	if !tablet.Dead() || pup.Master() == nil || !knight.HasMaster() {
		t.Fatalf("asking private dead %v; new private still linked %v, want linked", tablet.Dead(), pup.Master() != nil)
	}
	if !slices.Contains(wolf.Minions(), npc.PartyMember(pup)) {
		t.Fatal("the asking private's death took the new private out of the master's privates")
	}
}

// The asking private decaying (its despawn time) leaves the master's
// privates, as a one-time private does, and takes nothing of the master's
// party with it: the private it created stays in the world, still the
// master's.
func TestScriptPrivateOfPrivateOutlivesAskerDecay(t *testing.T) {
	t.Parallel()
	w := bootCivilianParty(t, &partyLog{})
	master := w.spawner.AddSpawn(spawnWolfID, script.Loc{X: w.at.X + 50, Y: w.at.Y, Z: w.at.Z}, false, 0)
	wolf := spawnedNPC(t, w, spawnWolfID).(*npc.Hostile)
	asker := w.spawner.CreateOnePrivate(master, spawnTabletID, 10*time.Second)
	tablet := spawnedNPC(t, w, spawnTabletID).(*npc.Folk)
	knight := w.spawner.CreateOnePrivate(asker, spawnPupID, 0)
	pup := spawnedNPC(t, w, spawnPupID).(*npc.Hostile)

	w.srv.Advance(t, 10*time.Second)
	if !asker.Decayed() {
		t.Fatal("the asking private did not leave at its despawn time")
	}
	if knight.Decayed() || master.Decayed() {
		t.Fatalf("new private gone %v, master gone %v; want both in the world", knight.Decayed(), master.Decayed())
	}
	if got := wolf.Minions(); slices.Contains(got, npc.PartyMember(tablet)) || !slices.Contains(got, npc.PartyMember(pup)) || len(got) != 1 {
		t.Fatalf("master's privates = %v, want only the new private", got)
	}
}
