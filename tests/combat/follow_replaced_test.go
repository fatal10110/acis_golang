package combat

import (
	"testing"
	"time"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/move"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
)

// costlyInstantSkillID is a self skill with no hit time, so its cast never
// stops the caster's movement, costing more MP than the attacker has.
const costlyInstantSkillID = 4

func costlyInstantSkill() modelskill.Definition {
	return modelskill.Definition{
		ID: costlyInstantSkillID, Level: 1, Activation: modelskill.ActivationActive, Target: modelskill.TargetSelf,
		MPConsume: 10_000, ReuseDelay: 60_000, StaticHitTime: true, StaticReuse: true, SkillType: "DUMMY",
	}
}

// bootFollowSkillPair is bootClickPair with the attacker knowing the
// self-targeted mid-swing skill and the costly instant skill.
func bootFollowSkillPair(t *testing.T) clickPair {
	t.Helper()
	return bootClickPairSeeded(t, 0, func(srv *gameservertest.Server, attackerID int32) {
		seedKnownSkill(t, srv, attackerID, midSwingSkillID, 1)
		seedKnownSkill(t, srv, attackerID, costlyInstantSkillID, 1)
	}, gameservertest.WithSkills(combatPersistence(t, []modelskill.Definition{midSwingSkill(), costlyInstantSkill()})))
}

// startFollowing has the attacker follow the victim 300 units away and
// waits for it to catch up.
func (p clickPair) startFollowing(t *testing.T) {
	t.Helper()
	p.walkVictimAway(t, 300)
	selectPlayerTarget(t, p.c, p.victimID)
	drainUntilQuiet(t, p.c)
	p.c.Send(encodeAction(p.victimID, int32(playerOrigin.X), int32(playerOrigin.Y), int32(playerOrigin.Z), false))
	assertFrameOpcode(t, mustRead(t, p.c, "follow ActionFailed"), serverpackets.OpcodeActionFailed, "follow ActionFailed")
	p.tickUntil(t, "the follower reaching the target", func() bool {
		ax, ay, az := p.srv.PlayerPosition(t, p.attackerID)
		vx, vy, vz := p.srv.PlayerPosition(t, p.victimID)
		return location.In3DRange(ax, ay, az, vx, vy, vz, playerFollowOffset+60)
	})
	drainUntilQuiet(t, p.c)
}

// assertFollowOver walks the victim 400 units off and lets three seconds of
// follow rechecks pass: the attacker sends no MoveToPawn toward it and stays
// where it was.
func (p clickPair) assertFollowOver(t *testing.T, after string) {
	t.Helper()
	ax, ay, _ := p.srv.PlayerPosition(t, p.attackerID)
	p.walkVictimAway(t, 400)
	p.tickFor(t, 3*time.Second)
	frames := readQuiet(p.c)
	if i := followMoveIndex(frames, p.attackerID, p.victimID); i >= 0 {
		t.Fatalf("attacker still followed the player after %s: opcodes %v", after, opcodes(frames))
	}
	if x, y, _ := p.srv.PlayerPosition(t, p.attackerID); x != ax || y != ay {
		t.Fatalf("attacker moved from (%d,%d) to (%d,%d) after %s", ax, ay, x, y, after)
	}
}

// tickFor lets d pass in movement-tick steps, running the movement-correction
// ticks a follow rechecks on.
func (p clickPair) tickFor(t *testing.T, d time.Duration) {
	t.Helper()
	for passed := time.Duration(0); passed < d; passed += move.PositionUpdateInterval {
		p.srv.Advance(t, move.PositionUpdateInterval)
		p.srv.TickPositions()
	}
}

// TestSkillCastEndsFollow has a following player cast a self skill. The cast
// replaces the follow: once it ends and the followed player walks off, the
// caster does not walk after it.
func TestSkillCastEndsFollow(t *testing.T) {
	t.Parallel()
	p := bootFollowSkillPair(t)
	p.startFollowing(t)

	p.c.Send(encodeRequestMagicSkillUse(midSwingSkillID, false, false))
	p.srv.AdvanceUntil(t, "the cast starting", func() bool { return p.srv.PlayerCastingNow(t, p.attackerID) })
	p.srv.AdvanceUntil(t, "the cast ending", func() bool { return !p.srv.PlayerCastingNow(t, p.attackerID) })
	drainUntilQuiet(t, p.c)

	p.assertFollowOver(t, "a skill cast")
}

// TestRefusedInstantSkillEndsFollow has a following player request an
// instant self skill it lacks the MP for. With no hit time nothing stops the
// player's movement, and the refused cast never ends a cast intention; the
// request still replaced the follow, so the player does not walk after the
// followed player again.
func TestRefusedInstantSkillEndsFollow(t *testing.T) {
	t.Parallel()
	p := bootFollowSkillPair(t)
	p.startFollowing(t)

	p.c.Send(encodeRequestMagicSkillUse(costlyInstantSkillID, false, false))
	assertSystemMessageNumber(t, mustRead(t, p.c, "not enough MP"), serverpackets.SystemMessageNotEnoughMP)
	drainUntilQuiet(t, p.c)

	p.assertFollowOver(t, "a refused instant skill")
}

// TestSitStandEndsFollow has a following player sit down and stand up. The
// sit replaces the follow: once standing, the player does not walk after
// the followed player when it walks off.
func TestSitStandEndsFollow(t *testing.T) {
	t.Parallel()
	p := bootClickPair(t, 0)
	p.startFollowing(t)

	sitPlayer(t, p.c)
	p.srv.Advance(t, 3*time.Second)
	p.c.Send(encodeRequestChangeWaitType(true))
	assertFrameOpcode(t, mustRead(t, p.c, "stand ChangeWaitType"), serverpackets.OpcodeChangeWaitType, "stand ChangeWaitType")
	p.srv.Advance(t, 3*time.Second)
	drainUntilQuiet(t, p.c)

	p.assertFollowOver(t, "a sit and stand")
}

// TestMidSwingCastReplacesQueuedFollow queues a follow behind a swing, then
// requests a skill during the same swing. The cast takes the queued slot:
// after the swing the skill is cast and the attacker never walks after the
// player.
func TestMidSwingCastReplacesQueuedFollow(t *testing.T) {
	t.Parallel()
	p := bootFollowSkillPair(t)
	if !p.srv.DrivesClock() {
		t.Skip("holding a swing open needs the driven clock")
	}
	p.walkVictimAway(t, 300)
	hostile := p.srv.SpawnHostileNPCAt(t, location.Location{X: hostileX - 20, Y: hostileY, Z: hostileZ})
	drainUntilQuiet(t, p.c)
	targetHostile(t, p.c, hostile.ObjectID())
	p.c.Send(encodeAction(hostile.ObjectID(), int32(playerOrigin.X), int32(playerOrigin.Y), int32(playerOrigin.Z), false))
	assertAttackBy(t, p.c, p.attackerID)

	selectPlayerTarget(t, p.c, p.victimID)
	p.c.Send(encodeAction(p.victimID, int32(playerOrigin.X), int32(playerOrigin.Y), int32(playerOrigin.Z), false))
	assertFrameOpcode(t, mustRead(t, p.c, "queued follow ActionFailed"), serverpackets.OpcodeActionFailed, "queued follow ActionFailed")
	p.c.Send(encodeRequestMagicSkillUse(midSwingSkillID, false, false))
	assertFrameOpcode(t, mustRead(t, p.c, "queued cast ActionFailed"), serverpackets.OpcodeActionFailed, "queued cast ActionFailed")

	p.srv.AdvanceUntil(t, "the queued cast starting", func() bool { return p.srv.PlayerCastingNow(t, p.attackerID) })
	p.tickFor(t, 3*time.Second)
	if frames := readQuiet(p.c); followMoveIndex(frames, p.attackerID, p.victimID) >= 0 {
		t.Fatalf("the replaced follow still ran: opcodes %v", opcodes(frames))
	}
	if x, _, _ := p.srv.PlayerPosition(t, p.attackerID); x != playerOrigin.X {
		t.Fatalf("attacker walked off to x=%d, want the replaced follow dropped", x)
	}
}

// TestConfusedForcedClickOnDeadPlayerDoesNotFollow has a confused player
// force (ctrl) a click on a dead player 300 units away, which it may not
// attack. An out-of-control player's attack request is only answered
// ActionFailed: it does not fall through to a follow.
func TestConfusedForcedClickOnDeadPlayerDoesNotFollow(t *testing.T) {
	t.Parallel()
	p := bootClickPair(t, 0)
	p.walkVictimAway(t, 300)
	p.srv.MarkPlayerDead(t, p.victimID)
	selectPlayerTarget(t, p.c, p.victimID)
	landEffect(t, onlineEffectHolder(t, p.srv, p.attackerID), "Confusion")
	drainUntilQuiet(t, p.c)

	p.c.Send(encodeAttackRequest(p.victimID, int32(playerOrigin.X), int32(playerOrigin.Y), int32(playerOrigin.Z), false))
	assertFrameOpcode(t, mustRead(t, p.c, "out-of-control ActionFailed"), serverpackets.OpcodeActionFailed, "out-of-control ActionFailed")
	p.tickFor(t, 3*time.Second)
	if frames := readQuiet(p.c); followMoveIndex(frames, p.attackerID, p.victimID) >= 0 {
		t.Fatalf("confused forced click followed the player: opcodes %v", opcodes(frames))
	}
	if x, _, _ := p.srv.PlayerPosition(t, p.attackerID); x != playerOrigin.X {
		t.Fatalf("confused attacker walked to x=%d", x)
	}
}
