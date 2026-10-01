package skills

import (
	"testing"
	"time"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/player"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/skill/effect"
)

// TestLaterGetUpEndsRecastFakeDeath pins the reference's uncancelled
// get-up tasks against a Fake Death cast again in between:
//   - each Player.stopFakeDeath schedules its own get-up task, which clears
//     _isFakeDeath when it runs (Player.java:7035-7056), so two restart
//     requests during fake death leave two get-ups running
//     (RequestRestartPoint.java:38-42);
//   - a Fake Death cast during the get-up is queued until the first
//     STOOD_UP (PlayableAI.java:313, PlayerAI.onEvtStoodUp) and lies the
//     player down again with the effect on;
//   - the second get-up then clears _isFakeDeath under that effect;
//     isFakeDeath() is the flag alone (Player.java:2141-2144);
//   - so, seated with the effect on but not playing dead, a stand request
//     stands up (PlayerAI.thinkStand, Player.standUp: ChangeWaitType
//     STANDING) and keeps the toggle, and a private store request is not
//     refused as fake death (RequestActionUse.java:92) and opens
//     (Player.tryOpenPrivateSellStore).
func TestLaterGetUpEndsRecastFakeDeath(t *testing.T) {
	t.Parallel()
	srv, c, objID := bootPostureCaster(t)
	if !srv.DrivesClock() {
		t.Skip("needs the driven clock to start the second get-up halfway into the first")
	}
	lieDown, getUp := fakeDeathDelays(t, srv, objID)
	var pc *player.Character
	onPlayerQueue(t, srv, objID, func(p *player.Character) { pc = p })
	startFakeDeath(t, c)
	srv.Advance(t, lieDown)
	drainUntilQuiet(t, c)

	firstAt := c.Now()
	c.Send(encodeRequestRestartPoint(0))
	readMatching(t, c, time.Second, "first restart's get-up", isWaitType(serverpackets.WaitFakeDeathStop))
	drainUntilQuiet(t, c)
	srv.Advance(t, getUp/2-c.Now().Sub(firstAt))
	c.Send(encodeRequestRestartPoint(0))
	readMatching(t, c, time.Second, "second restart's get-up", isWaitType(serverpackets.WaitFakeDeathStop))
	drainUntilQuiet(t, c)

	c.Send(encodeRequestMagicSkillUse(fakeDeathSkillID, false, false))
	readMatching(t, c, getUp, "Fake Death cast again at the first get-up's end", isWaitType(serverpackets.WaitFakeDeathStart))
	srv.AdvanceUntil(t, "second get-up ended the new fake death", func() bool { return !pc.FakeDead() })
	if !pc.SittingNow() || !pc.EffectList().IsAffected(effect.FlagFakeDeath) {
		t.Fatalf("after the second get-up SittingNow=%v effect on=%v, want the new lie-down and effect going on",
			pc.SittingNow(), pc.EffectList().IsAffected(effect.FlagFakeDeath))
	}
	srv.AdvanceUntil(t, "new lie-down ended", func() bool { return !pc.SittingNow() })
	drainUntilQuiet(t, c)

	c.Send(encodeRequestChangeWaitType(true))
	readMatching(t, c, time.Second, "stand-up", func(f []byte) bool {
		if isWaitType(serverpackets.WaitFakeDeathStop)(f) {
			t.Fatal("stand request got up out of fake death; the player no longer plays dead")
		}
		return isWaitType(serverpackets.WaitStanding)(f)
	})
	if !pc.EffectList().IsAffected(effect.FlagFakeDeath) {
		t.Fatal("stand request ended the Fake Death toggle")
	}
	srv.Advance(t, sitStandDelay)
	drainUntilQuiet(t, c)

	c.Send(encodeRequestActionUse(10, false, false))
	readMatching(t, c, time.Second, "private store sell list", func(f []byte) bool {
		if f[0] == serverpackets.OpcodeActionFailed {
			t.Fatal("private store request refused as fake death")
		}
		return f[0] == serverpackets.OpcodePrivateStoreManageListSell
	})
}
