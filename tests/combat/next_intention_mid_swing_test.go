package combat

import (
	"testing"
	"time"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/cast"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/player"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
)

// swingTarget is the monster bootMidSwing's player selected and swings at.
func swingTarget(t *testing.T, pc *player.Character) int32 {
	t.Helper()
	target := pc.Target()
	if target == nil {
		t.Fatal("the swinging player selects nothing")
	}
	return target.ObjectID()
}

// nextSwingTarget reads until objID's next Attack and returns its target.
// A MagicSkillUse by objID on the way fails the test: nothing queued may
// cast.
func nextSwingTarget(t *testing.T, c *scriptedClient, objID int32, what string) int32 {
	t.Helper()
	for i := 0; i < 100; i++ {
		frame := c.ReadWithTimeout(3 * time.Second)
		if frame == nil {
			t.Fatalf("%s: no next swing", what)
		}
		if frame[0] == serverpackets.OpcodeMagicSkillUse && wireReader(frame[1:]).ReadInt32() == objID {
			t.Fatalf("%s: the player cast", what)
		}
		if attackFrameBy(frame, objID) {
			return attackTargetID(frame)
		}
	}
	t.Fatalf("%s: no next swing within 100 frames", what)
	return 0
}

// TestMidSwingAttackDroppedBySilenceKeepsCurrentTarget pins the current
// intention surviving an attack queued behind the swing: a click on a second
// monster mid-swing only sets the next intention
// (PlayableAI.tryToAttack, PlayableAI.java:243-250), SilenceMagicPhysical's
// tryToIdle mid-swing replaces just that next intention
// (PlayableAI.java:362-367), and the swing's end re-thinks the current
// attack (PlayableAI.onEvtFinishedAttack, PlayableAI.java:66-77): the next
// swing hits the first monster.
func TestMidSwingAttackDroppedBySilenceKeepsCurrentTarget(t *testing.T) {
	t.Parallel()
	srv, pc := bootMidSwing(t)
	c, objID := srv.Client, srv.SoleObjectID(t)
	first := swingTarget(t, pc)
	second := srv.SpawnHostileNPCAt(t, location.Location{X: hostileX - 20, Y: hostileY + 20, Z: hostileZ})

	// Select, then attack, the second monster; nothing is read meanwhile, so
	// the driven clock holds the swing open.
	c.Send(encodeAction(second.ObjectID(), hostileX-20, hostileY+20, hostileZ, false))
	srv.Settle(t)
	c.Send(encodeAction(second.ObjectID(), hostileX-20, hostileY+20, hostileZ, false))
	srv.Settle(t)
	landSilenceMagicPhysical(t, srv, objID)
	srv.Settle(t)

	if got := nextSwingTarget(t, c, objID, "after the silence"); got != first {
		t.Fatalf("next swing target = %d, want the current attack's %d, not the dropped %d", got, first, second.ObjectID())
	}
}

// TestMidSwingAttackRunsQueuedTarget is the control: left alone, the attack
// queued behind the swing runs when the swing ends.
func TestMidSwingAttackRunsQueuedTarget(t *testing.T) {
	t.Parallel()
	srv, _ := bootMidSwing(t)
	c, objID := srv.Client, srv.SoleObjectID(t)
	second := srv.SpawnHostileNPCAt(t, location.Location{X: hostileX - 20, Y: hostileY + 20, Z: hostileZ})

	c.Send(encodeAction(second.ObjectID(), hostileX-20, hostileY+20, hostileZ, false))
	srv.Settle(t)
	c.Send(encodeAction(second.ObjectID(), hostileX-20, hostileY+20, hostileZ, false))
	srv.Settle(t)

	if got := nextSwingTarget(t, c, objID, "after the swing"); got != second.ObjectID() {
		t.Fatalf("next swing target = %d, want the queued %d", got, second.ObjectID())
	}
}

// TestMidSwingCastDroppedBySilenceKeepsAttacking pins the same for a cast
// queued behind the swing (PlayableAI.tryToCast, PlayableAI.java:312-318):
// the attack the swing is for stays current, so once SilenceMagicPhysical
// drops the queued cast the swing's end swings at the same monster again,
// and the cast never starts.
func TestMidSwingCastDroppedBySilenceKeepsAttacking(t *testing.T) {
	t.Parallel()
	srv, pc := bootMidSwing(t)
	c, objID := srv.Client, srv.SoleObjectID(t)
	first := swingTarget(t, pc)

	c.Send(encodeRequestMagicSkillUse(midSwingSkillID, false, false))
	assertFrameOpcode(t, mustRead(t, c, "queued ActionFailed"), serverpackets.OpcodeActionFailed, "queued ActionFailed")
	landSilenceMagicPhysical(t, srv, objID)
	srv.Settle(t)

	if got := nextSwingTarget(t, c, objID, "after the silence"); got != first {
		t.Fatalf("next swing target = %d, want %d", got, first)
	}
	if srv.PlayerCastingNow(t, objID) {
		t.Fatal("the cast the silence dropped started")
	}
}

// TestMidSwingQueuedCastRefusedAtSwingEndDoesNotAttack pins the queued cast
// replacing the attack at the swing's end whatever its outcome
// (PlayableAI.onEvtFinishedAttack runs the next intention,
// PlayableAI.java:66-77): a skill queued mid-swing, then put on reuse, is
// refused when the swing ends, and the attack the swing was for does not go
// on, not even when the AI is woken afterwards: it is no longer current.
func TestMidSwingQueuedCastRefusedAtSwingEndDoesNotAttack(t *testing.T) {
	t.Parallel()
	srv, pc := bootMidSwing(t)
	c, objID := srv.Client, srv.SoleObjectID(t)

	c.Send(encodeRequestMagicSkillUse(midSwingSkillID, false, false))
	assertFrameOpcode(t, mustRead(t, c, "queued ActionFailed"), serverpackets.OpcodeActionFailed, "queued ActionFailed")
	pc.DisableSkill(cast.ReuseKey(midSwingSkill()), 10*time.Minute)

	assertNoAttackFor(t, c, 3*time.Second, "after the refused queued cast")
	onPlayerQueue(t, srv, objID, func(pc *player.Character) { pc.WakeAI() })
	assertNoAttackFor(t, c, 2*time.Second, "after the AI woke up")
	if srv.PlayerCastingNow(t, objID) {
		t.Fatal("the queued cast on reuse started")
	}
}

// bootMidSwingWithScroll is bootMidSwing with a scroll whose attached skill
// runs through the item AI-cast path.
func bootMidSwingWithScroll(t *testing.T) (*gameservertest.Server, int32, int32) {
	t.Helper()
	srv := gameservertest.Boot(t,
		gameservertest.WithCharacter("Newbie", 5, 0),
		gameservertest.WithWantChars(1),
		gameservertest.WithSkills(combatPersistence(t, []modelskill.Definition{queueScrollSkill()})),
	)
	if !srv.DrivesClock() {
		t.Skip("holding a swing open needs the driven clock")
	}
	c, objID := srv.Client, srv.SoleObjectID(t)
	scroll := srv.GiveItem(t, objID, queueScrollTemplateID, 3)
	startInWorld(t, c)
	hostile := srv.SpawnHostileNPCAt(t, location.Location{X: hostileX - 20, Y: hostileY, Z: hostileZ})
	drainUntilQuiet(t, c)
	targetHostile(t, c, hostile.ObjectID())
	c.Send(encodeAction(hostile.ObjectID(), int32(playerOrigin.X), int32(playerOrigin.Y), int32(playerOrigin.Z), false))
	assertAttackBy(t, c, objID)
	return srv, hostile.ObjectID(), scroll
}

// TestMidSwingItemCastDroppedBySilenceKeepsAttacking is
// TestMidSwingCastDroppedBySilenceKeepsAttacking for a scroll's skill queued
// behind the swing through the item AI-cast path.
func TestMidSwingItemCastDroppedBySilenceKeepsAttacking(t *testing.T) {
	t.Parallel()
	srv, first, scroll := bootMidSwingWithScroll(t)
	c, objID := srv.Client, srv.SoleObjectID(t)

	c.Send(encodeUseItem(scroll, false))
	readUntil(t, c, serverpackets.OpcodeActionFailed, "queued item cast ActionFailed")
	landSilenceMagicPhysical(t, srv, objID)
	srv.Settle(t)

	if got := nextSwingTarget(t, c, objID, "after the silence"); got != first {
		t.Fatalf("next swing target = %d, want %d", got, first)
	}
	if srv.PlayerCastingNow(t, objID) {
		t.Fatal("the item cast the silence dropped started")
	}
}
