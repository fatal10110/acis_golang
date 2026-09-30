package skills

import (
	"testing"
	"time"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/player"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/item"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/itemcontainer"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
)

// Reference for the tests below (#2877): with nothing in flight,
// PlayableAI.tryToUseItem (PlayableAI.java:466-481) runs doUseItemIntention
// (AbstractAI.java:278-286); prepareIntention (PlayableAI.java:29-38)
// copies the current intention to _previousIntention, and
// PlayerAI.thinkUseItem (PlayerAI.java:505-519) toggles the item, then runs
// doIntention(_previousIntention) unless it was CAST or USE_ITEM, in which
// case USE_ITEM stays current and every later THINK toggles the item again:
// the arrival (CreatureAI.onEvtArrived, CreatureAI.java:55-69) and the
// ImmobileUntilAttacked exit (EffectImmobileUntilAttacked.java:41,53), the
// one effect exit that thinks a player.

// intentionCastSkillID is a self cast with a half-second hit time, used to
// hold a cast open, and a ground cast of the same id walks to its signet.
const intentionCastSkillID = 5

// bootIntentionCaster boots a caster knowing intentionCastSkillID, targeted
// as target says, holding a one-handed sword, in the world.
func bootIntentionCaster(t *testing.T, target modelskill.Target) (*gameservertest.Server, int32, int32) {
	t.Helper()
	srv := gameservertest.Boot(t,
		gameservertest.WithCharacter("Newbie", 5, 0),
		gameservertest.WithWantChars(1),
		gameservertest.WithSkills(skillPersistence(t,
			[]modelskill.Definition{
				{
					ID: intentionCastSkillID, Level: 1, Activation: modelskill.ActivationActive, Target: target,
					CastRange: 100, HitTime: 500, ReuseDelay: 60_000, StaticHitTime: true, StaticReuse: true,
					MPInitialConsume: 2, MPConsume: 3, SkillType: "BUFF",
					Effects: []modelskill.EffectTemplate{{Name: "Buff", Time: 60, Icon: true}},
				},
			},
		)),
	)
	objID := srv.SoleObjectID(t)
	seedKnownSkill(t, srv, objID, intentionCastSkillID, 1)
	sword := srv.GiveItem(t, objID, approachSwordID, 1)
	startInWorld(t, srv.Client)
	return srv, objID, sword
}

// startGroundCastApproach casts intentionCastSkillID at a signet point out
// of range and returns where the approach walk ends.
func startGroundCastApproach(t *testing.T, srv *gameservertest.Server, objID int32) location.Location {
	t.Helper()
	x, y, z := srv.PlayerPosition(t, objID)
	srv.Client.Send(encodeRequestExMagicSkillUseGround(int32(x+400), int32(y), int32(z), intentionCastSkillID, false, false))
	walk := srv.Client.Read()
	assertFrameOpcode(t, walk, serverpackets.OpcodeMoveToLocation, "ground-cast approach walk")
	_, dest, _ := gameservertest.ReadMoveToLocationCoords(t, walk)
	return dest
}

func isOpcode(opcode byte) func([]byte) bool {
	return func(frame []byte) bool { return frame[0] == opcode }
}

// indexAfter returns the position of the first frame after from that match
// accepts, or -1.
func (l frameLog) indexAfter(from int, match func([]byte) bool) int {
	for i := from + 1; i < len(l); i++ {
		if match(l[i]) {
			return i
		}
	}
	return -1
}

func assertRightHand(t *testing.T, srv *gameservertest.Server, objID, want int32, what string) {
	t.Helper()
	worn := srv.PlayerInventory(t, objID).ItemAt(itemcontainer.RHand)
	switch {
	case want == 0 && worn != nil:
		t.Fatalf("right hand %s = %v, want empty", what, worn)
	case want != 0 && (worn == nil || worn.ObjectID != want):
		t.Fatalf("right hand %s = %v, want %d", what, worn, want)
	}
}

// TestWeaponUseItemMidWalkWalksOnAfresh: a sword put on mid-walk re-runs
// the MOVE_TO it replaced (thinkMoveTo, PlayableAI.java:186-196): after the
// equip, a fresh MoveToLocation from where the player stands toward the
// same destination. The arrival then ends the walk idle, with no second
// toggle.
func TestWeaponUseItemMidWalkWalksOnAfresh(t *testing.T) {
	t.Parallel()
	srv, objID, sword := bootIntentionCaster(t, modelskill.TargetSelf)
	c := srv.Client
	x, y, z := srv.PlayerPosition(t, objID)

	c.Send(encodeMoveBackwardToLocation(int32(x+400), int32(y), int32(z)))
	walk := c.Read()
	assertFrameOpcode(t, walk, serverpackets.OpcodeMoveToLocation, "walk")
	_, dest, _ := gameservertest.ReadMoveToLocationCoords(t, walk)
	if srv.DrivesClock() {
		srv.Advance(t, 500*time.Millisecond)
	}
	drainUntilQuiet(t, c)

	c.Send(encodeSignetUseItem(sword))
	srv.Settle(t)
	ax, ay, az := srv.PlayerPosition(t, objID)
	log := readFrameLog(c)
	equipped := log.index(isSystemMessage(serverpackets.SystemMessageS1Equipped))
	if equipped < 0 {
		t.Fatal("no S1_EQUIPPED for the mid-walk sword")
	}
	if at := log.index(isOpcode(serverpackets.OpcodeMoveToLocation)); at >= 0 && at < equipped {
		t.Fatalf("MoveToLocation at frame %d ahead of S1_EQUIPPED at %d", at, equipped)
	}
	rewalk := log.indexAfter(equipped, isOpcode(serverpackets.OpcodeMoveToLocation))
	if rewalk < 0 {
		t.Fatal("no fresh MoveToLocation after the mid-walk toggle")
	}
	objectID, gotDest, origin := gameservertest.ReadMoveToLocationCoords(t, log[rewalk])
	if objectID != objID || gotDest != dest {
		t.Fatalf("fresh MoveToLocation object/dest = %d/%+v, want %d/%+v", objectID, gotDest, objID, dest)
	}
	if srv.DrivesClock() {
		if want := (location.Location{X: ax, Y: ay, Z: az}); origin != want {
			t.Fatalf("fresh MoveToLocation origin = %+v, want the current position %+v", origin, want)
		}
	}
	if at := log.index(isOpcode(serverpackets.OpcodeActionFailed)); at >= 0 {
		t.Fatalf("ActionFailed at frame %d: a re-run MOVE_TO answers none", at)
	}

	waitForPlayerPosition(t, srv, objID, dest)
	if at := readFrameLog(c).index(isSystemMessage(serverpackets.SystemMessageS1Disarmed)); at >= 0 {
		t.Fatalf("S1_DISARMED at frame %d: the MOVE_TO arrival toggled the sword again", at)
	}
	assertRightHand(t, srv, objID, sword, "after the arrival")
}

// TestWeaponUseItemMidPickupWalkWalksOnAfresh: a sword put on while walking
// to a ground item re-runs the PICK_UP it replaced (thinkPickUp,
// PlayableAI.java:199-234): ActionFailed, then a fresh MoveToLocation to
// the item, and the arrival collects it.
func TestWeaponUseItemMidPickupWalkWalksOnAfresh(t *testing.T) {
	t.Parallel()
	srv, objID, sword := bootIntentionCaster(t, modelskill.TargetSelf)
	c := srv.Client
	x, y, z := srv.PlayerPosition(t, objID)
	srv.SeedGroundItem(t, objID, item.AdenaID, 40, x+400, y, z)
	snaps := srv.GroundItems.Snapshots(nil)
	if len(snaps) != 1 {
		t.Fatalf("tracked ground items = %d, want 1", len(snaps))
	}
	drainUntilQuiet(t, c)

	c.Send(encodeAction(snaps[0].ObjectID, int32(x), int32(y), int32(z), false))
	var dest location.Location
	for frame := c.Read(); ; frame = c.Read() {
		if frame[0] == serverpackets.OpcodeMoveToLocation {
			_, dest, _ = gameservertest.ReadMoveToLocationCoords(t, frame)
			break
		}
	}
	if srv.DrivesClock() {
		srv.Advance(t, 500*time.Millisecond)
	}
	drainUntilQuiet(t, c)

	c.Send(encodeSignetUseItem(sword))
	srv.Settle(t)
	log := readFrameLog(c)
	equipped := log.index(isSystemMessage(serverpackets.SystemMessageS1Equipped))
	if equipped < 0 {
		t.Fatal("no S1_EQUIPPED for the mid-walk sword")
	}
	released := log.indexAfter(equipped, isOpcode(serverpackets.OpcodeActionFailed))
	if released < 0 {
		t.Fatal("no ActionFailed from the re-run pickup after the toggle")
	}
	rewalk := log.indexAfter(released, isOpcode(serverpackets.OpcodeMoveToLocation))
	if rewalk < 0 {
		t.Fatal("no fresh MoveToLocation to the item after the pickup's ActionFailed")
	}
	if _, gotDest, _ := gameservertest.ReadMoveToLocationCoords(t, log[rewalk]); gotDest != dest {
		t.Fatalf("fresh pickup walk dest = %+v, want %+v", gotDest, dest)
	}

	srv.AdvanceUntil(t, "pickup after the fresh walk", func() bool {
		return len(srv.GroundItems.Snapshots(nil)) == 0
	})
	assertRightHand(t, srv, objID, sword, "after the pickup")
}

// TestWeaponUseItemLeftCurrentRetogglesOnThink: a sword put on mid-way into
// a ground-cast approach leaves USE_ITEM current; the ImmobileUntilAttacked
// exit THINK takes it off again, the walk goes on, and the arrival puts it
// back on, casting nothing.
func TestWeaponUseItemLeftCurrentRetogglesOnThink(t *testing.T) {
	t.Parallel()
	srv, objID, sword := bootIntentionCaster(t, modelskill.TargetGround)
	c := srv.Client
	dest := startGroundCastApproach(t, srv, objID)

	c.Send(encodeSignetUseItem(sword))
	srv.Settle(t)
	assertRightHand(t, srv, objID, sword, "after the mid-walk UseItem")
	drainUntilQuiet(t, c)

	onPlayerQueue(t, srv, objID, func(pc *player.Character) { pc.WakeAI() })
	srv.Settle(t)
	if at := readFrameLog(c).index(isSystemMessage(serverpackets.SystemMessageS1Disarmed)); at < 0 {
		t.Fatal("no S1_DISARMED: the THINK did not toggle the still-current sword")
	}
	assertRightHand(t, srv, objID, 0, "after the THINK")

	waitForPlayerPosition(t, srv, objID, dest)
	log := readFrameLog(c)
	if at := log.index(isSystemMessage(serverpackets.SystemMessageS1Equipped)); at < 0 {
		t.Fatal("no S1_EQUIPPED: the arrival did not toggle the still-current sword")
	}
	if at := log.index(isOpcode(serverpackets.OpcodeMagicSkillUse)); at >= 0 {
		t.Fatalf("MagicSkillUse at frame %d after the toggle replaced the cast approach", at)
	}
	assertRightHand(t, srv, objID, sword, "after the arrival")
}

// TestWeaponUseItemReplacedStopsRetoggling: a walk requested after the
// toggle replaces the still-current USE_ITEM, so neither its arrival nor a
// later THINK toggles the sword again.
func TestWeaponUseItemReplacedStopsRetoggling(t *testing.T) {
	t.Parallel()
	srv, objID, sword := bootIntentionCaster(t, modelskill.TargetGround)
	c := srv.Client
	startGroundCastApproach(t, srv, objID)

	c.Send(encodeSignetUseItem(sword))
	srv.Settle(t)
	drainUntilQuiet(t, c)
	x, y, z := srv.PlayerPosition(t, objID)
	c.Send(encodeMoveBackwardToLocation(int32(x), int32(y+300), int32(z)))
	walk := c.Read()
	for walk[0] != serverpackets.OpcodeMoveToLocation {
		walk = c.Read()
	}
	_, dest, _ := gameservertest.ReadMoveToLocationCoords(t, walk)

	waitForPlayerPosition(t, srv, objID, dest)
	onPlayerQueue(t, srv, objID, func(pc *player.Character) { pc.WakeAI() })
	srv.Settle(t)
	log := readFrameLog(c)
	if at := log.index(isSystemMessage(serverpackets.SystemMessageS1Disarmed)); at >= 0 {
		t.Fatalf("S1_DISARMED at frame %d after a walk replaced the toggle", at)
	}
	if at := log.index(isOpcode(serverpackets.OpcodeMagicSkillUse)); at >= 0 {
		t.Fatalf("MagicSkillUse at frame %d after the walk", at)
	}
	assertRightHand(t, srv, objID, sword, "after the replacing walk and THINK")
}

// TestWeaponUseItemAfterCastStaysCurrent: a toggle queued behind a cast
// runs when the cast ends (onEvtFinishedCasting, PlayableAI.java:43-63);
// its previous intention is that CAST, so USE_ITEM stays current and a
// later THINK takes the sword off again.
func TestWeaponUseItemAfterCastStaysCurrent(t *testing.T) {
	t.Parallel()
	srv, objID, sword := bootIntentionCaster(t, modelskill.TargetSelf)
	if !srv.DrivesClock() {
		t.Skip("holding a cast open needs the driven clock")
	}
	c := srv.Client

	c.Send(encodeRequestMagicSkillUse(intentionCastSkillID, false, false))
	for frame := c.Read(); frame[0] != serverpackets.OpcodeMagicSkillUse; frame = c.Read() {
	}
	c.Send(encodeSignetUseItem(sword))
	srv.Settle(t)
	assertRightHand(t, srv, objID, 0, "mid-cast")

	srv.AdvanceUntil(t, "queued sword after the cast", func() bool {
		worn := srv.PlayerInventory(t, objID).ItemAt(itemcontainer.RHand)
		return worn != nil && worn.ObjectID == sword
	})
	srv.Settle(t)
	drainUntilQuiet(t, c)

	onPlayerQueue(t, srv, objID, func(pc *player.Character) { pc.WakeAI() })
	srv.Settle(t)
	if at := readFrameLog(c).index(isSystemMessage(serverpackets.SystemMessageS1Disarmed)); at < 0 {
		t.Fatal("no S1_DISARMED: the THINK did not toggle the sword left current after the cast")
	}
	assertRightHand(t, srv, objID, 0, "after the THINK")
}

// TestThinkMidWalkWalksOnAfresh: a THINK while MOVE_TO is current runs
// thinkMoveTo (PlayableAI.java:186-196) again, a fresh MoveToLocation toward
// the same destination; after the arrival (CreatureAI.java:55-69, MOVE_TO
// goes idle) a THINK walks nowhere.
func TestThinkMidWalkWalksOnAfresh(t *testing.T) {
	t.Parallel()
	srv, objID, _ := bootIntentionCaster(t, modelskill.TargetSelf)
	c := srv.Client
	x, y, z := srv.PlayerPosition(t, objID)

	c.Send(encodeMoveBackwardToLocation(int32(x+400), int32(y), int32(z)))
	walk := c.Read()
	assertFrameOpcode(t, walk, serverpackets.OpcodeMoveToLocation, "walk")
	_, dest, _ := gameservertest.ReadMoveToLocationCoords(t, walk)
	drainUntilQuiet(t, c)

	onPlayerQueue(t, srv, objID, func(pc *player.Character) { pc.WakeAI() })
	srv.Settle(t)
	log := readFrameLog(c)
	rewalk := log.index(isOpcode(serverpackets.OpcodeMoveToLocation))
	if rewalk < 0 {
		t.Fatal("no fresh MoveToLocation for the THINK mid-walk")
	}
	if _, gotDest, _ := gameservertest.ReadMoveToLocationCoords(t, log[rewalk]); gotDest != dest {
		t.Fatalf("fresh MoveToLocation dest = %+v, want %+v", gotDest, dest)
	}

	waitForPlayerPosition(t, srv, objID, dest)
	drainUntilQuiet(t, c)
	onPlayerQueue(t, srv, objID, func(pc *player.Character) { pc.WakeAI() })
	srv.Settle(t)
	if at := readFrameLog(c).index(isOpcode(serverpackets.OpcodeMoveToLocation)); at >= 0 {
		t.Fatalf("MoveToLocation at frame %d for a THINK after the walk arrived", at)
	}
}
