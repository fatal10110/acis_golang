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

// A private its master's death let go creates a private of its own, which
// joins its own set. The dead master's corpse decaying takes neither with
// it: a decay deletes no privates, only a spawn's delete does. The new one
// keeps its spawn, and at its own decay it leaves the private that asked
// for it.
func TestScriptPrivateOfUnlinkedPrivateKeepsSpawnAtMasterDecay(t *testing.T) {
	t.Parallel()
	w := bootCivilianParty(t, &partyLog{})
	master := w.spawner.AddSpawn(spawnWolfID, script.Loc{X: w.at.X + 50, Y: w.at.Y, Z: w.at.Z}, false, 0)
	wolf := spawnedNPC(t, w, spawnWolfID).(*npc.Hostile)
	asker := w.spawner.CreateOnePrivate(master, spawnTabletID, 0)
	tablet := spawnedNPC(t, w, spawnTabletID).(*npc.Folk)

	p := w.liveObject(t, w.player)
	onPlayerQueue(t, w, func() { wolf.TakeDamage(1_000_000, p) })
	w.srv.Settle(t)
	if !wolf.Dead() || tablet.Master() != nil {
		t.Fatalf("master dead %v, private still linked %v; want dead and let go", wolf.Dead(), tablet.Master() != nil)
	}

	knight := w.spawner.CreateOnePrivate(asker, spawnPupID, 0)
	pup := spawnedNPC(t, w, spawnPupID).(*npc.Hostile)
	if !tablet.IsMaster() || !slices.Contains(tablet.Minions(), npc.PartyMember(pup)) {
		t.Fatalf("let-go private master %v with privates %v, want the new private in its own set", tablet.IsMaster(), tablet.Minions())
	}

	onQueueOf(t, wolf, func() { wolf.Decay(w.srv.State, w.srv.NpcSpawns.RespawnHook(wolf.ObjectID())) })
	w.srv.Settle(t)
	if asker.Decayed() || knight.Decayed() {
		t.Fatalf("first private gone %v, new private gone %v; want both in the world", asker.Decayed(), knight.Decayed())
	}
	if rec, ok := w.srv.NpcSpawns.SpawnOf(pup.ObjectID()); !ok || rec.MasterID != tablet.ObjectID() {
		t.Fatalf("new private's spawn = %+v (%v), want the first private's private", rec, ok)
	}

	if !pup.Die(nil, nil) {
		t.Fatal("the new private did not die")
	}
	onQueueOf(t, pup, func() { pup.Decay(w.srv.State, w.srv.NpcSpawns.RespawnHook(pup.ObjectID())) })
	w.srv.Settle(t)
	if pup.Master() != nil || slices.Contains(tablet.Minions(), npc.PartyMember(pup)) {
		t.Fatalf("decayed one-time private still linked %v, in its master's privates %v", pup.Master() != nil, tablet.Minions())
	}
}

// A private's private sits in the top master's set, linked to the private
// that asked for it. When the asking private, let go by the master's death,
// becomes a master of its own and decays before the master's corpse, it
// takes no private with it: the one it created since and the one it
// created earlier stay in the world, and the earlier one keeps its spawn,
// which still names it.
func TestScriptPrivateInMasterSetKeepsSpawnAtAskerDecay(t *testing.T) {
	t.Parallel()
	w := bootCivilianParty(t, &partyLog{})
	master := w.spawner.AddSpawn(spawnWolfID, script.Loc{X: w.at.X + 50, Y: w.at.Y, Z: w.at.Z}, false, 0)
	wolf := spawnedNPC(t, w, spawnWolfID).(*npc.Hostile)
	asker := w.spawner.CreateOnePrivate(master, spawnTabletID, 0)
	tablet := spawnedNPC(t, w, spawnTabletID).(*npc.Folk)
	knight := w.spawner.CreateOnePrivate(asker, spawnPupID, 0)
	pup := spawnedNPC(t, w, spawnPupID).(*npc.Hostile)

	p := w.liveObject(t, w.player)
	onPlayerQueue(t, w, func() { wolf.TakeDamage(1_000_000, p) })
	w.srv.Settle(t)
	if !wolf.Dead() || tablet.Master() != nil || pup.Master() != nil {
		t.Fatalf("master dead %v, privates still linked %v/%v; want dead and both let go", wolf.Dead(), tablet.Master() != nil, pup.Master() != nil)
	}

	clerkH := w.spawner.CreateOnePrivate(asker, spawnClerkID, 0)
	clerk := spawnedNPC(t, w, spawnClerkID).(*npc.Folk)
	if !tablet.IsMaster() || !slices.Equal(tablet.Minions(), []npc.PartyMember{clerk}) {
		t.Fatalf("let-go private master %v with privates %v, want only the new one", tablet.IsMaster(), tablet.Minions())
	}

	onPlayerQueue(t, w, func() { tablet.TakeDamage(1_000_000, p) })
	w.srv.Settle(t)
	if !tablet.Dead() {
		t.Fatal("the asking private did not die")
	}
	onQueueOf(t, tablet, func() { tablet.Decay(w.srv.State, w.srv.NpcSpawns.RespawnHook(tablet.ObjectID())) })
	w.srv.Settle(t)
	if !asker.Decayed() || clerkH.Decayed() || knight.Decayed() {
		t.Fatalf("asking private gone %v, its own private gone %v, earlier private gone %v; want only the asking one gone",
			asker.Decayed(), clerkH.Decayed(), knight.Decayed())
	}
	if rec, ok := w.srv.NpcSpawns.SpawnOf(pup.ObjectID()); !ok || rec.MasterID != tablet.ObjectID() {
		t.Fatalf("earlier private's spawn = %+v (%v), want the asking private's private", rec, ok)
	}
}
