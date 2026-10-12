package combat

import (
	"testing"
	"time"

	"github.com/fatal10110/acis_golang/internal/commons/wire"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/attackable"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/clientpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
)

// The emote requests below pin RequestSocialAction.java:38: the emote is
// broadcast only while the requester's current intention is IDLE, whatever
// its combat stance or posture, and every other rejection is silent.

const socialIdleCastSkill = 3

func encodeRequestSocialAction(actionID int32) []byte {
	w := wire.NewPacketWriter(clientpackets.OpcodeRequestSocialAction)
	w.WriteInt32(actionID)
	return w.Bytes()
}

// assertEmoteShown reads the SocialAction broadcasting objID's emote.
func assertEmoteShown(t *testing.T, c *scriptedClient, objID, actionID int32, what string) {
	t.Helper()
	frame := mustRead(t, c, what)
	assertFrameOpcode(t, frame, serverpackets.OpcodeSocialAction, what)
	r := wireReader(frame[1:])
	if gotActor, gotAction := r.ReadInt32(), r.ReadInt32(); gotActor != objID || gotAction != actionID {
		t.Fatalf("%s = actor %d action %d, want actor %d action %d", what, gotActor, gotAction, objID, actionID)
	}
}

// assertEmoteDropped sends an emote and checks it answers nothing: no
// SocialAction, ActionFailed or system message reaches the requester, which
// every broadcast of its own emote includes.
func assertEmoteDropped(t *testing.T, c *scriptedClient, what string) {
	t.Helper()
	c.Send(encodeRequestSocialAction(2))
	for _, frame := range readQuiet(c) {
		switch frame[0] {
		case serverpackets.OpcodeSocialAction, serverpackets.OpcodeActionFailed, serverpackets.OpcodeSystemMessage:
			t.Fatalf("%s answered opcode %#x, want nothing", what, frame[0])
		}
	}
}

// bootSocialIdle boots one level 5 character, runs seed on its object id
// before it enters the world, then enters.
func bootSocialIdle(t *testing.T, seed func(*gameservertest.Server, int32), opts ...gameservertest.Option) (*gameservertest.Server, *scriptedClient, int32) {
	t.Helper()
	srv := gameservertest.Boot(t, append([]gameservertest.Option{
		gameservertest.WithCharacter("Newbie", 5, 0),
		gameservertest.WithWantChars(1),
	}, opts...)...)
	c, objID := srv.Client, srv.SoleObjectID(t)
	if seed != nil {
		seed(srv, objID)
	}
	startInWorld(t, c)
	return srv, c, objID
}

func TestEmoteDroppedWhileWalkingToPointThenShownOnArrival(t *testing.T) {
	t.Parallel()
	srv, c, objID := bootSocialIdle(t, nil)

	c.Send(encodeMoveBackwardToLocation(int32(playerOrigin.X)+100, int32(playerOrigin.Y), int32(playerOrigin.Z)))
	readUntil(t, c, serverpackets.OpcodeMoveToLocation, "walk")
	drainUntilQuiet(t, c)
	assertEmoteDropped(t, c, "emote while walking to a point")

	srv.Advance(t, 5*time.Second)
	drainUntilQuiet(t, c)
	c.Send(encodeRequestSocialAction(3))
	assertEmoteShown(t, c, objID, 3, "emote after the walk arrived")
}

func TestEmoteShownInAttackStanceWhileIdle(t *testing.T) {
	t.Parallel()
	srv, c, objID := bootSocialIdle(t, nil, gameservertest.WithAttackStanceClock(time.Now))
	attacker := srv.SpawnAttackingHostileNPCAt(t, location.Location{X: hostileX, Y: hostileY, Z: hostileZ})
	drainUntilQuiet(t, c)

	attacker.DoAttack(t, livePlayer(t, srv, objID).(attackable.Combatant))
	drainUntilQuiet(t, c)
	if !srv.AttackStance.InAttackStance(worldActor{id: objID}) {
		t.Fatal("monster hit left the player out of the attack stance")
	}

	c.Send(encodeRequestSocialAction(4))
	assertEmoteShown(t, c, objID, 4, "idle emote inside the attack stance")
}

func TestEmoteDroppedWhileAttacking(t *testing.T) {
	t.Parallel()
	srv, c, objID := bootSocialIdle(t, nil)
	hostile := srv.SpawnHostileNPCAt(t, location.Location{X: hostileX - 20, Y: hostileY, Z: hostileZ})
	drainUntilQuiet(t, c)

	targetHostile(t, c, hostile.ObjectID())
	c.Send(encodeAction(hostile.ObjectID(), int32(playerOrigin.X), int32(playerOrigin.Y), int32(playerOrigin.Z), false))
	assertAttackBy(t, c, objID)
	srv.AdvanceUntil(t, "opening swing", func() bool { return hostile.CurrentHP() < hostile.MaxHP() })
	awaitAutoAttackStart(t, c, objID)
	drainUntilQuiet(t, c)

	assertEmoteDropped(t, c, "emote while attacking")
}

func TestEmoteDroppedWhileCasting(t *testing.T) {
	t.Parallel()
	seed := func(srv *gameservertest.Server, objID int32) { seedKnownSkill(t, srv, objID, socialIdleCastSkill, 1) }
	srv, c, objID := bootSocialIdle(t, seed, gameservertest.WithSkills(combatPersistence(t,
		[]modelskill.Definition{{
			ID: socialIdleCastSkill, Level: 1, Activation: modelskill.ActivationActive, Target: modelskill.TargetSelf,
			HitTime: 4000, ReuseDelay: 60_000, StaticHitTime: true, StaticReuse: true, SkillType: "DUMMY",
		}},
	)))

	c.Send(encodeRequestMagicSkillUse(socialIdleCastSkill, false, false))
	drainUntilQuiet(t, c)
	if !srv.PlayerCastingNow(t, objID) {
		t.Fatal("cast was not in flight")
	}
	assertEmoteDropped(t, c, "emote while casting")
}

func TestEmoteDroppedWhileSittingDownThenShownSeated(t *testing.T) {
	t.Parallel()
	srv, c, objID := bootSocialIdle(t, nil)

	c.Send(encodeRequestChangeWaitType(false))
	assertFrameOpcode(t, mustRead(t, c, "sit"), serverpackets.OpcodeChangeWaitType, "sit")
	assertEmoteDropped(t, c, "emote while sitting down")

	// The finished sit-down leaves the player idle, seated
	// (PlayerAI.onEvtSatDown), so the emote goes through.
	srv.SettlePosture(t, objID)
	drainUntilQuiet(t, c)
	c.Send(encodeRequestSocialAction(5))
	assertEmoteShown(t, c, objID, 5, "emote once seated")
}

func TestEmoteWhileFishingAnswersCannotDoWhileFishing(t *testing.T) {
	t.Parallel()
	srv, c, objID := bootSocialIdle(t, nil)
	srv.SetPlayerFishing(t, objID, true)

	c.Send(encodeRequestSocialAction(2))
	frame := mustRead(t, c, "fishing emote reply")
	assertFrameOpcode(t, frame, serverpackets.OpcodeSystemMessage, "fishing emote reply")
	if got := wireReader(frame[1:]).ReadInt32(); got != serverpackets.SystemMessageCannotDoWhileFishing {
		t.Fatalf("fishing emote system message = %d, want CANNOT_DO_WHILE_FISHING_3 (%d)", got, serverpackets.SystemMessageCannotDoWhileFishing)
	}
	for _, f := range readQuiet(c) {
		if f[0] == serverpackets.OpcodeSocialAction {
			t.Fatal("fishing emote was broadcast")
		}
	}
}
