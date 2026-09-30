package skills

import (
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/itemcontainer"
	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
)

// approachSwordID is the shared catalog's one-handed sword.
const approachSwordID int32 = 30

// TestWeaponUseItemDropsGroundCastApproach walks a caster toward a signet
// point out of cast range, then toggles a sword on mid-walk (#2815).
//
// Reference: UseItem.java:145-155 hands the weapon to
// PlayableAI.tryToUseItem (PlayableAI.java:466-481); with no swing, cast or
// posture change in flight it runs doUseItemIntention (AbstractAI.java:
// 278-286), whose prepareIntention (PlayableAI.java:29-38) makes the CAST
// the previous intention and cancels only a follow task, so the walk goes
// on. PlayerAI.thinkUseItem (PlayerAI.java:505-519) toggles the sword and
// does not re-issue a previous CAST, so USE_ITEM stays current and the
// arrival (CreatureAI.onEvtArrived, CreatureAI.java:55-69) never casts.
// The reference's arrival THINK also toggles the sword a second time
// (thinkUseItem on the still-current USE_ITEM); that re-toggle is tracked
// by #2877, so the sword stays on here.
//
// The control walks the same way without the toggle and casts on arrival.
func TestWeaponUseItemDropsGroundCastApproach(t *testing.T) {
	t.Parallel()
	const skillID = 5
	for _, tc := range []struct {
		name   string
		toggle bool
	}{
		{name: "toggle drops the cast", toggle: true},
		{name: "control casts on arrival", toggle: false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			srv := gameservertest.Boot(t,
				gameservertest.WithCharacter("Newbie", 5, 0),
				gameservertest.WithWantChars(1),
				gameservertest.WithSkills(skillPersistence(t,
					[]modelskill.Definition{
						{
							ID: skillID, Level: 1, Activation: modelskill.ActivationActive, Target: modelskill.TargetGround,
							CastRange: 100, HitTime: 500, ReuseDelay: 60_000, StaticHitTime: true, StaticReuse: true,
							MPInitialConsume: 2, MPConsume: 3, SkillType: "BUFF",
							Effects: []modelskill.EffectTemplate{{Name: "Buff", Time: 60, Icon: true}},
						},
					},
				)),
			)
			c, objID := srv.Client, srv.SoleObjectID(t)
			seedKnownSkill(t, srv, objID, skillID, 1)
			sword := srv.GiveItem(t, objID, approachSwordID, 1)
			startInWorld(t, c)
			x, y, z := srv.PlayerPosition(t, objID)
			mpBefore := srv.PlayerCurrentMP(t, objID)

			c.Send(encodeRequestExMagicSkillUseGround(int32(x+400), int32(y), int32(z), skillID, false, false))
			walk := c.Read()
			assertFrameOpcode(t, walk, serverpackets.OpcodeMoveToLocation, "ground-cast approach walk")
			_, dest, _ := gameservertest.ReadMoveToLocationCoords(t, walk)

			if tc.toggle {
				c.Send(encodeSignetUseItem(sword))
				srv.Settle(t)
				if worn := srv.PlayerInventory(t, objID).ItemAt(itemcontainer.RHand); worn == nil || worn.ObjectID != sword {
					t.Fatalf("right hand after the mid-walk UseItem = %v, want the sword", worn)
				}
			}
			waitForPlayerPosition(t, srv, objID, dest)
			log := readFrameLog(c)

			cast := log.index(func(frame []byte) bool { return frame[0] == serverpackets.OpcodeMagicSkillUse })
			if !tc.toggle {
				if cast < 0 {
					t.Fatal("the walk arrived without casting; the control needs the approach to cast")
				}
				return
			}
			if cast >= 0 {
				t.Fatalf("MagicSkillUse at frame %d after the toggle replaced the cast approach", cast)
			}
			if at := log.index(isSystemMessage(serverpackets.SystemMessageS1Equipped)); at < 0 {
				t.Fatal("no S1_EQUIPPED for the mid-walk sword")
			}
			if at := log.index(isSystemMessage(serverpackets.SystemMessageDistTooFarCastingStopped)); at >= 0 {
				t.Fatalf("DIST_TOO_FAR_CASTING_STOPPED at frame %d with no cast intention left", at)
			}
			if srv.PlayerCastingNow(t, objID) {
				t.Fatal("casting after the arrival")
			}
			if got := srv.PlayerCurrentMP(t, objID); got != mpBefore {
				t.Fatalf("MP after the arrival = %d, want %d untouched", got, mpBefore)
			}
			if worn := srv.PlayerInventory(t, objID).ItemAt(itemcontainer.RHand); worn == nil || worn.ObjectID != sword {
				t.Fatalf("right hand after the arrival = %v, want the sword", worn)
			}
		})
	}
}
