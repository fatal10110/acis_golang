package combat

import (
	"testing"
	"time"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/player"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
)

// The nextActionAttack fixture: short offensive ONE-target casts on the
// selected monster, with and without nextActionAttack, and one whose MP cost
// no caster can pay. The fixture monsters are parked and never swing back.
const (
	followUpOneSkillID    = 7
	plainOneSkillID       = 8
	followUpCostlySkillID = 9
	followUpLongSkillID   = 10
)

// followUpLongSkill is a nextActionAttack ONE-target cast long enough that
// the harness's reads, which move the driven clock while they wait, never
// leave its interrupt window.
func followUpLongSkill() modelskill.Definition {
	def := followUpOneSkill(followUpLongSkillID, true, 1)
	def.HitTime = queueHitTime
	return def
}

func followUpOneSkill(id int, nextActionAttack bool, mp int) modelskill.Definition {
	return modelskill.Definition{
		ID: modelskill.ID(id), Level: 1, Activation: modelskill.ActivationActive, Target: modelskill.TargetOne,
		CastRange: 600, HitTime: 500, StaticHitTime: true, ReuseDelay: 60_000, StaticReuse: true,
		MPConsume: mp, SkillType: "DUMMY", Offensive: true, NextActionIsAttack: nextActionAttack,
	}
}

// followUpCaster is a player beside two parked monsters, A and B, knowing
// the fixture skills and carrying the fixture scroll, whose attached skill
// is re-typed as a nextActionAttack ONE-target skill no caster can pay MP
// for.
type followUpCaster struct {
	srv    *gameservertest.Server
	c      *scriptedClient
	pc     *player.Character
	objID  int32
	a, b   int32
	scroll int32
	origin location.Location
}

func bootFollowUpCaster(t *testing.T) followUpCaster {
	t.Helper()
	srv := gameservertest.Boot(t,
		gameservertest.WithCharacter("Newbie", 5, 0),
		gameservertest.WithWantChars(1),
		gameservertest.WithSkills(combatPersistence(t, []modelskill.Definition{
			followUpOneSkill(followUpOneSkillID, true, 1),
			followUpOneSkill(plainOneSkillID, false, 1),
			followUpOneSkill(followUpCostlySkillID, true, 1_000_000),
			followUpOneSkill(queueScrollSkillID, true, 1_000_000),
			followUpLongSkill(),
		})),
	)
	if !srv.DrivesClock() {
		t.Skip("holding a cast open needs the driven clock")
	}
	c, objID := srv.Client, srv.SoleObjectID(t)
	for _, id := range []int{followUpOneSkillID, plainOneSkillID, followUpCostlySkillID, followUpLongSkillID} {
		seedKnownSkill(t, srv, objID, id, 1)
	}
	scroll := srv.GiveItem(t, objID, queueScrollTemplateID, 3)
	startInWorld(t, c)
	a := srv.SpawnHostileNPCAt(t, location.Location{X: hostileX - 20, Y: hostileY, Z: hostileZ})
	b := srv.SpawnHostileNPCAt(t, location.Location{X: hostileX - 20, Y: hostileY + 30, Z: hostileZ})
	drainUntilQuiet(t, c)
	return followUpCaster{srv: srv, c: c, pc: onlinePlayer(t, srv, objID), objID: objID, a: a.ObjectID(), b: b.ObjectID(), scroll: scroll, origin: playerOrigin}
}

// selectAmidFrames clicks id to select it while other frames (a swing's)
// may be interleaved with the selection's replies.
func (f followUpCaster) selectAmidFrames(t *testing.T, id int32) {
	t.Helper()
	f.c.Send(encodeAction(id, int32(f.origin.X), int32(f.origin.Y), int32(f.origin.Z), false))
	for i := 0; i < 100; i++ {
		frame := f.c.ReadWithTimeout(time.Second)
		if frame == nil {
			t.Fatalf("MyTargetSelected for %d never arrived", id)
		}
		if frame[0] == serverpackets.OpcodeMyTargetSelected && wireReader(frame[1:]).ReadInt32() == id {
			return
		}
	}
	t.Fatalf("MyTargetSelected for %d not found within 100 frames", id)
}

// attackMonster selects id, clicks it again to attack, and returns once the
// player's first swing at it is out.
func (f followUpCaster) attackMonster(t *testing.T, id int32) {
	t.Helper()
	f.selectAmidFrames(t, id)
	drainUntilQuiet(t, f.c)
	f.c.Send(encodeAction(id, int32(f.origin.X), int32(f.origin.Y), int32(f.origin.Z), false))
	if got := f.nextAttackTarget(t, "first swing"); got != id {
		t.Fatalf("first swing target = %d, want %d", got, id)
	}
}

// readUntilOwnMagicSkillUse reads up to the player's own MagicSkillUse.
func (f followUpCaster) readUntilOwnMagicSkillUse(t *testing.T, what string) {
	t.Helper()
	for i := 0; i < 100; i++ {
		frame := f.c.ReadWithTimeout(3 * time.Second)
		if frame == nil {
			t.Fatalf("%s never arrived", what)
		}
		if frame[0] == serverpackets.OpcodeMagicSkillUse && wireReader(frame[1:]).ReadInt32() == f.objID {
			return
		}
	}
	t.Fatalf("%s not found within 100 frames", what)
}

// nextAttackTarget reads up to the player's next Attack and returns the
// target it names.
func (f followUpCaster) nextAttackTarget(t *testing.T, what string) int32 {
	t.Helper()
	for i := 0; i < 100; i++ {
		frame := f.c.ReadWithTimeout(3 * time.Second)
		if frame == nil {
			t.Fatalf("%s: no Attack by the player", what)
		}
		if attackFrameBy(frame, f.objID) {
			return attackTargetID(frame)
		}
	}
	t.Fatalf("%s: no Attack by the player within 100 frames", what)
	return 0
}

// TestNextActionAttackSkillAttacksItsTarget pins the follow-up with no
// attack before the cast: a nextActionAttack skill cast on a monster ends in
// an attack on that monster.
func TestNextActionAttackSkillAttacksItsTarget(t *testing.T) {
	t.Parallel()
	f := bootFollowUpCaster(t)
	f.selectAmidFrames(t, f.a)
	drainUntilQuiet(t, f.c)

	f.c.Send(encodeRequestMagicSkillUse(followUpOneSkillID, false, false))
	f.readUntilOwnMagicSkillUse(t, "follow-up skill MagicSkillUse")
	if got := f.nextAttackTarget(t, "after the follow-up skill"); got != f.a {
		t.Fatalf("follow-up attack target = %d, want the cast's target %d", got, f.a)
	}
}

// TestNextActionAttackSkillAttacksItsTargetNotThePriorOne pins the follow-up
// target: a player attacking monster A who casts a nextActionAttack skill
// on monster B attacks B once the cast ends, not A.
func TestNextActionAttackSkillAttacksItsTargetNotThePriorOne(t *testing.T) {
	t.Parallel()
	f := bootFollowUpCaster(t)
	f.attackMonster(t, f.a)

	f.selectAmidFrames(t, f.b)
	f.c.Send(encodeRequestMagicSkillUse(followUpOneSkillID, false, false))
	f.readUntilOwnMagicSkillUse(t, "follow-up skill MagicSkillUse")
	if !f.pc.CastingNow() {
		t.Fatal("follow-up skill not in flight after its MagicSkillUse")
	}
	if got := f.nextAttackTarget(t, "after the follow-up skill"); got != f.b {
		t.Fatalf("follow-up attack target = %d, want the cast's target %d (not the prior target %d)", got, f.b, f.a)
	}
}

// TestPlainSkillEndsTheAttack is the other half: a skill without
// nextActionAttack, cast on the monster being attacked, leaves the player
// idle once it ends.
func TestPlainSkillEndsTheAttack(t *testing.T) {
	t.Parallel()
	f := bootFollowUpCaster(t)
	f.attackMonster(t, f.a)

	f.c.Send(encodeRequestMagicSkillUse(plainOneSkillID, false, false))
	f.readUntilOwnMagicSkillUse(t, "plain skill MagicSkillUse")
	f.srv.AdvanceUntil(t, "plain skill end", func() bool { return !f.pc.CastingNow() })
	assertNoAttackBy(t, f.srv, f.c, f.objID, 3*time.Second, "after the plain skill")
}

// TestNextActionAttackSkillRefusedForMPStillAttacks pins the cost-check
// refusal: a nextActionAttack skill the player cannot pay MP for is answered
// with NOT_ENOUGH_MP, then the player attacks the skill's target anyway.
func TestNextActionAttackSkillRefusedForMPStillAttacks(t *testing.T) {
	t.Parallel()
	f := bootFollowUpCaster(t)
	f.selectAmidFrames(t, f.a)
	drainUntilQuiet(t, f.c)

	f.c.Send(encodeRequestMagicSkillUse(followUpCostlySkillID, false, false))
	refusal := readUntil(t, f.c, serverpackets.OpcodeSystemMessage, "NOT_ENOUGH_MP")[0]
	if got := wireReader(refusal[1:]).ReadInt32(); got != int32(serverpackets.SystemMessageNotEnoughMP) {
		t.Fatalf("refusal system message = %d, want NOT_ENOUGH_MP (%d)", got, serverpackets.SystemMessageNotEnoughMP)
	}
	if got := f.nextAttackTarget(t, "after the refused follow-up skill"); got != f.a {
		t.Fatalf("attack after the refusal target = %d, want the skill's target %d", got, f.a)
	}
	if f.pc.CastingNow() {
		t.Fatal("refused skill started casting")
	}
}

// TestNextActionAttackItemSkillRefusedForMPStillAttacks pins the same
// cost-check refusal on the item-carried path: a scroll whose attached
// nextActionAttack skill the player cannot pay MP for is answered with
// NOT_ENOUGH_MP, then the player attacks the selected monster, and no cast
// starts.
func TestNextActionAttackItemSkillRefusedForMPStillAttacks(t *testing.T) {
	t.Parallel()
	f := bootFollowUpCaster(t)
	f.selectAmidFrames(t, f.a)
	drainUntilQuiet(t, f.c)

	f.c.Send(encodeUseItem(f.scroll, false))
	refusal := readUntil(t, f.c, serverpackets.OpcodeSystemMessage, "NOT_ENOUGH_MP")[0]
	if got := wireReader(refusal[1:]).ReadInt32(); got != int32(serverpackets.SystemMessageNotEnoughMP) {
		t.Fatalf("refusal system message = %d, want NOT_ENOUGH_MP (%d)", got, serverpackets.SystemMessageNotEnoughMP)
	}
	for i := 0; i < 100; i++ {
		frame := f.c.ReadWithTimeout(3 * time.Second)
		if frame == nil {
			t.Fatal("after the refused item skill: no Attack by the player")
		}
		if frame[0] == serverpackets.OpcodeMagicSkillUse && wireReader(frame[1:]).ReadInt32() == f.objID {
			t.Fatal("refused item skill sent MagicSkillUse")
		}
		if attackFrameBy(frame, f.objID) {
			if got := attackTargetID(frame); got != f.a {
				t.Fatalf("attack after the refusal target = %d, want the selected monster %d", got, f.a)
			}
			break
		}
	}
	if f.pc.CastingNow() {
		t.Fatal("refused item skill started casting")
	}
}

// TestBrokenNextActionAttackCastDoesNotAttack pins the follow-up on the
// abort path: a nextActionAttack cast broken inside its interrupt window
// (the damage and abort-cast break, InterruptCast) hands on to no attack.
// CreatureCast.stop (CreatureCast.java:404-434) notifies FINISHED_CASTING
// while the cast is still in flight, so the follow-up's PlayerAI.thinkAttack
// (PlayerAI.java:169-215) re-queues it behind the cast with ActionFailed,
// and PlayableCast.stop's tryToIdle (PlayableCast.java:101-107) then idles
// the player, dropping it. PlayerCast.stop answers ActionFailed, and
// CASTING_INTERRUPTED comes last (CreatureCast.java:439-446).
func TestBrokenNextActionAttackCastDoesNotAttack(t *testing.T) {
	t.Parallel()
	f := bootFollowUpCaster(t)
	f.selectAmidFrames(t, f.a)
	drainUntilQuiet(t, f.c)

	f.c.Send(encodeRequestMagicSkillUse(followUpLongSkillID, false, false))
	f.readUntilOwnMagicSkillUse(t, "follow-up skill MagicSkillUse")
	if !f.pc.CastingNow() {
		t.Fatal("follow-up skill not in flight after its MagicSkillUse")
	}
	drainUntilQuiet(t, f.c)

	onPlayerQueue(t, f.srv, f.objID, func(pc *player.Character) { pc.InterruptCast() })
	assertOpcodes(t, framesUntilQuiet(f.c), interruptReply, "the broken follow-up skill")
	assertStaysIdle(t, f.srv, f.pc)
}
