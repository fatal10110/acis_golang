package pets

import (
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/item"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
)

// Reference: Player.restore (Player.java:4144-4150) relinks a pet still in
// the world to the restored player at character selection, before
// World.addPlayer and CharSelected. A connection lost before EnterWorld then
// runs the player's cleanup, whose _summon.unSummon (Player.java:6280-6283)
// reaches Pet.unSummon (Pet.java:340-355): a living pet hands its items to
// its owner, is stored and leaves the world's pet list, while a dead one
// stays in it for the next login. aCis revision in the outer repo.

// selectAndDrop selects the owner's character and loses the connection
// before EnterWorld, then waits until the selected player is out of the
// world and everything its departure queued has run.
func (o *offlinePetWorld) selectAndDrop(t *testing.T) {
	t.Helper()
	o.client.Send(encodeRequestGameStart(0))
	readUntilOpcode(t, o.client, serverpackets.OpcodeCharSelected, "CharSelected")
	if err := o.client.Close(); err != nil {
		t.Fatalf("close the selecting client: %v", err)
	}
	o.srv.AdvanceUntil(t, "selected owner out of the world", func() bool {
		_, ok := o.srv.State.Player(o.ownerID)
		return !ok
	})
	o.srv.Settle(t)
	o.client = o.srv.DialClient(t, o.srv.Account(), 1)
}

// TestRevivedPetLeavesWithALoadingScreenDrop: an owner whose pet was revived
// while it was away selects its character, which takes the pet over, and
// loses the connection before EnterWorld. The pet leaves with the session as
// on any logout: it is out of the world and of its owner's summon slot, its
// row is saved alive with its regained exp, and its adena is in the owner's
// rows, so the next login holds the adena and has no pet.
func TestRevivedPetLeavesWithALoadingScreenDrop(t *testing.T) {
	t.Parallel()
	o := leaveWolfDead(t)
	wolf := o.wolf
	o.npcResurrects(t)
	if wolf.Dead() {
		t.Fatal("monster's resurrection left the offline owner's pet dead")
	}

	o.selectAndDrop(t)
	if _, ok := o.srv.State.Object(wolf.ObjectID()); ok {
		t.Fatal("revived pet stayed in the world after its owner's loading-screen drop")
	}
	if _, ok := o.srv.State.Summon(o.ownerID); ok {
		t.Fatal("revived pet kept its owner's summon slot after the drop")
	}
	state := o.savedPetState(t)
	if state.CurHP <= 0 || state.Exp != 510 {
		t.Fatalf("pet saved with HP %v exp %d, want alive with 510 exp", state.CurHP, state.Exp)
	}
	if got := o.ownerItemCount(t, item.AdenaID); got != 40 {
		t.Fatalf("owner adena rows after the drop = %d, want the pet's 40", got)
	}
	if got := o.collarItemCount(t, item.AdenaID); got != 0 {
		t.Fatalf("adena still saved under the collar = %d, want none", got)
	}

	frames := o.relogIn(t)
	if sawPetInfo(frames, wolf.ObjectID()) {
		t.Fatal("next login got PetInfo for a pet that left with the dropped session")
	}
	if inst := o.ownerInventory(t).ItemByTemplateID(item.AdenaID); inst == nil || inst.Snapshot().Count != 40 {
		t.Fatal("next login does not hold the pet's 40 adena")
	}
}

// TestPetCorpseOutlastsALoadingScreenDrop: an owner whose pet lies dead
// selects its character, which takes the corpse over, and loses the
// connection before EnterWorld. The corpse stays where it lies, in its
// owner's summon slot and left behind again, with its adena, and the next
// login takes it over.
func TestPetCorpseOutlastsALoadingScreenDrop(t *testing.T) {
	t.Parallel()
	o := leaveWolfDead(t)
	wolf := o.wolf

	o.selectAndDrop(t)
	if obj, ok := o.srv.State.Summon(o.ownerID); !ok || obj.ObjectID() != wolf.ObjectID() {
		t.Fatal("a loading-screen drop took the pet corpse out of its owner's summon slot")
	}
	if _, ok := o.srv.State.Object(wolf.ObjectID()); !ok || !wolf.Dead() || !wolf.OwnerLeft() {
		t.Fatalf("pet corpse after the drop: in world %v, dead %v, owner left %v; want a corpse left behind", ok, wolf.Dead(), wolf.OwnerLeft())
	}
	if got := petItemCount(wolf, item.AdenaID); got != 40 {
		t.Fatalf("pet corpse carries %d adena after the drop, want 40", got)
	}

	frames := o.relogIn(t)
	if !sawPetInfo(frames, wolf.ObjectID()) || wolf.OwnerLeft() {
		t.Fatal("the next login did not take the pet corpse over")
	}
}
