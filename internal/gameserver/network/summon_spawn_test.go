package network

import (
	"bytes"
	"context"
	"testing"
	"time"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/npc"
	petmodel "github.com/fatal10110/acis_golang/internal/gameserver/model/actor/pet"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/summon"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/item"
	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/worldobject"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/clientpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	skillstate "github.com/fatal10110/acis_golang/internal/gameserver/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/world"
)

type fakePetStoreNoSaved struct{}

func (fakePetStoreNoSaved) Get(context.Context, int32) (petmodel.State, bool, error) {
	return petmodel.State{}, false, nil
}

func (fakePetStoreNoSaved) Save(context.Context, int32, petmodel.State) error { return nil }

type fakePetStoreSaved struct{ state petmodel.State }

func (f fakePetStoreSaved) Get(context.Context, int32) (petmodel.State, bool, error) {
	return f.state, true, nil
}

func (fakePetStoreSaved) Save(context.Context, int32, petmodel.State) error { return nil }

type recordingPetStore struct {
	savedItemObjectID int32
	savedState        petmodel.State
	restoreState      petmodel.State
}

func (s *recordingPetStore) Get(context.Context, int32) (petmodel.State, bool, error) {
	return s.restoreState, s.restoreState != (petmodel.State{}), nil
}

func (s *recordingPetStore) Save(_ context.Context, itemObjectID int32, state petmodel.State) error {
	s.savedItemObjectID = itemObjectID
	s.savedState = state
	return nil
}

type fakeSummonIDs struct{ next int32 }

func (f *fakeSummonIDs) NextID() (int32, error) {
	f.next++
	return f.next, nil
}

const (
	summonTestCollarTemplateID = 91000
	summonTestWyvernTemplateID = 91001
)

func summonTestPetNPCTemplate() *npc.Template {
	return &npc.Template{
		ID: 12500, Name: "Wolf", Level: 10,
		STR: 40, CON: 43, DEX: 30, INT: 22, WIT: 20, MEN: 20,
		BaseAttackRange: 40,
		Pet: &npc.PetData{
			Food1: 2515, Food2: 2516,
			AutoFeedLimit: 55, HungryLimit: 30, UnsummonLimit: 10,
			Levels: map[int]npc.PetLevelStats{
				10: {MaxHP: 400, MaxMP: 80, PAtk: 100, PDef: 90, MAtk: 20, MDef: 40, MaxMeal: 3000, MealInNormal: 5, MealInBattle: 10},
			},
		},
		Skills: map[int]int{2046: 1},
	}
}

// newSummonTestLink builds a GameClientLink whose npcs/summonItems/petStore
// are wired for gameSummonSpawner.SpawnPet, sharing state with the caller
// so the caller can assert against it.
func newSummonTestLink(t *testing.T) (*GameClientLink, *world.State) {
	t.Helper()
	npcs := npc.NewTable([]*npc.Template{summonTestPetNPCTemplate()})
	summonItems, err := item.NewSummonItemTable([]item.SummonItem{
		{ItemID: summonTestCollarTemplateID, NPCID: 12500, SummonType: summonItemTypePet},
		{ItemID: summonTestWyvernTemplateID, NPCID: 12621, SummonType: summonItemTypeWyvern},
	})
	if err != nil {
		t.Fatalf("build summon item table: %v", err)
	}
	state := world.New()
	link := NewGameClientLink(GameClientLinkConfig{
		World:         state,
		NPCs:          npcs,
		SummonItems:   summonItems,
		PetStore:      fakePetStoreNoSaved{},
		IDs:           &fakeSummonIDs{},
		Skills:        summonTestSkillTable(t),
		ItemTemplates: testItemTemplates(),
	})
	return link, state
}

func TestUseSummonItemMountsWyvern(t *testing.T) {
	link, state := newSummonTestLink(t)
	frames := &frameCapture{}
	live := newTestLivePlayer(t, 1, frames)
	state.Spawn(live, 0, 0, 0, 0)
	live.Character.SetUserInfoUpdater(func() {
		live.SendFrame(serverpackets.FrameUserInfo(serverpackets.UserInfoSnapshot{Character: live.Character, Template: live.template}))
	})
	inst := &item.Instance{ObjectID: 501, TemplateID: summonTestWyvernTemplateID, OwnerID: live.ObjectID(), Count: 1, Location: item.LocationInventory}
	live.Inventory().Restore([]*item.Instance{inst})

	if !link.useSummonItem(live, live.Inventory(), inst) {
		t.Fatal("useSummonItem returned false, want handled wyvern")
	}
	if got := live.Character.MountType(); got != 2 {
		t.Fatalf("MountType() = %d, want 2", got)
	}
	if got := live.Character.MountObjectID(); got != inst.ObjectID {
		t.Fatalf("MountObjectID() = %d, want %d", got, inst.ObjectID)
	}
	assertOpcodeSequence(t, frames.frames, serverpackets.OpcodeRide, serverpackets.OpcodeUserInfo)

	frames.frames = nil
	link.destroyLiveItem(live, inst.ObjectID, 1)
	if got := live.Inventory().ItemByObjectID(inst.ObjectID); got == nil {
		t.Fatal("destroying mounted control item removed it")
	}
	assertOpcodeSequence(t, frames.frames, serverpackets.OpcodeActionFailed)
}

func TestUseSummonItemRejectsWyvernWhileSitting(t *testing.T) {
	link, _ := newSummonTestLink(t)
	frames := &frameCapture{}
	live := newTestLivePlayer(t, 1, frames)
	live.Character.SetStanding(false)
	inst := &item.Instance{ObjectID: 502, TemplateID: summonTestWyvernTemplateID, OwnerID: live.ObjectID(), Count: 1, Location: item.LocationInventory}
	live.Inventory().Restore([]*item.Instance{inst})

	if !link.useSummonItem(live, live.Inventory(), inst) {
		t.Fatal("useSummonItem returned false, want handled rejection")
	}
	if got := live.Character.MountType(); got != 0 {
		t.Fatalf("MountType() = %d, want 0", got)
	}
	assertOpcodeSequence(t, frames.frames, serverpackets.OpcodeSystemMessage)
}

func TestUseSummonItemRejectsWyvernInCombat(t *testing.T) {
	link, _ := newSummonTestLink(t)
	frames := &frameCapture{}
	live := newTestLivePlayer(t, 1, frames)
	live.Character.SetInCombat(true)
	inst := &item.Instance{ObjectID: 503, TemplateID: summonTestWyvernTemplateID, OwnerID: live.ObjectID(), Count: 1, Location: item.LocationInventory}
	live.Inventory().Restore([]*item.Instance{inst})

	if !link.useSummonItem(live, live.Inventory(), inst) {
		t.Fatal("useSummonItem returned false, want handled rejection")
	}
	if got := live.Character.MountType(); got != 0 {
		t.Fatalf("MountType() = %d, want 0", got)
	}
	assertOpcodeSequence(t, frames.frames, serverpackets.OpcodeSystemMessage)
}

func TestUseSummonItemRejectsSecondWyvern(t *testing.T) {
	link, state := newSummonTestLink(t)
	frames := &frameCapture{}
	live := newTestLivePlayer(t, 1, frames)
	state.Spawn(live, 0, 0, 0, 0)
	first := &item.Instance{ObjectID: 504, TemplateID: summonTestWyvernTemplateID, OwnerID: live.ObjectID(), Count: 1, Location: item.LocationInventory}
	second := &item.Instance{ObjectID: 505, TemplateID: summonTestWyvernTemplateID, OwnerID: live.ObjectID(), Count: 1, Location: item.LocationInventory}
	live.Inventory().Restore([]*item.Instance{first, second})
	if !live.Character.Mount(12621, first.ObjectID) {
		t.Fatal("Mount() = false, want true")
	}

	if !link.useSummonItem(live, live.Inventory(), second) {
		t.Fatal("useSummonItem returned false, want handled rejection")
	}
	if got := live.Character.MountObjectID(); got != first.ObjectID {
		t.Fatalf("MountObjectID() = %d, want %d", got, first.ObjectID)
	}
	assertOpcodeSequence(t, frames.frames, serverpackets.OpcodeSystemMessage)
}

// summonTestSkillTable registers SUMMON_CREATURE (2046,1) with a zero hit
// time so a test can drive its Launch/Hit phases synchronously without a
// fake clock, mirroring itemAICastSkillTable's TELEPORT fixture.
func summonTestSkillTable(t *testing.T) *skillstate.Persistence {
	t.Helper()
	store := newMemorySkillSaveStore()
	return skillstate.NewPersistence(store, modelskill.NewTable([]modelskill.Definition{
		{
			ID: 2046, Level: 1, Activation: modelskill.ActivationActive, Target: modelskill.TargetSelf,
			SkillType: "SUMMON_CREATURE", StaticHitTime: true, HitTime: 0, StaticReuse: true, ReuseDelay: 0,
		},
	}), store)
}

func TestGameSummonSpawnerSpawnPetRegistersLiveActor(t *testing.T) {
	link, state := newSummonTestLink(t)
	live := newTestLivePlayer(t, 1, &frameCapture{})
	inst := &item.Instance{ObjectID: 500, TemplateID: summonTestCollarTemplateID, OwnerID: live.ObjectID()}

	spawner := &gameSummonSpawner{link: link, live: live}
	if !spawner.SpawnPet(live.Character, inst) {
		t.Fatalf("SpawnPet returned false")
	}

	obj, ok := state.Summon(live.ObjectID())
	if !ok {
		t.Fatalf("pet not registered in world.State as the owner's active summon")
	}
	pet, ok := obj.(*summon.Actor)
	if !ok {
		t.Fatalf("registered summon is %T, want *summon.Actor", obj)
	}
	if pet.NPCID() != 12500 {
		t.Fatalf("NPCID() = %d, want 12500", pet.NPCID())
	}
	if pet.Name() != "Wolf" {
		t.Fatalf("Name() = %q, want template name %q (no saved row)", pet.Name(), "Wolf")
	}
	if pet.PetInventory() == nil {
		t.Fatal("PetInventory() = nil, want a dedicated carried-item inventory")
	}

	// TryUseSkill previously short-circuited to false whenever no AI was
	// attached (Actor.brain == nil); SpawnPet must leave it non-nil so an
	// owner-commanded skill dispatch actually reaches the AI layer, even
	// though the cast controller itself is wired by a follow-up.
	if !pet.TryUseSkill(2046, live.Character) {
		t.Fatalf("TryUseSkill(2046) = false after spawn, want true (AI must be attached)")
	}
}

func TestGameSummonSpawnerSpawnServitorRegistersLiveActor(t *testing.T) {
	link, state := newSummonTestLink(t)
	link.npcs = npc.NewTable([]*npc.Template{{
		ID: 14848, Name: "Cat", Level: 40,
		STR: 40, CON: 43, DEX: 30, INT: 22, WIT: 20, MEN: 20,
		BaseAttackRange: 40, CollisionRadius: 12.5,
		Skills: map[int]int{1126: 1},
	}})
	live := newTestLivePlayer(t, 1, &frameCapture{})

	spawner := &gameSummonSpawner{link: link, live: live}
	if !spawner.SpawnServitor(live.Character, modelskill.Definition{NpcID: 14848}) {
		t.Fatal("SpawnServitor returned false")
	}

	obj, ok := state.Summon(live.ObjectID())
	if !ok {
		t.Fatal("servitor not registered in world.State as the owner's active summon")
	}
	servitor, ok := obj.(*summon.Actor)
	if !ok {
		t.Fatalf("registered summon is %T, want *summon.Actor", obj)
	}
	if servitor.NPCID() != 14848 {
		t.Fatalf("NPCID() = %d, want 14848", servitor.NPCID())
	}
	if got := servitor.CollisionRadius(); got != 12.5 {
		t.Fatalf("CollisionRadius() = %v, want 12.5", got)
	}
}

func TestGameSummonSpawnerSpawnPetRestoresSavedName(t *testing.T) {
	link, state := newSummonTestLink(t)
	link.petStore = fakePetStoreSaved{state: petmodel.State{
		Name: "Fang", Level: 10, CurHP: 400, CurMP: 80, Fed: 3000,
	}}
	live := newTestLivePlayer(t, 1, &frameCapture{})
	inst := &item.Instance{ObjectID: 500, TemplateID: summonTestCollarTemplateID, OwnerID: live.ObjectID()}

	spawner := &gameSummonSpawner{link: link, live: live}
	if !spawner.SpawnPet(live.Character, inst) {
		t.Fatalf("SpawnPet returned false")
	}

	obj, _ := state.Summon(live.ObjectID())
	pet := obj.(*summon.Actor)
	if pet.Name() != "Fang" {
		t.Fatalf("Name() = %q, want restored saved name %q", pet.Name(), "Fang")
	}
}

func TestGameClientLinkReturnPetSavesCollarState(t *testing.T) {
	link, _ := newSummonTestLink(t)
	want := petmodel.State{Name: "Fang", Level: 10, Exp: 1234, SP: 56, CurHP: 200, CurMP: 40, Fed: 2500}
	store := &recordingPetStore{restoreState: want}
	link.petStore = store
	live := newTestLivePlayer(t, 1, &frameCapture{})
	inst := &item.Instance{ObjectID: 500, TemplateID: summonTestCollarTemplateID, OwnerID: live.ObjectID()}
	if !(&gameSummonSpawner{link: link, live: live}).SpawnPet(live.Character, inst) {
		t.Fatal("SpawnPet returned false")
	}

	link.handleSummonActionUse(context.Background(), live, clientpackets.RequestActionUse{ActionID: 19})

	if store.savedItemObjectID != inst.ObjectID {
		t.Fatalf("saved collar = %d, want %d", store.savedItemObjectID, inst.ObjectID)
	}
	if got := store.savedState; got != want {
		t.Fatalf("saved state = %+v, want %+v", got, want)
	}
}

func TestGameClientLinkDetachLivePlayerSavesPetCollarState(t *testing.T) {
	link, state := newSummonTestLink(t)
	store := &recordingPetStore{}
	link.petStore = store
	live := newTestLivePlayer(t, 1, &frameCapture{})
	inst := &item.Instance{ObjectID: 500, TemplateID: summonTestCollarTemplateID, OwnerID: live.ObjectID()}
	if !(&gameSummonSpawner{link: link, live: live}).SpawnPet(live.Character, inst) {
		t.Fatal("SpawnPet returned false")
	}
	obj, _ := state.Summon(live.ObjectID())
	if got := obj.(*summon.Actor).PetInventory().AddNew(20, 1, 900); got == nil {
		t.Fatal("add carried pet potion")
	}

	link.detachLivePlayer(context.Background(), live)

	if store.savedItemObjectID != inst.ObjectID {
		t.Fatalf("saved collar = %d, want %d", store.savedItemObjectID, inst.ObjectID)
	}
	if got := live.Inventory().ItemCount(20, -1, true); got != 1 {
		t.Fatalf("owner pet potion count = %d, want 1", got)
	}
}

func TestGameClientLinkReturnPetTransfersCarriedItemsToOwner(t *testing.T) {
	link, state := newSummonTestLink(t)
	live := newTestLivePlayer(t, 1, &frameCapture{})
	inst := &item.Instance{ObjectID: 500, TemplateID: summonTestCollarTemplateID, OwnerID: live.ObjectID()}
	if !(&gameSummonSpawner{link: link, live: live}).SpawnPet(live.Character, inst) {
		t.Fatal("SpawnPet returned false")
	}
	obj, ok := state.Summon(live.ObjectID())
	if !ok {
		t.Fatal("spawned pet missing from world")
	}
	pet := obj.(*summon.Actor)
	if got := pet.PetInventory().AddNew(20, 1, 900); got == nil {
		t.Fatal("add carried pet potion")
	}

	link.handleSummonActionUse(context.Background(), live, clientpackets.RequestActionUse{ActionID: 19})

	if got := live.Inventory().ItemCount(20, -1, true); got != 1 {
		t.Fatalf("owner pet potion count = %d, want 1", got)
	}
}

func TestGameClientLinkReturnPetDropsCarriedItemsWhenOwnerInventoryIsFull(t *testing.T) {
	link, state := newSummonTestLink(t)
	drops := &recordingGroundDropper{}
	link.groundItems = drops
	live := newTestLivePlayer(t, 1, &frameCapture{})
	live.Inventory().SlotLimit = 1
	live.Inventory().AddNew(30, 1, 800)
	inst := &item.Instance{ObjectID: 500, TemplateID: summonTestCollarTemplateID, OwnerID: live.ObjectID()}
	if !(&gameSummonSpawner{link: link, live: live}).SpawnPet(live.Character, inst) {
		t.Fatal("SpawnPet returned false")
	}
	obj, _ := state.Summon(live.ObjectID())
	pet := obj.(*summon.Actor)
	if got := pet.PetInventory().AddNew(20, 1, 900); got == nil {
		t.Fatal("add carried pet potion")
	}

	link.handleSummonActionUse(context.Background(), live, clientpackets.RequestActionUse{ActionID: 19})

	if len(drops.drops) != 1 {
		t.Fatalf("ground drops = %d, want 1", len(drops.drops))
	}
	if got := drops.drops[0].ground.Instance.TemplateID; got != 20 {
		t.Fatalf("dropped template = %d, want 20", got)
	}
}

// TestUseSummonItemSpawnsPetEndToEnd drives the pet-collar item-use trigger
// itself (rather than gameSummonSpawner.SpawnPet directly), proving the
// whole chain from a client's item-use action through the timed
// Launch/Hit cast to a live, AI-attached pet in the world.
func TestUseSummonItemSpawnsPetEndToEnd(t *testing.T) {
	link, state := newSummonTestLink(t)
	live := newTestLivePlayer(t, 1, &frameCapture{})
	inst := &item.Instance{ObjectID: 500, TemplateID: summonTestCollarTemplateID, OwnerID: live.ObjectID()}
	live.Inventory().Restore([]*item.Instance{inst})

	if !link.useSummonItem(live, live.Inventory(), inst) {
		t.Fatalf("useSummonItem returned false, want true (handled)")
	}

	// The Hit phase fires on a real time.AfterFunc(0, ...) timer (the cast
	// controller has no injectable clock at this layer, matching every
	// other production cast path); poll briefly instead of asserting
	// synchronously.
	var obj worldobject.Object
	var ok bool
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		if obj, ok = state.Summon(live.ObjectID()); ok {
			break
		}
		time.Sleep(time.Millisecond)
	}
	if !ok {
		t.Fatalf("pet not registered in world.State after item use")
	}
	pet, ok := obj.(*summon.Actor)
	if !ok {
		t.Fatalf("registered summon is %T, want *summon.Actor", obj)
	}
	if !pet.TryUseSkill(2046, live.Character) {
		t.Fatalf("TryUseSkill(2046) = false after item-use spawn, want true")
	}
}

func TestUseSummonItemRejectsSittingOwner(t *testing.T) {
	link, _ := newSummonTestLink(t)
	frames := &frameCapture{}
	live := newTestLivePlayer(t, 1, frames)
	live.Character.SetStanding(false)
	inst := &item.Instance{ObjectID: 500, TemplateID: summonTestCollarTemplateID, OwnerID: live.ObjectID()}

	if !link.useSummonItem(live, live.Inventory(), inst) {
		t.Fatal("useSummonItem returned false, want handled rejection")
	}
	if len(frames.frames) != 1 {
		t.Fatalf("frames = %d, want exactly one rejection", len(frames.frames))
	}
	assertSystemMessageIDFrame(t, frames.frames[0], 31)
}

// TestGameSummonSpawnerSpawnPetBroadcastsSpawnRelation covers Summon.onSpawn's
// two RelationChanged origins (Summon.java:336-349, 351-355): the owner gets
// a self-view for the newly spawned pet, and every nearby observer gets the
// pet's own RelationChanged — but not a resend of the owner's own relation,
// since spawning a pet doesn't change the owner's karma/pvp-flag state.
func TestGameSummonSpawnerSpawnPetBroadcastsSpawnRelation(t *testing.T) {
	link, state := newSummonTestLink(t)
	link.ids = &fakeSummonIDs{next: 100}
	selfFrames := &frameCapture{}
	observerFrames := &frameCapture{}
	live := newTestLivePlayer(t, 1, selfFrames)
	observer := newTestLivePlayer(t, 2, observerFrames)
	live.Character.KarmaPoints = 500
	state.Spawn(live, 0, 0, 0, 0)
	state.Spawn(observer, 100, 0, 0, 0)
	// Spawn's mutual Discover already exchanged CharInfo frames between the
	// two; clear those before exercising SpawnPet's relation broadcast.
	selfFrames.frames = nil
	observerFrames.frames = nil

	inst := &item.Instance{ObjectID: 500, TemplateID: summonTestCollarTemplateID, OwnerID: live.ObjectID()}
	spawner := &gameSummonSpawner{link: link, live: live}
	if !spawner.SpawnPet(live.Character, inst) {
		t.Fatalf("SpawnPet returned false")
	}
	obj, ok := state.Summon(live.ObjectID())
	if !ok {
		t.Fatalf("pet not registered in world.State")
	}
	pet := obj.(*summon.Actor)

	if len(selfFrames.frames) != 1 {
		t.Fatalf("self frames = %d, want 1 (summon self-view)", len(selfFrames.frames))
	}
	wantSelf := relationChangedPayload(pet.ObjectID(), serverpackets.RelationHasKarma, 0, 500, 0)
	if !bytes.Equal(selfFrames.frames[0], wantSelf) {
		t.Fatalf("self frame = %x, want %x", selfFrames.frames[0], wantSelf)
	}

	if len(observerFrames.frames) != 1 {
		t.Fatalf("observer frames = %d, want 1 (summon only, no owner resend)", len(observerFrames.frames))
	}
	wantObserver := relationChangedPayload(pet.ObjectID(), serverpackets.RelationHasKarma, 1, 500, 0)
	if !bytes.Equal(observerFrames.frames[0], wantObserver) {
		t.Fatalf("observer frame = %x, want %x", observerFrames.frames[0], wantObserver)
	}
}

func TestGameSummonSpawnerSpawnPetRejectsSecondSummon(t *testing.T) {
	link, _ := newSummonTestLink(t)
	live := newTestLivePlayer(t, 1, &frameCapture{})
	inst := &item.Instance{ObjectID: 500, TemplateID: summonTestCollarTemplateID, OwnerID: live.ObjectID()}

	spawner := &gameSummonSpawner{link: link, live: live}
	if !spawner.SpawnPet(live.Character, inst) {
		t.Fatalf("first SpawnPet returned false")
	}
	if spawner.SpawnPet(live.Character, inst) {
		t.Fatalf("second SpawnPet returned true, want false (SUMMON_ONLY_ONE)")
	}
}
