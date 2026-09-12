package combat

import (
	"testing"
	"time"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/ai"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/attack"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/attackable"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/creature"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
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

// assertLandedDamagingHit parses frame's first Attack hit and asserts it
// targeted targetID with real, non-missed damage. Run before checking what
// actually happened to the target's HP, it proves the swing itself could
// have hurt the target — so a passing "HP unchanged" assertion afterward
// means a guard blocked real damage, not that the swing missed or rolled
// zero anyway.
func assertLandedDamagingHit(t *testing.T, frame []byte, targetID int32) int32 {
	t.Helper()
	r := wireReader(frame[1:])
	r.ReadInt32() // attacker id, already checked by assertAttackBy
	if got := r.ReadInt32(); got != targetID {
		t.Fatalf("Attack target id = %d, want %d", got, targetID)
	}
	damage := r.ReadInt32()
	flags := r.ReadUint8()
	if flags&attack.HitMiss != 0 {
		t.Fatal("Attack hit missed, want a landed hit")
	}
	if damage <= 0 {
		t.Fatalf("Attack hit damage = %d, want > 0", damage)
	}
	return damage
}

// TestNonInvulnerablePlayerTakesMeleeDamage is the positive control for
// TestInvulnerablePlayerTakesNoMeleeDamage /
// TestSpawnProtectedPlayerTakesNoMeleeDamage: the same NPC auto-attack
// against a normal (non-invul) player deals its full landed damage,
// non-lethally, so the invul tests' "HP unchanged" is proven to mean the
// guard blocked a real hit rather than a miss or a zero-damage roll.
func TestNonInvulnerablePlayerTakesMeleeDamage(t *testing.T) {
	srv := gameservertest.Boot(t,
		gameservertest.WithCharacter("Newbie", 5, 0),
		gameservertest.WithWantChars(1),
	)
	c := srv.Client
	startInWorld(t, c)
	objID := srv.SoleObjectID(t)
	victim := livePlayer(t, srv, objID)
	attacker := srv.SpawnAttackingHostileNPCAt(t, location.Location{X: hostileX, Y: hostileY, Z: hostileZ})
	drainUntilQuiet(t, c)

	beforeHP := srv.PlayerCurrentHP(t, objID)

	attacker.DoAttack(t, victim.(attackable.Combatant), 5*time.Second)
	frame := assertAttackBy(t, c, attacker.ObjectID())
	damage := assertLandedDamagingHit(t, frame, objID)

	afterHP := srv.PlayerCurrentHP(t, objID)
	if want := beforeHP - int(damage); afterHP != want {
		t.Fatalf("player HP after melee = %d, want %d (before %d minus landed damage %d)", afterHP, want, beforeHP, damage)
	}
	if afterHP <= 0 {
		t.Fatalf("player HP after melee = %d, want a survived non-lethal hit", afterHP)
	}
}

// TestInvulnerablePlayerTakesNoMeleeDamage pins PlayerStatus.java:106-116:
// melee damage from a real NPC auto-attack against an invulnerable player
// (GM invul, mid-teleport) is dropped before any HP/CP change, while the
// Attack frame itself still goes out (PlayerStatus never suppresses the
// swing, only its damage).
func TestInvulnerablePlayerTakesNoMeleeDamage(t *testing.T) {
	srv := gameservertest.Boot(t,
		gameservertest.WithCharacter("Newbie", 5, 0),
		gameservertest.WithWantChars(1),
	)
	c := srv.Client
	startInWorld(t, c)
	objID := srv.SoleObjectID(t)
	victim := livePlayer(t, srv, objID)
	attacker := srv.SpawnAttackingHostileNPCAt(t, location.Location{X: hostileX, Y: hostileY, Z: hostileZ})
	drainUntilQuiet(t, c)

	victim.SetInvul(true)
	beforeHP := srv.PlayerCurrentHP(t, objID)

	attacker.DoAttack(t, victim.(attackable.Combatant), 5*time.Second)
	frame := assertAttackBy(t, c, attacker.ObjectID())
	assertLandedDamagingHit(t, frame, objID)

	if got := srv.PlayerCurrentHP(t, objID); got != beforeHP {
		t.Fatalf("invulnerable player HP after melee = %d, want unchanged %d", got, beforeHP)
	}
}

// TestSpawnProtectedPlayerTakesNoMeleeDamage covers the spawn-protection leg
// of the same guard (Invul() also reports true for SpawnProtected(),
// character_effects.go:82), using the real protection window activated by a
// restart-point teleport rather than SetInvul directly, driven through a
// real NPC auto-attack.
func TestSpawnProtectedPlayerTakesNoMeleeDamage(t *testing.T) {
	srv := gameservertest.Boot(t,
		gameservertest.WithCharacter("Newbie", 5, 0),
		gameservertest.WithWantChars(1),
		gameservertest.WithRestartPoints(restartTable()),
		gameservertest.WithSpawnProtection(5*time.Second),
	)
	enterAfterRestart(t, srv)
	c := srv.Client
	objID := srv.SoleObjectID(t)
	victim := livePlayer(t, srv, objID)
	attacker := srv.SpawnAttackingHostileNPCAt(t, location.Location{X: hostileX, Y: hostileY, Z: hostileZ})
	drainUntilQuiet(t, c)

	beforeHP := srv.PlayerCurrentHP(t, objID)

	attacker.DoAttack(t, victim.(attackable.Combatant), 5*time.Second)
	frame := assertAttackBy(t, c, attacker.ObjectID())
	assertLandedDamagingHit(t, frame, objID)

	if got := srv.PlayerCurrentHP(t, objID); got != beforeHP {
		t.Fatalf("spawn-protected player HP after melee = %d, want unchanged %d", got, beforeHP)
	}
}

// deniedDamageAttacker is a minimal creature.DeathActor standing in for a GM
// whose access level has canGiveDamage=false (character_effects.go:107):
// production code only reaches CanGiveDamage() through this interface, so
// the melee target under test cannot distinguish this from a real GM
// character.
type deniedDamageAttacker struct{ objID int32 }

func (deniedDamageAttacker) CanGiveDamage() bool { return false }
func (a deniedDamageAttacker) ObjectID() int32   { return a.objID }

// TestNoDamagePermissionAttackerStillWakesSeatedPlayer pins the split in
// PlayerStatus.reduceHp: the sleep/immobile-stop and stand-up block
// (PlayerStatus.java:118-134) runs before the canGiveDamage check
// (:136-140), so a hit from a damage-denied attacker still stands the
// player up even though it deals no damage.
func TestNoDamagePermissionAttackerStillWakesSeatedPlayer(t *testing.T) {
	srv := gameservertest.Boot(t,
		gameservertest.WithCharacter("Newbie", 5, 0),
		gameservertest.WithWantChars(1),
	)
	c := srv.Client
	startInWorld(t, c)
	objID := srv.SoleObjectID(t)
	victim := livePlayer(t, srv, objID)
	attacker := deniedDamageAttacker{objID: srv.NewObjectID()}
	sitPlayer(t, c)
	drainUntilQuiet(t, c)

	beforeHP := srv.PlayerCurrentHP(t, objID)

	if newlyDead := victim.TakeDamage(50, attacker); newlyDead {
		t.Fatal("damage-denied attacker killed seated player via melee")
	}
	assertFrameOpcode(t, mustRead(t, c, "stand ChangeWaitType"), serverpackets.OpcodeChangeWaitType, "stand ChangeWaitType")
	if got := srv.PlayerCurrentHP(t, objID); got != beforeHP {
		t.Fatalf("seated player HP after damage-denied melee = %d, want unchanged %d", got, beforeHP)
	}
}

// TestInvulnerableHostileTakesNoMeleeDamage pins the NPC counterpart
// (CreatureStatus.java:209-219) through the real client attack flow: melee
// damage against an invulnerable NPC is dropped, but — matching
// Npc.reduceCurrentHp (Npc.java:390-464), which runs hate/party/shot-roll
// unconditionally one layer above the invul guard — the NPC still aggroes
// on the attacker.
func TestInvulnerableHostileTakesNoMeleeDamage(t *testing.T) {
	srv := gameservertest.Boot(t,
		gameservertest.WithCharacter("Newbie", 5, 0),
		gameservertest.WithWantChars(1),
	)
	c, objID := srv.Client, srv.SoleObjectID(t)
	startInWorld(t, c)
	target := srv.SpawnHostileNPCAt(t, location.Location{X: hostileX - 20, Y: hostileY, Z: hostileZ})
	drainUntilQuiet(t, c)

	target.SetInvul(true)
	beforeHP := target.CurrentHP()

	targetHostile(t, c, target.ObjectID())
	c.Send(encodeAction(target.ObjectID(), int32(playerOrigin.X), int32(playerOrigin.Y), int32(playerOrigin.Z), false))
	assertAutoAttackStart(t, c, objID)
	assertAttackBy(t, c, objID)

	waitFor(t, "invulnerable NPC registers attacker hate", func() bool {
		return target.AI().CurrentIntention() == ai.IntentionAttack
	})
	if got := target.CurrentHP(); got != beforeHP {
		t.Fatalf("invulnerable NPC HP after melee = %d, want unchanged %d", got, beforeHP)
	}
}

// TestAttackerWithoutDamagePermissionDealsNoMeleeDamage pins
// CreatureStatus.java:221-226 through the real client attack flow: an
// attacker whose access level lacks canGiveDamage deals no melee damage,
// even to a live target, but — same Npc.reduceCurrentHp layering as
// above — the NPC still aggroes on the attacker.
func TestAttackerWithoutDamagePermissionDealsNoMeleeDamage(t *testing.T) {
	srv := gameservertest.Boot(t,
		gameservertest.WithCharacter("Newbie", 5, 0),
		gameservertest.WithWantChars(1),
	)
	c, objID := srv.Client, srv.SoleObjectID(t)
	startInWorld(t, c)
	target := srv.SpawnHostileNPCAt(t, location.Location{X: hostileX - 20, Y: hostileY, Z: hostileZ})
	drainUntilQuiet(t, c)

	attacker := livePlayer(t, srv, objID)
	attacker.SetCanGiveDamage(false)
	beforeHP := target.CurrentHP()

	targetHostile(t, c, target.ObjectID())
	c.Send(encodeAction(target.ObjectID(), int32(playerOrigin.X), int32(playerOrigin.Y), int32(playerOrigin.Z), false))
	assertAutoAttackStart(t, c, objID)
	assertAttackBy(t, c, objID)

	waitFor(t, "NPC registers damage-denied attacker hate", func() bool {
		return target.AI().CurrentIntention() == ai.IntentionAttack
	})
	if got := target.CurrentHP(); got != beforeHP {
		t.Fatalf("NPC HP after damage-denied melee = %d, want unchanged %d", got, beforeHP)
	}
}
