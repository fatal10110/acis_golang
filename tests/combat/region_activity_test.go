package combat

import (
	"testing"
	"time"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/sim"
	"github.com/fatal10110/acis_golang/internal/gameserver/skill/effect"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
)

// TestRegionDeactivationResetsHostileOnItsQueue pins that the last player
// leaving a region resets the monsters in it — effects stopped, AI back to
// peace — as a task on each monster's own queue, not on the leaving
// player's. The player's logout drives the deactivation; the effect's exit
// hook runs inside the reset and reports which queue it ran on.
func TestRegionDeactivationResetsHostileOnItsQueue(t *testing.T) {
	t.Parallel()
	srv := gameservertest.Boot(t,
		gameservertest.WithCharacter("Newbie", 5, 0),
		gameservertest.WithWantChars(1),
	)
	c := srv.Client
	startInWorld(t, c)
	hostile := srv.SpawnHostileNPCAt(t, location.Location{X: hostileX, Y: hostileY, Z: hostileZ})
	drainUntilQuiet(t, c)
	srv.Settle(t)

	q := hostile.Queue()
	onQueue := make(chan bool, 1)
	hostile.EffectList().Add(&effect.Effect{
		Skill:    effect.Skill{ID: 1},
		Template: modelskill.EffectTemplate{Name: "test"},
		OnExit:   func(*effect.Effect) { onQueue <- ownsQueue(q) },
	})

	c.Send(encodeLogout())

	select {
	case owned := <-onQueue:
		if !owned {
			t.Fatal("region deactivation stopped the monster's effects off the monster's queue")
		}
	case <-time.After(5 * time.Second):
		t.Fatal("region deactivation never stopped the monster's effects")
	}
	srv.Settle(t)
	if got := hostile.EffectList().All(); len(got) != 0 {
		t.Fatalf("effects after region deactivation = %d, want 0", len(got))
	}
}

// ownsQueue reports whether the caller is running one of q's tasks.
func ownsQueue(q *sim.Queue) (owned bool) {
	defer func() {
		if recover() != nil {
			owned = false
		}
	}()
	sim.AssertOwner(q)
	return true
}
