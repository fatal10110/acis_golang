package combat

import (
	"testing"
	"time"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/cast"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/player"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/network"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
)

const (
	midSwingSkillID  = 3
	midSwingToggleID = 288
)

func midSwingSkill() modelskill.Definition {
	return modelskill.Definition{
		ID: midSwingSkillID, Level: 1, Activation: modelskill.ActivationActive, Target: modelskill.TargetSelf,
		HitTime: 500, ReuseDelay: 60_000, StaticHitTime: true, StaticReuse: true, SkillType: "DUMMY",
	}
}

func midSwingToggle() modelskill.Definition {
	return modelskill.Definition{
		ID: midSwingToggleID, Level: 1, Activation: modelskill.ActivationToggle, Target: modelskill.TargetSelf,
		MPConsume: 12, SkillType: "BUFF",
		Effects: []modelskill.EffectTemplate{{Name: "Buff", Time: 60, Icon: true}},
	}
}

// bootMidSwing puts the caster in the middle of its opening swing against
// the fixture monster: the swing's Attack is out, and
// the driven clock holds the swing open until the test advances it.
func bootMidSwing(t *testing.T) (*gameservertest.Server, *player.Character) {
	t.Helper()
	srv := gameservertest.Boot(t,
		gameservertest.WithCharacter("Newbie", 5, 0),
		gameservertest.WithWantChars(1),
		gameservertest.WithSkills(combatPersistence(t, []modelskill.Definition{midSwingSkill(), midSwingToggle()})),
	)
	if !srv.DrivesClock() {
		t.Skip("holding a swing open needs the driven clock")
	}
	c, objID := srv.Client, srv.SoleObjectID(t)
	seedKnownSkill(t, srv, objID, midSwingSkillID, 1)
	seedKnownSkill(t, srv, objID, midSwingToggleID, 1)
	startInWorld(t, c)
	hostile := srv.SpawnHostileNPCAt(t, location.Location{X: hostileX - 20, Y: hostileY, Z: hostileZ})
	drainUntilQuiet(t, c)

	obj, ok := srv.State.Player(objID)
	if !ok {
		t.Fatal("player missing from world state")
	}
	pc, ok := network.OnlineCharacter(obj)
	if !ok {
		t.Fatalf("player %T is not an online character", obj)
	}

	targetHostile(t, c, hostile.ObjectID())
	c.Send(encodeAction(hostile.ObjectID(), int32(playerOrigin.X), int32(playerOrigin.Y), int32(playerOrigin.Z), false))
	assertAttackBy(t, c, objID)
	return srv, pc
}

// TestMidSwingCastOnCooldownIsRejectedNotQueued pins the pre-attempt gate
// running ahead of the swing queue: a skill still on cooldown, requested
// mid-swing, answers at once with S1_PREPARED_FOR_REUSE and ActionFailed,
// and is not replayed when the swing ends.
func TestMidSwingCastOnCooldownIsRejectedNotQueued(t *testing.T) {
	t.Parallel()
	srv, pc := bootMidSwing(t)
	c, objID := srv.Client, srv.SoleObjectID(t)
	// A short cooldown: a queued replay at swing end would find it expired
	// and start the cast.
	pc.DisableSkill(cast.ReuseKey(midSwingSkill()), 50*time.Millisecond)

	c.Send(encodeRequestMagicSkillUse(midSwingSkillID, false, false))
	assertSystemMessageSkillFrame(t, mustRead(t, c, "reuse rejection"), serverpackets.SystemMessageS1PreparedForReuse, midSwingSkillID, 1)
	assertFrameOpcode(t, mustRead(t, c, "reuse ActionFailed"), serverpackets.OpcodeActionFailed, "reuse ActionFailed")

	for passed := time.Duration(0); passed < 2*time.Second; passed += 10 * time.Millisecond {
		srv.Advance(t, 10*time.Millisecond)
		if srv.PlayerCastingNow(t, objID) {
			t.Fatalf("rejected mid-swing cast started %v later: it was queued", passed)
		}
	}
}

// TestMidSwingCastableRequestWaitsForTheSwing pins the other half: a request
// that passes the gate mid-swing is queued with ActionFailed and starts once
// the swing is over.
func TestMidSwingCastableRequestWaitsForTheSwing(t *testing.T) {
	t.Parallel()
	srv, _ := bootMidSwing(t)
	c, objID := srv.Client, srv.SoleObjectID(t)

	c.Send(encodeRequestMagicSkillUse(midSwingSkillID, false, false))
	assertFrameOpcode(t, mustRead(t, c, "queued ActionFailed"), serverpackets.OpcodeActionFailed, "queued ActionFailed")
	if srv.PlayerCastingNow(t, objID) {
		t.Fatal("queued cast started before the swing ended")
	}
	srv.AdvanceUntil(t, "queued cast start", func() bool { return srv.PlayerCastingNow(t, objID) })
}

func toggleOn(pc *player.Character) bool {
	_, on := pc.EffectList().ActiveBySkillID(midSwingToggleID)
	return on
}

// TestMidSwingToggleWaitsForTheSwing pins toggles sharing the swing queue: a
// toggle requested mid-swing is answered with ActionFailed, sends no
// MagicSkillUse and applies nothing until the swing ends, then turns on.
func TestMidSwingToggleWaitsForTheSwing(t *testing.T) {
	t.Parallel()
	srv, pc := bootMidSwing(t)
	c := srv.Client

	c.Send(encodeRequestMagicSkillUse(midSwingToggleID, false, false))
	assertFrameOpcode(t, mustRead(t, c, "queued toggle ActionFailed"), serverpackets.OpcodeActionFailed, "queued toggle ActionFailed")
	if toggleOn(pc) {
		t.Fatal("toggle turned on mid-swing")
	}
	srv.AdvanceUntil(t, "queued toggle on", func() bool { return toggleOn(pc) })
}

// TestMidSwingToggleOnReuseIsRefusedNotQueued pins the pre-attempt gate
// answering a toggle ahead of the swing queue: one still on reuse answers
// S1_PREPARED_FOR_REUSE then ActionFailed and never turns on, even with its
// reuse over once the swing ends.
func TestMidSwingToggleOnReuseIsRefusedNotQueued(t *testing.T) {
	t.Parallel()
	srv, pc := bootMidSwing(t)
	c := srv.Client
	pc.DisableSkill(cast.ReuseKey(midSwingToggle()), 50*time.Millisecond)

	c.Send(encodeRequestMagicSkillUse(midSwingToggleID, false, false))
	assertSystemMessageSkillFrame(t, mustRead(t, c, "reuse rejection"), serverpackets.SystemMessageS1PreparedForReuse, midSwingToggleID, 1)
	assertFrameOpcode(t, mustRead(t, c, "reuse ActionFailed"), serverpackets.OpcodeActionFailed, "reuse ActionFailed")

	for passed := time.Duration(0); passed < 2*time.Second; passed += 10 * time.Millisecond {
		srv.Advance(t, 10*time.Millisecond)
		if toggleOn(pc) {
			t.Fatalf("toggle refused mid-swing turned on %v later: it was queued", passed)
		}
	}
}
