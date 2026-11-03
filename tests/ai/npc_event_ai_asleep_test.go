package ai

import (
	"testing"

	actorai "github.com/fatal10110/acis_golang/internal/gameserver/model/actor/ai"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/npc"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
)

// Reference: AbstractAI.notifyEvent refuses an event only for an actor that
// is neither visible nor teleporting; AiTaskManager membership, which
// isAISleeping mirrors, gates the periodic runAI alone.

// sleepingDuel spawns two monsters in territory in a quiet region far from
// the character, both asleep, the first in reach of the second, and returns
// them.
func sleepingDuel(t *testing.T, srv *gameservertest.Server, at location.Location) (*npc.Hostile, *npc.Hostile) {
	t.Helper()
	far := offset(at, 40000)
	tmpl := gameservertest.MovingHostileTemplate("Monster")
	tmpl.BaseAttackRange = 40
	h := spawnLatched(t, srv, tmpl, far, far)
	victim := spawnLatched(t, srv, gameservertest.MovingHostileTemplate("Monster"), offset(far, 30), offset(far, 30))
	if _, active := srv.State.RegionActivity(h); active {
		t.Fatal("test setup: the far region is active")
	}
	if !h.AISleeping() || !victim.AISleeping() {
		t.Fatal("test setup: a monster spawned in territory in a quiet region is awake")
	}
	return h, victim
}

// TestSleepingMonsterRunsEventAI pins a monster asleep in a quiet region
// reacting to an attack event from another NPC: desire selection run by the
// first attack desire, a finished swing or RunAI promotes the attack, while
// the AI task still leaves it alone.
func TestSleepingMonsterRunsEventAI(t *testing.T) {
	t.Parallel()
	srv, at := bootSleepLatch(t)
	for _, tc := range []struct {
		name  string
		event func(t *testing.T, h, victim *npc.Hostile) error
	}{
		{"first attack desire", func(t *testing.T, h, victim *npc.Hostile) error {
			h.AddAttackDesire(victim, 5)
			return nil
		}},
		{"RunAI", func(t *testing.T, h, victim *npc.Hostile) error {
			queueAttack(t, h, victim)
			return h.RunAI()
		}},
		{"AttackFinished", func(t *testing.T, h, victim *npc.Hostile) error {
			queueAttack(t, h, victim)
			return h.AttackFinished()
		}},
	} {
		h, victim := sleepingDuel(t, srv, at)
		onNPCQueue(t, srv, h, func() {
			if err := tc.event(t, h, victim); err != nil {
				t.Errorf("%s: error: %v", tc.name, err)
			}
		})
		if got := h.AI().CurrentIntention(); got != actorai.IntentionAttack {
			t.Fatalf("%s: CurrentIntention() = %v, want %v", tc.name, got, actorai.IntentionAttack)
		}
		assertLatch(t, srv, h, true, tc.name+" in the quiet region")
	}
}

// TestOffGridMonsterRunsNoEventAI pins the one event gate: a monster taken
// off the grid runs no desire selection on an event.
func TestOffGridMonsterRunsNoEventAI(t *testing.T) {
	t.Parallel()
	srv, at := bootSleepLatch(t)
	h, victim := sleepingDuel(t, srv, at)
	onNPCQueue(t, srv, h, func() {
		srv.State.Despawn(h)
		queueAttack(t, h, victim)
		if err := h.RunAI(); err != nil {
			t.Errorf("RunAI() error: %v", err)
		}
	})
	if placed, _ := srv.State.RegionActivity(h); placed {
		t.Fatal("test setup: the despawned monster is still on the grid")
	}
	if got := h.AI().CurrentIntention(); got == actorai.IntentionAttack {
		t.Fatalf("CurrentIntention() off the grid = %v, want no attack", got)
	}
}

// queueAttack queues an attack desire on victim without running desire
// selection: victim is already the most hated.
func queueAttack(t *testing.T, h, victim *npc.Hostile) {
	t.Helper()
	h.AddDamageHate(victim, 0, 100)
	h.AddAttackDesire(victim, 5)
	if got := h.AI().CurrentIntention(); got == actorai.IntentionAttack {
		t.Errorf("test setup: queueing the attack desire ran desire selection (CurrentIntention() = %v)", got)
	}
}
