package combat

import (
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/ai"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
)

// TestFirstCombatHateAttacksWithoutAITick pins that the first attack desire
// on an NPC with no current most-hated target runs the AI loop immediately.
// Fixture NPCs are not registered on the 1s AI task, so an idle intention
// after this call means the combat-hate path queued the desire and waited.
func TestFirstCombatHateAttacksWithoutAITick(t *testing.T) {
	srv := gameservertest.Boot(t,
		gameservertest.WithCharacter("Newbie", 5, 0),
		gameservertest.WithWantChars(1),
	)
	attacker := srv.SpawnHostileNPCAt(t, location.Location{X: hostileX, Y: hostileY, Z: hostileZ})
	defender := srv.SpawnHostileNPCAt(t, location.Location{X: hostileX + 40, Y: hostileY, Z: hostileZ})

	if got := defender.AI().CurrentIntention(); got != ai.IntentionIdle {
		t.Fatalf("CurrentIntention() before hate = %v, want %v", got, ai.IntentionIdle)
	}

	defender.AddCombatDamageHate(attacker, 50)

	if got := defender.AI().CurrentIntention(); got != ai.IntentionAttack {
		t.Fatalf("CurrentIntention() after first combat hate = %v, want %v (must not wait for AI tick)", got, ai.IntentionAttack)
	}
}

// TestAttackDesireWithExistingHateWaitsForTick pins that addAttackDesire
// skips the immediate AI loop when a most-hated attacker is already on the
// threat table. The desire still queues; the next tick promotes it.
func TestAttackDesireWithExistingHateWaitsForTick(t *testing.T) {
	srv := gameservertest.Boot(t,
		gameservertest.WithCharacter("Newbie", 5, 0),
		gameservertest.WithWantChars(1),
	)
	attacker := srv.SpawnHostileNPCAt(t, location.Location{X: hostileX, Y: hostileY, Z: hostileZ})
	defender := srv.SpawnHostileNPCAt(t, location.Location{X: hostileX + 40, Y: hostileY, Z: hostileZ})

	defender.AddDamageHate(attacker, 50, 50)
	defender.AddAttackDesire(attacker, 200)

	if got := defender.AI().CurrentIntention(); got != ai.IntentionIdle {
		t.Fatalf("CurrentIntention() with existing most-hated = %v, want %v", got, ai.IntentionIdle)
	}
	if got := defender.AI().Desires().Len(); got != 1 {
		t.Fatalf("queued desires = %d, want 1", got)
	}
}

// TestFirstAttackDesireWithoutMostHatedAttacksWithoutAITick pins the
// scripted/DOT addAttackDesire path: a threat entry with non-positive hate
// does not count as most-hated, so the first attack desire still runs the
// AI loop immediately.
func TestFirstAttackDesireWithoutMostHatedAttacksWithoutAITick(t *testing.T) {
	srv := gameservertest.Boot(t,
		gameservertest.WithCharacter("Newbie", 5, 0),
		gameservertest.WithWantChars(1),
	)
	attacker := srv.SpawnHostileNPCAt(t, location.Location{X: hostileX, Y: hostileY, Z: hostileZ})
	defender := srv.SpawnHostileNPCAt(t, location.Location{X: hostileX + 40, Y: hostileY, Z: hostileZ})

	defender.AddDamageHate(attacker, 10, 0)
	defender.AddAttackDesire(attacker, 200)

	if got := defender.AI().CurrentIntention(); got != ai.IntentionAttack {
		t.Fatalf("CurrentIntention() after first attack desire = %v, want %v (must not wait for AI tick)", got, ai.IntentionAttack)
	}
}
