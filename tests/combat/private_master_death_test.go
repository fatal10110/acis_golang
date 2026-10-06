package combat

import (
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/ai"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
)

// TestPrivateStaysAPrivateWhenItsMasterDies kills a master through Die,
// which lets its privates go. A private 500 off its own spawn point stays
// in territory and keeps wandering around where it stands instead of
// around its spawn point: a private stays one once its master is gone.
func TestPrivateStaysAPrivateWhenItsMasterDies(t *testing.T) {
	t.Parallel()
	srv := gameservertest.Boot(t,
		gameservertest.WithCharacter("Newbie", 5, 0),
		gameservertest.WithWantChars(1),
	)
	c := srv.Client
	startInWorld(t, c)

	home := location.Location{X: hostileX, Y: hostileY, Z: hostileZ}
	current := location.Location{X: hostileX + 500, Y: hostileY, Z: hostileZ}
	master := srv.SpawnMovingHostileNPCAt(t, "Monster", home, home)
	minion := srv.SpawnMovingHostileNPCAt(t, "Monster", home, current)
	master.AddMinion(minion)
	minion.SetMaster(master)
	if !minion.InTerritory() {
		t.Fatal("InTerritory() = false while the master lives, want true")
	}

	if !master.Die(nil, nil) {
		t.Fatal("master.Die() = false, want a fresh death")
	}
	if minion.Master() != nil {
		t.Fatal("the master's death left the private linked to it")
	}
	if minion.SpawnMaster() != master || !minion.IsPrivate() {
		t.Fatal("the master's death made the private a free spawn")
	}
	if !minion.InTerritory() {
		t.Fatal("InTerritory() = false after the master died, want a private always in territory")
	}
	drainUntilQuiet(t, c)

	tickThinkWander(t, minion)
	if got := minion.AI().CurrentIntention(); got != ai.IntentionWander {
		t.Fatalf("CurrentIntention() = %v, want wander after the master died", got)
	}
	assertChangeMoveType(t, mustRead(t, c, "ChangeMoveType"), minion.ObjectID(), false)
	dest := moveToLocationDest(t, mustRead(t, c, "MoveToLocation"))
	offset := 60 * 3
	if absInt(dest.X-current.X) > offset || absInt(dest.Y-current.Y) > offset || dest.Z != current.Z {
		t.Fatalf("private wander dest = %+v, want within ±%d of where it stands %+v", dest, offset, current)
	}
	if absInt(dest.X-home.X) <= offset && absInt(dest.Y-home.Y) <= offset {
		t.Fatalf("private wander dest = %+v, around its spawn point %+v", dest, home)
	}
}
