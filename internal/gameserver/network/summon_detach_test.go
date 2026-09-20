package network

import (
	"testing"

	"github.com/rs/zerolog"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/npc"
	petmodel "github.com/fatal10110/acis_golang/internal/gameserver/model/actor/pet"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/summon"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/item"
	"github.com/fatal10110/acis_golang/internal/gameserver/task"
	"github.com/fatal10110/acis_golang/internal/gameserver/world"
	"github.com/fatal10110/acis_golang/internal/testsupport"
)

const (
	detachTestNPCID      = 12077
	detachTestCollarID   = item.AdenaID
	detachTestCollarObjI = 7001
	detachTestCollarObjD = 7002
)

func detachTestPetTemplate() *npc.Template {
	stats := npc.PetLevelStats{
		MaxExp: 100, MaxHP: 60, MaxMP: 40,
		PAtk: 10, PDef: 10, MAtk: 5, MDef: 5,
		MaxMeal: 100, MealInNormal: 1, MealInBattle: 2,
	}
	return &npc.Template{
		ID: detachTestNPCID, Name: "TestPet", Level: 1,
		STR: 40, CON: 40, DEX: 30, INT: 20, WIT: 20, MEN: 20,
		BaseAttackRange: 40, AtkSpd: 300, RunSpeed: 120, WalkSpeed: 60,
		CollisionRadius: 8, CollisionHeight: 20,
		Pet: &npc.PetData{
			AutoFeedLimit: 0.55, HungryLimit: 0.3, UnsummonLimit: 0.1,
			Levels: map[int]npc.PetLevelStats{1: stats, 2: stats},
		},
	}
}

// TestSpawnRestoredPetStopsOnceOwnerDetached pins what a pets-row restore
// continuation may still do after its owner has logged out. sim.Queue refuses
// later posts but runs every task it already accepted, so a continuation
// queued just before detachLivePlayer closed the queue still runs — and
// logout leaves the inventory in place, so the control-item check alone does
// not stop it. Publishing a pet there would leave one in the world for an
// offline owner, past the cleanup that would have removed it
// (Player.cleanup aborts the cast and unsummons, Player.java:6266-6283).
//
// The in-world half of the test runs first so the detached half cannot pass
// for the wrong reason: the same call has to publish a pet while the owner is
// still there.
func TestSpawnRestoredPetStopsOnceOwnerDetached(t *testing.T) {
	state := world.New()
	npcTmpl := detachTestPetTemplate()
	summonItems, err := item.NewSummonItemTable([]item.SummonItem{
		{ItemID: detachTestCollarID, NPCID: detachTestNPCID, SummonType: summonItemTypePet},
	})
	if err != nil {
		t.Fatalf("build summon item table: %v", err)
	}
	summonItem, ok := summonItems.Item(detachTestCollarID)
	if !ok {
		t.Fatal("missing summon item fixture")
	}
	instances := task.NewItemInstances(nil, testItemTemplates(), nil, nil)
	link := &GameClientLink{
		itemInstances: instances,
		world:         state,
		npcs:          npc.NewTable([]*npc.Template{npcTmpl}),
		summonItems:   summonItems,
		itemTemplates: testItemTemplates(),
		ids:           &sequentialIDs{next: 9000},
		log:           zerolog.Nop(),
	}

	spawnFor := func(t *testing.T, id, collarObjectID int32, detach bool) (*livePlayer, bool) {
		t.Helper()
		live := newTestLivePlayer(t, id, &testsupport.FrameCapture{})
		state.Spawn(live, int(id)*100, 0, 0, 0)
		state.AddPlayer(live)
		collar := live.Inventory().AddNew(detachTestCollarID, 1, collarObjectID)
		live.Inventory().DrainUpdates()
		if detach {
			link.detachLivePlayer(live)
		}
		(&gameSummonSpawner{link: link, live: live}).spawnRestoredPet(collar, summonItem, npcTmpl, petmodel.State{}, false)
		_, spawned := state.Summon(live.ObjectID())
		return live, spawned
	}

	owner, spawned := spawnFor(t, 11, detachTestCollarObjI, false)
	if !spawned {
		t.Fatal("no pet published for an in-world owner: the fixture is not reaching the spawn")
	}
	// The pet's inventory is built with its persistence dependency, under
	// the pet's own object id as owner.
	obj, _ := state.Summon(owner.ObjectID())
	petInv := obj.(*summon.Actor).PetInventory()
	loot := petInv.AddNew(item.AdenaID, 5, 7100)
	if loot == nil || !instances.Contains(loot) {
		t.Fatal("pet inventory mutation did not reach the item persistence task")
	}
	// Internal consistency only, not reference parity: Java keys pet items on
	// the player's id, Go on the pet's (tracked in the pet-inventory owner id
	// issue).
	// The recorded owner is the write's lane key; it must be the id the
	// teardown flush enqueues on (flushItemPersistence uses inv.OwnerID()).
	if owner, _ := instances.PendingOwner(loot.ObjectID); owner != petInv.OwnerID() {
		t.Errorf("pet item lane owner = %d, want the pet inventory's owner %d", owner, petInv.OwnerID())
	}
	petInv.ReleasePersistence()
	instances.RemoveItems([]*item.Instance{loot})
	loot.AddCount(1)
	if instances.Contains(loot) {
		t.Error("pet item scheduled a write after the inventory was released")
	}
	if _, spawned := spawnFor(t, 12, detachTestCollarObjD, true); spawned {
		t.Fatal("pet published for an owner that had already detached")
	}
}
