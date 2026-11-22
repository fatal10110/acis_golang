package skills

import (
	"testing"
	"time"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/player"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/skill/effect"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
)

// landSelfEffect lands an effect of skill id on pc, cast by pc itself, on
// pc's queue.
func landSelfEffect(t *testing.T, srv *gameservertest.Server, pc *player.Character, id modelskill.ID, tmpl modelskill.EffectTemplate) *effect.Effect {
	t.Helper()
	e, err := effect.New(effect.Skill{ID: id, Level: 1, SkillType: "BUFF"}, tmpl)
	if err != nil {
		t.Fatalf("effect.New(%s): %v", tmpl.Name, err)
	}
	e.Effector, e.Effected = pc, pc
	onPlayerQueue(t, srv, pc.ObjectID(), func(pc *player.Character) { pc.EffectList().Add(e) })
	if !e.InUse() {
		t.Fatalf("%s (skill %d) did not go into use", tmpl.Name, id)
	}
	return e
}

// expireEffects lets d pass and runs the effect sweep once.
func expireEffects(t *testing.T, srv *gameservertest.Server, d time.Duration) {
	t.Helper()
	srv.Advance(t, d)
	srv.TickEffects()
}

// TestInvulnerabilityHoldsUntilItsLastSourceEnds overlays Invincible and
// Petrification (different stack groups) on one player, then a GM's own
// invulnerability. Each ending source leaves the player invulnerable while
// another is active, so a monster's landed hit still deals nothing; only
// when the last one ends does the same hit hurt (#5084).
func TestInvulnerabilityHoldsUntilItsLastSourceEnds(t *testing.T) {
	t.Parallel()
	srv := gameservertest.Boot(t,
		gameservertest.WithCharacter("Newbie", 5, 0),
		gameservertest.WithWantChars(1),
	)
	c, objID := srv.Client, srv.SoleObjectID(t)
	startInWorld(t, c)
	pc := onlineCharacter(t, srv, objID)
	attacker := srv.SpawnAttackingHostileNPCAt(t, location.Location{X: hostileX, Y: hostileY, Z: hostileZ})
	drainUntilQuiet(t, c)

	hitDealsNothing := func(when string) {
		t.Helper()
		before := srv.PlayerCurrentHP(t, objID)
		attacker.DoAttack(t, pc)
		if got := srv.PlayerCurrentHP(t, objID); got != before {
			t.Fatalf("%s: player HP after the monster's hit = %d, want unchanged %d", when, got, before)
		}
		drainUntilQuiet(t, c)
	}

	invincible := landSelfEffect(t, srv, pc, 9201, modelskill.EffectTemplate{Name: "Invincible", Time: 6, StackType: "invincibility", StackOrder: 1})
	petrified := landSelfEffect(t, srv, pc, 9202, modelskill.EffectTemplate{Name: "Petrification", Time: 2, StackType: "turn_stone", StackOrder: 1})
	drainUntilQuiet(t, c)

	expireEffects(t, srv, 2200*time.Millisecond)
	if petrified.InUse() || !invincible.InUse() {
		t.Fatalf("after 2.2s: petrification in use %v, invincible in use %v; want false, true", petrified.InUse(), invincible.InUse())
	}
	if !pc.Invul() {
		t.Fatal("petrification ending cleared invulnerability while Invincible is still in use")
	}
	hitDealsNothing("petrification ended, Invincible in use")

	// A GM's own invulnerability outlasts the effect.
	pc.SetInvul(true)
	expireEffects(t, srv, 4*time.Second)
	if invincible.InUse() {
		t.Fatal("Invincible still in use after its 6s")
	}
	if !pc.Invul() {
		t.Fatal("Invincible ending cleared the GM's own invulnerability")
	}
	hitDealsNothing("Invincible ended, GM invulnerability on")

	pc.SetInvul(false)
	if pc.Invul() {
		t.Fatal("player still invulnerable with every source ended")
	}
	before := srv.PlayerCurrentHP(t, objID)
	attacker.DoAttack(t, pc)
	if got := srv.PlayerCurrentHP(t, objID); got >= before {
		t.Fatalf("player HP after the monster's hit with every source ended = %d, want below %d", got, before)
	}
	drainUntilQuiet(t, c)
}

// TestMovementLockHoldsUntilItsLastSourceEnds lands two ImobileBuff skills
// of different stack groups on one player. The shorter one ending leaves
// the player immobilized, so a move request gets only its ActionFailed ack
// and the player stays put; when the last one ends the same
// request walks (#5084).
func TestMovementLockHoldsUntilItsLastSourceEnds(t *testing.T) {
	t.Parallel()
	srv := gameservertest.Boot(t,
		gameservertest.WithCharacter("Newbie", 5, 0),
		gameservertest.WithWantChars(1),
	)
	c, objID := srv.Client, srv.SoleObjectID(t)
	startInWorld(t, c)
	pc := onlineCharacter(t, srv, objID)
	drainUntilQuiet(t, c)

	short := landSelfEffect(t, srv, pc, 9211, modelskill.EffectTemplate{Name: "ImobileBuff", Time: 2, StackType: "lock_a", StackOrder: 1})
	long := landSelfEffect(t, srv, pc, 9212, modelskill.EffectTemplate{Name: "ImobileBuff", Time: 6, StackType: "lock_b", StackOrder: 1})
	drainUntilQuiet(t, c)

	expireEffects(t, srv, 2200*time.Millisecond)
	if short.InUse() || !long.InUse() {
		t.Fatalf("after 2.2s: short lock in use %v, long lock in use %v; want false, true", short.InUse(), long.InUse())
	}
	if !pc.Immobilized() {
		t.Fatal("the shorter lock ending cleared the movement lock while the longer one is in use")
	}
	drainUntilQuiet(t, c)
	x, y, z := srv.PlayerPosition(t, objID)
	c.Send(encodeMoveBackwardToLocation(int32(x+300), int32(y), int32(z)))
	expectGroundClickAck(t, c)
	srv.Settle(t)
	for _, f := range c.ReadQueued() {
		if f[0] == serverpackets.OpcodeMoveToLocation {
			t.Fatal("move request while still immobilized answered MoveToLocation")
		}
	}
	if gx, gy, gz := srv.PlayerPosition(t, objID); gx != x || gy != y || gz != z {
		t.Fatalf("immobilized player moved to (%d,%d,%d), want (%d,%d,%d)", gx, gy, gz, x, y, z)
	}
	drainUntilQuiet(t, c)

	expireEffects(t, srv, 4*time.Second)
	if long.InUse() {
		t.Fatal("the longer lock still in use after its 6s")
	}
	if pc.Immobilized() {
		t.Fatal("player still immobilized with every lock ended")
	}
	drainUntilQuiet(t, c)
	c.Send(encodeMoveBackwardToLocation(int32(x+300), int32(y), int32(z)))
	expectGroundClickAck(t, c)
	assertFrameOpcode(t, c.Read(), serverpackets.OpcodeMoveToLocation, "move request once every lock ended")
	drainUntilQuiet(t, c)
}
