package network

import (
	"context"
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/npc"
	petmodel "github.com/fatal10110/acis_golang/internal/gameserver/model/actor/pet"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/summon"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/item"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/clientpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/world"
	"github.com/fatal10110/acis_golang/internal/testsupport"
)

type petNameStoreStub struct {
	taken bool
	saved petmodel.State
}

func (s *petNameStoreStub) Get(context.Context, int32) (petmodel.State, bool, error) {
	return petmodel.State{}, false, nil
}
func (s *petNameStoreStub) NameTaken(context.Context, string) (bool, error) { return s.taken, nil }
func (s *petNameStoreStub) Save(_ context.Context, _ int32, state petmodel.State) error {
	s.saved = state
	return nil
}

func TestRenamePetPersistsCollarAndRefreshesOwner(t *testing.T) {
	state := world.New()
	live := newTestLivePlayer(t, 1, &testsupport.FrameCapture{})
	state.Spawn(live, 0, 0, 0, 0)
	collar := &item.Instance{ObjectID: 77, Location: item.LocationInventory}
	live.Inventory().Restore([]*item.Instance{collar})
	actor := summon.NewPet(summon.PetConfig{ObjectID: 2, Owner: live, ControlItemID: 77, Name: "Wolf", Level: 1})
	summon.SpawnBesideOwner(state, actor, live, location.Location{})
	updates := 0
	actor.SetStatusUpdater(func() { updates++ })
	store := &petNameStoreStub{}
	link := &GameClientLink{world: state, npcs: npc.NewTable(nil), petStore: store}
	if got := link.renamePet(context.Background(), live, "Fenrir"); got != petRenameApplied {
		t.Fatalf("renamePet() = %v", got)
	}
	if actor.Name() != "Fenrir" || store.saved.Name != "Fenrir" {
		t.Fatal("rename was not persisted")
	}
	reloaded := summon.NewPet(summon.PetConfig{Name: store.saved.Name})
	if reloaded.Name() != "Fenrir" {
		t.Fatalf("reloaded Name() = %q, want Fenrir", reloaded.Name())
	}
	if collar.Snapshot().CustomType2 != 1 || updates != 1 {
		t.Fatal("collar or owner refresh missing")
	}
}

func TestRenamePetRejectsTakenAndNPCNames(t *testing.T) {
	for _, tt := range []struct {
		name  string
		taken bool
		npcs  *npc.Table
		want  petRenameResult
	}{
		{"Fenrir", true, npc.NewTable(nil), petRenameNameTaken},
		{"Wolf", false, npc.NewTable([]*npc.Template{{ID: 1, Name: "Wolf"}}), petRenameNPCName},
	} {
		t.Run(tt.name, func(t *testing.T) {
			state := world.New()
			live := newTestLivePlayer(t, 1, &testsupport.FrameCapture{})
			state.Spawn(live, 0, 0, 0, 0)
			actor := summon.NewPet(summon.PetConfig{ObjectID: 2, Owner: live, ControlItemID: 77, Name: "Puppy"})
			summon.SpawnBesideOwner(state, actor, live, location.Location{})
			link := &GameClientLink{world: state, npcs: tt.npcs, petStore: &petNameStoreStub{taken: tt.taken}}
			if got := link.renamePet(context.Background(), live, tt.name); got != tt.want {
				t.Fatalf("renamePet() = %v, want %v", got, tt.want)
			}
			if actor.Name() != "Puppy" {
				t.Fatalf("Name() = %q after rejection", actor.Name())
			}
		})
	}
}

func TestHandleRequestChangePetNameGates(t *testing.T) {
	newActor := func(state *world.State, live *livePlayer, named bool) *summon.Actor {
		actor := summon.NewPet(summon.PetConfig{ObjectID: 2, Owner: live, ControlItemID: 77, Name: "Wolf", Named: named})
		summon.SpawnBesideOwner(state, actor, live, location.Location{})
		return actor
	}

	t.Run("no active pet is silent", func(t *testing.T) {
		state := world.New()
		capture := &testsupport.FrameCapture{}
		live := newTestLivePlayer(t, 1, capture)
		state.Spawn(live, 0, 0, 0, 0)
		link := &GameClientLink{world: state, npcs: npc.NewTable(nil), petStore: &petNameStoreStub{}}
		link.handleRequestChangePetName(context.Background(), live, clientpackets.RequestChangePetName{Name: "Rex"})
		if len(capture.Frames()) != 0 {
			t.Fatalf("frames sent = %d, want 0", len(capture.Frames()))
		}
	})

	t.Run("invalid length rejects before already-named gate", func(t *testing.T) {
		state := world.New()
		capture := &testsupport.FrameCapture{}
		live := newTestLivePlayer(t, 1, capture)
		state.Spawn(live, 0, 0, 0, 0)
		newActor(state, live, true)
		link := &GameClientLink{world: state, npcs: npc.NewTable(nil), petStore: &petNameStoreStub{}}
		link.handleRequestChangePetName(context.Background(), live, clientpackets.RequestChangePetName{Name: ""})
		frames := capture.Frames()
		if len(frames) != 1 || !isSystemMessage(frames[0], serverpackets.SystemMessageNamingCharnameUpTo16Chars) {
			t.Fatalf("frames = %v, want NAMING_CHARNAME_UP_TO_16CHARS", frames)
		}
	})

	t.Run("already named rejects before pattern check", func(t *testing.T) {
		state := world.New()
		capture := &testsupport.FrameCapture{}
		live := newTestLivePlayer(t, 1, capture)
		state.Spawn(live, 0, 0, 0, 0)
		actor := newActor(state, live, true)
		link := &GameClientLink{world: state, npcs: npc.NewTable(nil), petStore: &petNameStoreStub{}}
		link.handleRequestChangePetName(context.Background(), live, clientpackets.RequestChangePetName{Name: "!!!"})
		frames := capture.Frames()
		if len(frames) != 1 || !isSystemMessage(frames[0], serverpackets.SystemMessageNamingYouCannotSetNameOfThePet) {
			t.Fatalf("frames = %v, want NAMING_YOU_CANNOT_SET_NAME_OF_THE_PET", frames)
		}
		if actor.Name() != "Wolf" {
			t.Fatalf("Name() = %q, want unchanged", actor.Name())
		}
	})

	t.Run("invalid pattern rejected", func(t *testing.T) {
		state := world.New()
		capture := &testsupport.FrameCapture{}
		live := newTestLivePlayer(t, 1, capture)
		state.Spawn(live, 0, 0, 0, 0)
		newActor(state, live, false)
		link := &GameClientLink{world: state, npcs: npc.NewTable(nil), petStore: &petNameStoreStub{}}
		link.handleRequestChangePetName(context.Background(), live, clientpackets.RequestChangePetName{Name: "Re x!"})
		frames := capture.Frames()
		if len(frames) != 1 || !isSystemMessage(frames[0], serverpackets.SystemMessageNamingPetnameContainsInvalidChars) {
			t.Fatalf("frames = %v, want NAMING_PETNAME_CONTAINS_INVALID_CHARS", frames)
		}
	})

	t.Run("npc name collision is silent", func(t *testing.T) {
		state := world.New()
		capture := &testsupport.FrameCapture{}
		live := newTestLivePlayer(t, 1, capture)
		state.Spawn(live, 0, 0, 0, 0)
		actor := newActor(state, live, false)
		link := &GameClientLink{world: state, npcs: npc.NewTable([]*npc.Template{{ID: 1, Name: "Rex"}}), petStore: &petNameStoreStub{}}
		link.handleRequestChangePetName(context.Background(), live, clientpackets.RequestChangePetName{Name: "Rex"})
		if len(capture.Frames()) != 0 {
			t.Fatalf("frames sent = %d, want 0 (silent npc-name reject)", len(capture.Frames()))
		}
		if actor.Name() != "Wolf" {
			t.Fatalf("Name() = %q after silent rejection", actor.Name())
		}
	})

	t.Run("taken name rejected with message", func(t *testing.T) {
		state := world.New()
		capture := &testsupport.FrameCapture{}
		live := newTestLivePlayer(t, 1, capture)
		state.Spawn(live, 0, 0, 0, 0)
		newActor(state, live, false)
		link := &GameClientLink{world: state, npcs: npc.NewTable(nil), petStore: &petNameStoreStub{taken: true}}
		link.handleRequestChangePetName(context.Background(), live, clientpackets.RequestChangePetName{Name: "Rex"})
		frames := capture.Frames()
		if len(frames) != 1 || !isSystemMessage(frames[0], serverpackets.SystemMessageNamingAlreadyInUseByAnotherPet) {
			t.Fatalf("frames = %v, want NAMING_ALREADY_IN_USE_BY_ANOTHER_PET", frames)
		}
	})

	t.Run("applied renames and refreshes owner PetInfo", func(t *testing.T) {
		state := world.New()
		capture := &testsupport.FrameCapture{}
		live := newTestLivePlayer(t, 1, capture)
		live.npcs = npc.NewTable([]*npc.Template{{ID: 0}})
		state.Spawn(live, 0, 0, 0, 0)
		actor := newActor(state, live, false)
		link := &GameClientLink{world: state, npcs: live.npcs, petStore: &petNameStoreStub{}}
		link.handleRequestChangePetName(context.Background(), live, clientpackets.RequestChangePetName{Name: "Rex"})
		if actor.Name() != "Rex" || !actor.IsNamed() {
			t.Fatalf("Name() = %q, IsNamed() = %v, want Rex/true", actor.Name(), actor.IsNamed())
		}
		// Spawning already sent one PetInfo frame (owner discovering its own
		// pet); the rename must send a second, refreshed one.
		frames := capture.Frames()
		if len(frames) != 2 || frames[1][0] != serverpackets.OpcodePetInfo {
			t.Fatalf("frames = %v, want spawn PetInfo + refreshed PetInfo", frames)
		}
	})
}

func isSystemMessage(frame []byte, id int) bool {
	if len(frame) < 5 || frame[0] != serverpackets.OpcodeSystemMessage {
		return false
	}
	got := int(frame[1]) | int(frame[2])<<8 | int(frame[3])<<16 | int(frame[4])<<24
	return got == id
}
