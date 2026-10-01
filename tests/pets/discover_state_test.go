package pets

import (
	"testing"

	"github.com/fatal10110/acis_golang/internal/commons/wire"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/summon"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/sim"
	"github.com/fatal10110/acis_golang/internal/testsupport"
)

// discoverWalk is how far past the owner a summon walking into view heads:
// far enough that the walk outlasts the test on either clock.
const discoverWalk = 4000

// TestOwnerAndWatcherSeeWalkingPetMove pins what players coming to know a
// walking pet are shown. The owner gets PetInfo, then its PetItemList, then
// the pet's MoveToLocation from where it stands to the end of its walk; a
// player beside the owner gets NpcInfo then the same MoveToLocation.
func TestOwnerAndWatcherSeeWalkingPetMove(t *testing.T) {
	t.Parallel()
	h, pet, queue, home := bootStillPetOutOfView(t)
	watcher := h.enterWatcher(t)
	target := location.Location{X: home.X + discoverWalk, Y: home.Y, Z: home.Z}

	walkSummonIntoView(t, h, pet, queue, home, target, nil)
	ownerFrames := drainFrames(t, h.client)
	watcherFrames := drainFrames(t, watcher)

	info := framePetInfo(ownerFrames, pet.ObjectID())
	list := frameIndexFrom(ownerFrames, serverpackets.OpcodePetItemList, info+1)
	if info < 0 || list < 0 || list+1 >= len(ownerFrames) {
		t.Fatalf("owner frames = opcodes %x, want PetInfo, PetItemList then the pet's MoveToLocation", frameOpcodes(ownerFrames))
	}
	h.requireDescribedWalk(t, ownerFrames[list+1], pet.ObjectID(), home, target, "owner")

	npcInfo := frameIndex(watcherFrames, serverpackets.OpcodeNPCInfo, pet.ObjectID())
	if npcInfo < 0 || npcInfo+1 >= len(watcherFrames) {
		t.Fatalf("watcher frames = opcodes %x, want the pet's NpcInfo then its MoveToLocation", frameOpcodes(watcherFrames))
	}
	h.requireDescribedWalk(t, watcherFrames[npcInfo+1], pet.ObjectID(), home, target, "watcher")
}

// TestOwnerSeesWalkingServitorMove pins the servitor side: having no item
// list, the owner's PetInfo for a walking servitor is followed straight by
// its MoveToLocation.
func TestOwnerSeesWalkingServitorMove(t *testing.T) {
	t.Parallel()
	h, cat, _ := bootFearServitor(t, catTemplate(), summonCatSkillID)
	queue, home := h.parkOutOfView(t, cat)
	target := location.Location{X: home.X + discoverWalk, Y: home.Y, Z: home.Z}

	walkSummonIntoView(t, h, cat, queue, home, target, nil)
	frames := drainFrames(t, h.client)
	info := framePetInfo(frames, cat.ObjectID())
	if info < 0 || info+1 >= len(frames) {
		t.Fatalf("owner frames = opcodes %x, want the servitor's PetInfo then its MoveToLocation", frameOpcodes(frames))
	}
	h.requireDescribedWalk(t, frames[info+1], cat.ObjectID(), home, target, "owner")
}

// TestOwnerPetMoveDroppedWhenPetUnsummonedFirst discovers the walking pet
// and unsummons it in one owner-queue task: the posted item list runs after
// the PetDelete and must not describe the walk of a pet that is gone.
func TestOwnerPetMoveDroppedWhenPetUnsummonedFirst(t *testing.T) {
	t.Parallel()
	h, pet, queue, home := bootStillPetOutOfView(t)
	target := location.Location{X: home.X + discoverWalk, Y: home.Y, Z: home.Z}

	walkSummonIntoView(t, h, pet, queue, home, target, func() { pet.Unsummon() })
	frames := drainFrames(t, h.client)
	requireNoPetItemListAfter(t, frames, serverpackets.OpcodePetDelete)
	requireNoMoveFor(t, frames, pet.ObjectID())
}

// TestOwnerPetMoveDroppedWhenPetLeftViewFirst discovers the walking pet and
// moves it back out of the owner's view in one owner-queue task. The pet is
// still walking when the posted item list runs, so only the sighting guard
// keeps its MoveToLocation from following the DeleteObject.
func TestOwnerPetMoveDroppedWhenPetLeftViewFirst(t *testing.T) {
	t.Parallel()
	h, pet, queue, home := bootStillPetOutOfView(t)
	away := location.Location{X: home.X + 20000, Y: home.Y, Z: home.Z}
	target := location.Location{X: away.X + discoverWalk, Y: home.Y, Z: home.Z}

	walkSummonIntoView(t, h, pet, queue, home, target, func() {
		pet.Move().SetPosition(away)
		pet.SyncPosition(away)
	})
	frames := drainFrames(t, h.client)
	requireNoPetItemListAfter(t, frames, serverpackets.OpcodeDeleteObject)
	requireNoMoveFor(t, frames, pet.ObjectID())
	if _, moving := pet.MovingTo(); !moving {
		t.Fatal("test setup: the pet stopped walking, so the guard went untested")
	}
}

// bootStillPetOutOfView spawns the owner's wolf, toggles its follow off and
// parks it out of the owner's view. It returns the owner's location, where
// moving the pet brings it back into view.
func bootStillPetOutOfView(t *testing.T) (*petWorld, *summon.Actor, *sim.Queue, location.Location) {
	t.Helper()
	h := bootOwnerWithCollar(t)
	pet, _ := h.spawnWolf(t)
	drainUntilQuiet(t, h.client)
	queue, home := h.parkOutOfView(t, pet)
	return h, pet, queue, home
}

// parkOutOfView toggles a's follow off, so the walk a test starts is the
// only one it takes, then moves it 20000 units away on the owner's queue so
// the owner forgets it. It returns the owner's queue and location.
func (h *petWorld) parkOutOfView(t *testing.T, a *summon.Actor) (*sim.Queue, location.Location) {
	t.Helper()
	h.client.Send(encodeRequestActionUse(petFollowToggleAction, false))
	h.srv.AdvanceUntil(t, "summon follow toggled off", func() bool { return !a.FollowActive() })
	drainUntilQuiet(t, h.client)

	queue := h.srv.PlayerQueue(t, h.ownerID)
	x, y, z := h.srv.PlayerPosition(t, h.ownerID)
	queue.Post(func() { a.SyncPosition(location.Location{X: x + 20000, Y: y, Z: z}) })
	readUntilOpcode(t, h.client, serverpackets.OpcodeDeleteObject, "DeleteObject as the summon leaves view")
	drainUntilQuiet(t, h.client)
	return queue, location.Location{X: x, Y: y, Z: z}
}

// walkSummonIntoView, in one task on the owner's queue, starts a's walk
// from home toward target, moves it to home (into view of everyone there),
// then runs after when it is non-nil.
func walkSummonIntoView(t *testing.T, h *petWorld, a *summon.Actor, queue *sim.Queue, home, target location.Location, after func()) {
	t.Helper()
	var walkErr error
	queue.Post(func() {
		a.Move().SetPosition(home)
		if _, walkErr = a.Move().MoveToLocation(target); walkErr != nil {
			return
		}
		a.SyncPosition(home)
		if after != nil {
			after()
		}
	})
	h.srv.Settle(t)
	if walkErr != nil {
		t.Fatalf("start summon walk: %v", walkErr)
	}
}

// enterWatcher brings a second character into the world beside the owner.
func (h *petWorld) enterWatcher(t *testing.T) *testsupport.ScriptedClient {
	t.Helper()
	h.srv.SeedCharacterFor(t, "watcher", "Watcher", 1, 0)
	watcher := h.srv.DialClient(t, "watcher", 1)
	startInWorld(t, watcher)
	drainUntilQuiet(t, watcher)
	drainUntilQuiet(t, h.client)
	return watcher
}

// requireDescribedWalk fails unless frame is objID's MoveToLocation to
// target, from home on a driven clock, or from somewhere on the walk
// between them on the wall clock.
func (h *petWorld) requireDescribedWalk(t *testing.T, frame []byte, objID int32, home, target location.Location, who string) {
	t.Helper()
	if frame[0] != serverpackets.OpcodeMoveToLocation {
		t.Fatalf("%s: frame after the summon's info = %#x, want MoveToLocation", who, frame[0])
	}
	r := wire.NewReader(frame[1:])
	id := r.ReadInt32()
	dest := location.Location{X: int(r.ReadInt32()), Y: int(r.ReadInt32()), Z: int(r.ReadInt32())}
	origin := location.Location{X: int(r.ReadInt32()), Y: int(r.ReadInt32()), Z: int(r.ReadInt32())}
	if id != objID || dest != target {
		t.Fatalf("%s: MoveToLocation object %d to %v, want %d to %v", who, id, dest, objID, target)
	}
	if h.srv.DrivesClock() {
		if origin != home {
			t.Fatalf("%s: MoveToLocation origin = %v, want the summon's position %v", who, origin, home)
		}
	} else if origin.Y != home.Y || origin.X < home.X || origin.X > target.X {
		t.Fatalf("%s: MoveToLocation origin = %v, want on the walk between %v and %v", who, origin, home, target)
	}
}

// requireNoMoveFor fails if frames hold a MoveToLocation for objID.
func requireNoMoveFor(t *testing.T, frames [][]byte, objID int32) {
	t.Helper()
	if i := frameIndex(frames, serverpackets.OpcodeMoveToLocation, objID); i >= 0 {
		t.Fatalf("frames = opcodes %x, MoveToLocation for the departed pet at %d", frameOpcodes(frames), i)
	}
}

// framePetInfo returns the index of objID's PetInfo, -1 when there is none.
func framePetInfo(frames [][]byte, objID int32) int {
	for i, frame := range frames {
		if frame[0] == serverpackets.OpcodePetInfo && len(frame) >= 9 && wire.NewReader(frame[5:]).ReadInt32() == objID {
			return i
		}
	}
	return -1
}

// frameIndexFrom returns the index of the first opcode frame at or after from,
// -1 when there is none or from is negative.
func frameIndexFrom(frames [][]byte, opcode byte, from int) int {
	if from <= 0 {
		return -1
	}
	for i := from; i < len(frames); i++ {
		if frames[i][0] == opcode {
			return i
		}
	}
	return -1
}
