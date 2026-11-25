package pets

import (
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/pet"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
)

// joinSecondOwner brings a second player holding a wolf collar into h's
// world, as a petWorld of its own.
func (h *petWorld) joinSecondOwner(t *testing.T, name string) *petWorld {
	t.Helper()
	id := h.srv.SeedCharacterFor(t, "player2", name, 1, 0).ID
	collarID := h.srv.GiveItem(t, id, wolfCollarID, 1)
	c := h.srv.DialClient(t, "player2", 1)
	startInWorld(t, c)
	drainUntilQuiet(t, h.client)
	return &petWorld{srv: h.srv, client: c, ownerID: id, collarID: collarID, seeded: map[int32][]int32{}}
}

// TestConcurrentPetRenamesToOneNameApplyOnce holds the first owner's
// pets-table read of a name while a second owner renames its own pet to the
// same name: the second rename is refused as a name in use, though the
// table does not hold it yet, and only the first is applied and saved
// (#5060).
func TestConcurrentPetRenamesToOneNameApplyOnce(t *testing.T) {
	t.Parallel()
	entered, release := make(chan struct{}), make(chan struct{})
	var held atomic.Bool
	var releaseOnce sync.Once
	letGo := func() { releaseOnce.Do(func() { close(release) }) }
	h := bootOwnerWithCollarOpts(t, []gameservertest.Option{gameservertest.WithPetNameLookupHold(func() {
		if held.CompareAndSwap(false, true) {
			close(entered)
			<-release
		}
	})})
	// Registered after the boot, so it runs before the server shuts down.
	t.Cleanup(letGo)
	first, _ := h.spawnWolf(t)
	h2 := h.joinSecondOwner(t, "Rival")
	second, _ := h2.spawnWolf(t)
	drainUntilQuiet(t, h.client)
	drainUntilQuiet(t, h2.client)

	h.client.Send(encodeRequestChangePetName("Fenrir"))
	select {
	case <-entered:
	case <-time.After(30 * time.Second):
		t.Fatal("first rename never read the pets table")
	}
	// A read waits for every connection to catch up, the held one too, so
	// the second answer is only awaited here and read once the first read
	// goes on.
	h2.client.Send(encodeRequestChangePetName("fenrir"))
	h.srv.AwaitFrameQueued(t, h2.client)
	letGo()
	frames := drainFrames(t, h2.client)
	if len(frames) != 1 {
		t.Fatalf("second rename frames = opcodes %x, want one refusal", frameOpcodes(frames))
	}
	assertStaticSystemMessage(t, frames[0], serverpackets.SystemMessageNamingAlreadyInUseByAnotherPet)
	frames = drainFrames(t, h.client)
	if len(frames) != 1 || frames[0][0] != serverpackets.OpcodePetInfo {
		t.Fatalf("first rename frames = opcodes %x, want only a refreshed PetInfo", frameOpcodes(frames))
	}
	if _, name := readPetInfoName(t, frames[0]); name != "Fenrir" {
		t.Fatalf("first rename PetInfo name = %q, want Fenrir", name)
	}
	if got := first.Name(); got != "Fenrir" {
		t.Fatalf("first pet Name() = %q, want Fenrir", got)
	}
	if second.IsNamed() {
		t.Fatalf("second pet named %q, want it unnamed", second.Name())
	}
	if state := h.savedPetState(t); state.Name != "Fenrir" {
		t.Fatalf("first pets row name = %q, want Fenrir", state.Name)
	}
	if state, ok, err := h.srv.Pets.Get(petCtx(), h2.collarID); err != nil || (ok && state.Name != "") {
		t.Fatalf("second pets row = %+v ok=%v err=%v, want no name", state, ok, err)
	}
}

// TestRefusedPetRenameFreesTheName: a rename refused because the pets
// table holds the name leaves no claim on it behind, so once the table no
// longer holds it the same name is applied (#5060).
func TestRefusedPetRenameFreesTheName(t *testing.T) {
	t.Parallel()
	h := bootOwnerWithCollar(t)
	wolf, _ := h.spawnWolf(t)
	if err := h.srv.Pets.Save(petCtx(), 999999, pet.State{Name: "Fenrir", Level: wolfLevel}); err != nil {
		t.Fatalf("seed taken name: %v", err)
	}
	frames := sysMessages(renameTo(t, h, "Fenrir"))
	if len(frames) != 1 {
		t.Fatalf("taken-name rejections = %d, want exactly one", len(frames))
	}
	assertStaticSystemMessage(t, frames[0], serverpackets.SystemMessageNamingAlreadyInUseByAnotherPet)

	if err := h.srv.Pets.Save(petCtx(), 999999, pet.State{Name: "Other", Level: wolfLevel}); err != nil {
		t.Fatalf("rename seeded pet: %v", err)
	}
	frames = renameTo(t, h, "Fenrir")
	if len(frames) != 1 || frames[0][0] != serverpackets.OpcodePetInfo {
		t.Fatalf("rename frames = opcodes %x, want only a refreshed PetInfo", frameOpcodes(frames))
	}
	if got := wolf.Name(); got != "Fenrir" {
		t.Fatalf("Name() = %q, want Fenrir", got)
	}
}
