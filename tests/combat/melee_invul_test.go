package combat

import (
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/creature"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
)

// meleeVictim is the melee-damage surface under test on both Character and
// Hostile: apply a hit, report whether it newly killed the target.
type meleeVictim interface {
	TakeDamage(int, creature.DeathActor) bool
	SetInvul(bool) bool
}

// meleeAttacker is the damage-permission surface under test on Character.
type meleeAttacker interface {
	creature.DeathActor
	SetCanGiveDamage(bool)
}

// livePlayer returns the sole booted client's live player, asserted against
// the melee interfaces under test rather than the concrete *player.Character
// the world stores it as wrapped in (network.livePlayer embeds it).
func livePlayer(t *testing.T, srv *gameservertest.Server, objID int32) interface {
	meleeVictim
	meleeAttacker
} {
	t.Helper()
	obj, ok := srv.State.Player(objID)
	if !ok {
		t.Fatalf("world.Player(%d) missing", objID)
	}
	victim, ok := obj.(interface {
		meleeVictim
		meleeAttacker
	})
	if !ok {
		t.Fatalf("world.Player(%d) = %T, does not expose melee test surface", objID, obj)
	}
	return victim
}

// TestInvulnerablePlayerTakesNoMeleeDamage pins PlayerStatus.java:106-116:
// melee damage from another actor against an invulnerable player (spawn
// protection, GM invul, mid-teleport) is dropped before any HP/CP change.
func TestInvulnerablePlayerTakesNoMeleeDamage(t *testing.T) {
	srv := gameservertest.Boot(t,
		gameservertest.WithCharacter("Newbie", 5, 0),
		gameservertest.WithWantChars(1),
	)
	startInWorld(t, srv.Client)
	objID := srv.SoleObjectID(t)
	victim := livePlayer(t, srv, objID)
	attacker := srv.SpawnHostileNPCAt(t, location.Location{X: hostileX, Y: hostileY, Z: hostileZ})

	victim.SetInvul(true)
	beforeHP := srv.PlayerCurrentHP(t, objID)

	if newlyDead := victim.TakeDamage(beforeHP*10+1000, attacker); newlyDead {
		t.Fatal("invulnerable player died to melee damage")
	}
	if got := srv.PlayerCurrentHP(t, objID); got != beforeHP {
		t.Fatalf("invulnerable player HP after melee = %d, want unchanged %d", got, beforeHP)
	}
}

// TestInvulnerableHostileTakesNoMeleeDamage pins the NPC counterpart
// (CreatureStatus.java:209-219): melee damage against an invulnerable NPC is
// dropped before any HP change.
func TestInvulnerableHostileTakesNoMeleeDamage(t *testing.T) {
	srv := gameservertest.Boot(t,
		gameservertest.WithCharacter("Newbie", 5, 0),
		gameservertest.WithWantChars(1),
	)
	startInWorld(t, srv.Client)
	objID := srv.SoleObjectID(t)
	attacker := livePlayer(t, srv, objID)
	target := srv.SpawnHostileNPCAt(t, location.Location{X: hostileX, Y: hostileY, Z: hostileZ})

	target.SetInvul(true)
	beforeHP := target.CurrentHP()

	if newlyDead := target.TakeDamage(beforeHP*10+1000, attacker); newlyDead {
		t.Fatal("invulnerable NPC died to melee damage")
	}
	if got := target.CurrentHP(); got != beforeHP {
		t.Fatalf("invulnerable NPC HP after melee = %d, want unchanged %d", got, beforeHP)
	}
}

// TestAttackerWithoutDamagePermissionDealsNoMeleeDamage pins
// CreatureStatus.java:221-226: an attacker whose access level lacks
// canGiveDamage deals no melee damage, even to a live target.
func TestAttackerWithoutDamagePermissionDealsNoMeleeDamage(t *testing.T) {
	srv := gameservertest.Boot(t,
		gameservertest.WithCharacter("Newbie", 5, 0),
		gameservertest.WithWantChars(1),
	)
	startInWorld(t, srv.Client)
	objID := srv.SoleObjectID(t)
	attacker := livePlayer(t, srv, objID)
	target := srv.SpawnHostileNPCAt(t, location.Location{X: hostileX, Y: hostileY, Z: hostileZ})

	attacker.SetCanGiveDamage(false)
	beforeHP := target.CurrentHP()

	if newlyDead := target.TakeDamage(beforeHP*10+1000, attacker); newlyDead {
		t.Fatal("damage-denied attacker killed NPC via melee")
	}
	if got := target.CurrentHP(); got != beforeHP {
		t.Fatalf("NPC HP after damage-denied melee = %d, want unchanged %d", got, beforeHP)
	}
}
