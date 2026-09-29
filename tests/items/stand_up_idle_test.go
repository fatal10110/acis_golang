package items

import (
	"testing"
	"time"

	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/sim"
	"github.com/fatal10110/acis_golang/internal/gameserver/skill/effect"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
	"github.com/fatal10110/acis_golang/internal/testsupport"
)

// punchOfDoomID is the skill whose StunSelf effect stuns its own caster.
const punchOfDoomID = 81

// stunSelfHolder is the live player surface a StunSelf effect lands on.
type stunSelfHolder interface {
	effect.Actor
	EffectList() *effect.List
	Queue() *sim.Queue
}

// landStunSelfOn applies a real StunSelf effect to objID's live player on
// its own queue and waits for its start hook to finish.
func landStunSelfOn(t *testing.T, srv *gameservertest.Server, objID int32) {
	t.Helper()
	obj, ok := srv.State.Player(objID)
	if !ok {
		t.Fatalf("world.Player(%d) missing", objID)
	}
	target, ok := obj.(stunSelfHolder)
	if !ok {
		t.Fatalf("world.Player(%d) = %T is not an effect holder", objID, obj)
	}
	e, err := effect.New(effect.Skill{ID: punchOfDoomID, Level: 1}, modelskill.EffectTemplate{Name: "StunSelf", Time: 9})
	if err != nil {
		t.Fatalf("effect.New(StunSelf): %v", err)
	}
	e.Effector, e.Effected = target, target
	done := make(chan struct{})
	if !target.Queue().Post(func() { target.EffectList().Add(e); close(done) }) {
		t.Fatal("post StunSelf: queue closed")
	}
	<-done
}

// opcodesUntilQuiet reads every frame c receives until it goes quiet and
// counts them by opcode.
func opcodesUntilQuiet(t *testing.T, c *testsupport.ScriptedClient) map[byte]int {
	t.Helper()
	seen := map[byte]int{}
	for i := 0; i < 100; i++ {
		frame := c.ReadWithTimeout(300 * time.Millisecond)
		if frame == nil {
			return seen
		}
		seen[frame[0]]++
	}
	t.Fatal("client kept receiving frames after 100 reads")
	return nil
}

// TestStunSelfDuringStandUpWaitsOutTheTransition pins PlayableAI.tryToIdle's
// isStandingNow() term for a player: an idle that lands while the player is
// standing up is answered ActionFailed and replaces the item cast queued
// behind the stand-up, which never starts when the stand-up ends.
func TestStunSelfDuringStandUpWaitsOutTheTransition(t *testing.T) {
	t.Parallel()
	srv := gameservertest.Boot(t,
		gameservertest.WithSkills(consumableSkills(t)),
		gameservertest.WithCharacter("Newbie", 5, 0),
		gameservertest.WithWantChars(1))
	c, objID := srv.Client, srv.SoleObjectID(t)
	scroll := srv.GiveItem(t, objID, escapeScrollID, 3)
	startInWorld(t, c)
	sitAndSettle(t, srv)

	changePosture(t, c, true)
	c.Send(encodeUseItem(scroll, false))
	assertFrameOpcode(t, c.Read(), serverpackets.OpcodeActionFailed, "queued item cast")

	landStunSelfOn(t, srv, objID)
	seen := opcodesUntilQuiet(t, c)
	if n := seen[serverpackets.OpcodeActionFailed]; n != 1 {
		t.Fatalf("ActionFailed frames after StunSelf during the stand-up = %d, want 1", n)
	}
	if n := seen[serverpackets.OpcodeMagicSkillUse]; n != 0 {
		t.Fatalf("MagicSkillUse frames after StunSelf during the stand-up = %d, want 0", n)
	}

	srv.Advance(t, sitStandDelay)
	if n := opcodesUntilQuiet(t, c)[serverpackets.OpcodeMagicSkillUse]; n != 0 {
		t.Fatalf("MagicSkillUse frames at the stand-up's end = %d, want 0: the idle must replace the queued cast", n)
	}
	if srv.PlayerCastingNow(t, objID) {
		t.Fatal("queued item cast started at the stand-up's end: the idle must replace it")
	}
	assertItemCount(t, srv, objID, scroll, 3)
}
