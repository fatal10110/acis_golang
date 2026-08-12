package network

import (
	"context"
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/npc"
	petmodel "github.com/fatal10110/acis_golang/internal/gameserver/model/actor/pet"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/summon"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/item"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	"github.com/fatal10110/acis_golang/internal/gameserver/world"
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
	live := newTestLivePlayer(t, 1, &frameCapture{})
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
			live := newTestLivePlayer(t, 1, &frameCapture{})
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
