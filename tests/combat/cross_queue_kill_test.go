package combat

import (
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/attackable"
	"github.com/fatal10110/acis_golang/internal/gameserver/network"
	"github.com/fatal10110/acis_golang/internal/gameserver/sim"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
)

// killableVictim is the slice of a live player this test drives from another
// actor's queue: the death sequence a lethal hit runs, and a revive so the
// same victim can die again.
type killableVictim interface {
	Die(killer attackable.Combatant) bool
	Revive(fraction float64) bool
	Queue() *sim.Queue
}

// TestPKCountersSurviveKillsOnTwoQueues kills two innocent victims over and
// over, each death running on its victim's own queue the way a damage-over-
// time tick does. Both deaths credit the same killer's PK count and karma, so
// on the worker pool the two queues update the killer at the same time; every
// kill must still count.
func TestPKCountersSurviveKillsOnTwoQueues(t *testing.T) {
	t.Parallel()
	const rounds = 25

	srv := gameservertest.Boot(t,
		gameservertest.WithCharacter("Killer", 5, 0),
		gameservertest.WithWantChars(1),
	)
	c, killerID := srv.Client, srv.SoleObjectID(t)
	first := srv.SeedCharacterFor(t, "victim1", "VictimOne", 1, 0)
	second := srv.SeedCharacterFor(t, "victim2", "VictimTwo", 1, 0)
	firstClient := srv.DialClient(t, "victim1", 1)
	secondClient := srv.DialClient(t, "victim2", 1)
	startInWorld(t, firstClient)
	startInWorld(t, secondClient)
	startInWorld(t, c)

	killerObj, ok := srv.State.Player(killerID)
	if !ok {
		t.Fatal("killer missing from world state")
	}
	killer, ok := network.OnlineCharacter(killerObj)
	if !ok {
		t.Fatalf("killer %T is not an online character", killerObj)
	}
	victims := make([]killableVictim, 0, 2)
	for _, id := range []int32{first.ID, second.ID} {
		obj, ok := srv.State.Player(id)
		if !ok {
			t.Fatalf("victim %d missing from world state", id)
		}
		victims = append(victims, obj.(killableVictim))
	}

	for range rounds {
		for _, v := range victims {
			v.Queue().Post(func() {
				if v.Die(killer) {
					v.Revive(1)
				}
			})
		}
	}
	srv.Settle(t)

	if got, want := killer.ProgressionValues().PKKills, 2*rounds; got != want {
		t.Fatalf("killer PK kills = %d, want %d: a kill credited on one queue was lost to the other", got, want)
	}
}
