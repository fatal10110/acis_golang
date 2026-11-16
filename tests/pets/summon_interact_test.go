package pets

import (
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/summon"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/staticobject"
)

// spawnStaticNear spawns a town map dx east of the pet.
func spawnStaticNear(t *testing.T, h *petWorld, petActor *summon.Actor, dx int) (*staticobject.Object, location.Location) {
	t.Helper()
	x, y, z := petActor.Position()
	at := location.Location{X: x + dx, Y: y, Z: z}
	obj, err := staticobject.NewObject(h.srv.NewObjectID(), &staticobject.Template{
		ID: 24180018, Location: at, Type: staticobject.MapType, Texture: "testmap", MapX: 17, MapY: 21,
	})
	if err != nil {
		t.Fatal(err)
	}
	h.srv.State.Spawn(obj, at.X, at.Y, at.Z, 0)
	drainUntilQuiet(t, h.client)
	return obj, at
}

// TestSummonCommandOnStaticObject pins the pet's attack (16) and move-to
// (53) on an object that is no creature (RequestActionUse: tryToInteract;
// move-to first turns following off, which sends the pet idle). The interact
// intention (PlayableAI.tryToInteract, SummonAI.thinkInteract) stops whatever
// the pet followed and never walks it to the object: with SHIFT held and the
// object beyond the pet's attack range the pet goes idle, which for one
// following its owner means following it again; otherwise it keeps the
// intention where it stands.
func TestSummonCommandOnStaticObject(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name       string
		action     int32
		shift      bool
		follow     bool
		dx         int
		wantIntent summon.Intent
		wantFollow bool
	}{
		{"attack", petAttackAction, false, true, 300, summon.IntentInteractTarget, true},
		{"attack with shift out of reach, following", petAttackAction, true, true, 300, summon.IntentFollowOwner, true},
		{"attack with shift out of reach, not following", petAttackAction, true, false, 300, summon.IntentIdle, false},
		{"attack with shift in reach", petAttackAction, true, true, 20, summon.IntentInteractTarget, true},
		{"move-to", moveToTargetAction, false, true, 300, summon.IntentInteractTarget, false},
		{"move-to with shift out of reach", moveToTargetAction, true, true, 300, summon.IntentIdle, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			h, petActor := bootFollowingPet(t)
			if !tc.follow {
				h.toggleFollow(t)
			}
			obj, at := spawnStaticNear(t, h, petActor, tc.dx)

			h.commandAt(t, obj.ObjectID(), at, tc.action, tc.shift)
			if petActor.Move().Moving() {
				t.Fatal("pet started moving toward an object that is no creature")
			}
			if got := petActor.Intent(); got != tc.wantIntent {
				t.Fatalf("pet intent = %v, want %v", got, tc.wantIntent)
			}
			if got := petActor.FollowActive(); got != tc.wantFollow {
				t.Fatalf("pet FollowActive() = %v, want %v", got, tc.wantFollow)
			}
			if tc.wantIntent == summon.IntentFollowOwner {
				h.ownerWalksAway(t)
				h.requirePetCatchesUp(t, petActor, "pet gone idle while following its owner")
			} else {
				h.requirePetStaysPut(t, petActor, "pet sent to an object that is no creature")
				if got := petActor.Intent(); got != tc.wantIntent {
					t.Fatalf("pet intent after AI thinks = %v, want %v", got, tc.wantIntent)
				}
			}
			drainUntilQuiet(t, h.client)
		})
	}
}
