package skills

import (
	"testing"
	"time"

	"github.com/fatal10110/acis_golang/internal/commons/wire"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/cast"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/player"
	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/network"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/clientpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/skill/effect"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
)

const (
	gateActiveSkillID int32 = 3
	gateToggleSkillID int32 = 288
	gateSiegeSkillID  int32 = 13
)

func gateSkills() []modelskill.Definition {
	return []modelskill.Definition{
		{
			ID: modelskill.ID(gateActiveSkillID), Level: 1, Activation: modelskill.ActivationActive, Target: modelskill.TargetSelf,
			HitTime: 500, ReuseDelay: 60_000, StaticHitTime: true, StaticReuse: true, MPConsume: 5, SkillType: "DUMMY",
		},
		{
			ID: modelskill.ID(gateToggleSkillID), Level: 1, Activation: modelskill.ActivationToggle, Target: modelskill.TargetSelf,
			MPConsume: 12, SkillType: "BUFF",
			Effects: []modelskill.EffectTemplate{{Name: "Buff", Time: 60, Icon: true}},
		},
		{
			ID: modelskill.ID(gateSiegeSkillID), Level: 1, Activation: modelskill.ActivationActive, Target: modelskill.TargetSelf,
			HitTime: 500, ReuseDelay: 60_000, StaticHitTime: true, StaticReuse: true, MPConsume: 5, SkillType: "DUMMY",
			SiegeSummonSkill: true,
		},
	}
}

func encodeRequestActionUse(actionID int32, ctrl, shift bool) []byte {
	w := wire.NewPacketWriter(clientpackets.OpcodeRequestActionUse)
	w.WriteInt32(actionID)
	w.WriteInt32(wire.BoolInt32(ctrl))
	w.WriteUint8(wire.BoolByte(shift))
	return w.Bytes()
}

// onPlayerQueue runs fn on the online player's actor queue and waits for it,
// staging state no single client packet reaches.
func onPlayerQueue(t *testing.T, srv *gameservertest.Server, objID int32, fn func(*player.Character)) {
	t.Helper()
	obj, ok := srv.State.Player(objID)
	if !ok {
		t.Fatal("player missing from world state")
	}
	pc, ok := network.OnlineCharacter(obj)
	if !ok {
		t.Fatalf("player %T is not an online character", obj)
	}
	done := make(chan struct{})
	if !pc.Queue().Post(func() { fn(pc); close(done) }) {
		t.Fatal("post to player queue: queue closed")
	}
	<-done
}

// TestPlayerPreAttemptGateRejectsBeforeAnyCost drives RequestMagicSkillUse
// against each player-only pre-attempt rule: the request answers with the
// rule's system message then ActionFailed, and nothing else — no cast
// packets, no MP spent, no cast in flight.
func TestPlayerPreAttemptGateRejectsBeforeAnyCost(t *testing.T) {
	t.Parallel()
	// sit seats the player and lets the sit-down end: a player still
	// sitting down is not seated yet.
	sit := func(t *testing.T, srv *gameservertest.Server, _ int32) {
		srv.Client.Send(encodeRequestActionUse(0, false, false))
		assertFrameOpcode(t, srv.Client.Read(), serverpackets.OpcodeChangeWaitType, "sit ChangeWaitType")
		srv.Advance(t, sitStandDelay)
		drainUntilQuiet(t, srv.Client)
	}
	for _, tt := range []struct {
		name    string
		skillID int32
		setup   func(*testing.T, *gameservertest.Server, int32)
		assert  func(*testing.T, []byte)
	}{
		{
			name: "sitting", skillID: gateActiveSkillID, setup: sit,
			assert: func(t *testing.T, f []byte) {
				assertStaticSystemMessage(t, f, serverpackets.SystemMessageCannotMoveWhileSitting)
			},
		},
		{
			name: "sitting toggle", skillID: gateToggleSkillID, setup: sit,
			assert: func(t *testing.T, f []byte) {
				assertStaticSystemMessage(t, f, serverpackets.SystemMessageCannotMoveWhileSitting)
			},
		},
		{
			// Reuse is the shared check and answers before the player's own.
			name: "reuse before sitting", skillID: gateActiveSkillID,
			setup: func(t *testing.T, srv *gameservertest.Server, objID int32) {
				onPlayerQueue(t, srv, objID, func(pc *player.Character) {
					pc.DisableSkill(cast.ReuseKey(gateSkills()[0]), time.Minute)
				})
				sit(t, srv, objID)
			},
			assert: func(t *testing.T, f []byte) {
				assertSystemMessageSkillFrame(t, f, serverpackets.SystemMessageS1PreparedForReuse, gateActiveSkillID, 1)
			},
		},
		{
			name: "fake death", skillID: gateActiveSkillID,
			setup: func(t *testing.T, srv *gameservertest.Server, objID int32) {
				onPlayerQueue(t, srv, objID, func(pc *player.Character) {
					fake, err := effect.New(effect.Skill{ID: 60}, modelskill.EffectTemplate{Name: "FakeDeath"})
					if err != nil {
						t.Errorf("new fake-death effect: %v", err)
						return
					}
					fake.Effected = pc
					pc.EffectList().Add(fake)
				})
				drainUntilQuiet(t, srv.Client)
			},
			assert: func(t *testing.T, f []byte) {
				assertStaticSystemMessage(t, f, serverpackets.SystemMessageCannotMoveWhileSitting)
			},
		},
		{
			name: "fishing", skillID: gateActiveSkillID,
			setup: func(t *testing.T, srv *gameservertest.Server, objID int32) {
				onPlayerQueue(t, srv, objID, func(pc *player.Character) { pc.SetFishing(true) })
			},
			assert: func(t *testing.T, f []byte) {
				assertStaticSystemMessage(t, f, serverpackets.SystemMessageOnlyFishingSkillsNow)
			},
		},
		{
			name: "formal wear", skillID: gateActiveSkillID,
			setup: func(t *testing.T, srv *gameservertest.Server, objID int32) {
				onPlayerQueue(t, srv, objID, func(pc *player.Character) {
					inv := pc.Inventory()
					inst := inv.ItemByTemplateID(gameservertest.FormalWearID)
					tmpl, ok := inv.Templates().Get(gameservertest.FormalWearID)
					if inst == nil || !ok {
						t.Errorf("formal wear item %v / template %v missing", inst, ok)
						return
					}
					inv.EquipItem(inst, tmpl)
				})
				drainUntilQuiet(t, srv.Client)
			},
			assert: func(t *testing.T, f []byte) {
				assertStaticSystemMessage(t, f, serverpackets.SystemMessageCannotUseSkillsWithFormalWear)
			},
		},
		{
			// No siege is ever active, so a siege-summon skill always refuses.
			name: "siege summon outside siege", skillID: gateSiegeSkillID,
			assert: func(t *testing.T, f []byte) {
				assertSystemMessageSkillFrame(t, f, serverpackets.SystemMessageS1CannotBeUsed, gateSiegeSkillID, 1)
			},
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			srv := gameservertest.Boot(t,
				gameservertest.WithCharacter("Newbie", 5, 0),
				gameservertest.WithWantChars(1),
				gameservertest.WithSkills(skillPersistence(t, gateSkills())),
			)
			c, objID := srv.Client, srv.SoleObjectID(t)
			for _, def := range gateSkills() {
				seedKnownSkill(t, srv, objID, int(def.ID), 1)
			}
			srv.GiveItem(t, objID, gameservertest.FormalWearID, 1)
			startInWorld(t, c)
			if tt.setup != nil {
				tt.setup(t, srv, objID)
			}
			mpBefore := srv.PlayerCurrentMP(t, objID)

			c.Send(encodeRequestMagicSkillUse(tt.skillID, false, false))
			tt.assert(t, c.Read())
			assertFrameOpcode(t, c.Read(), serverpackets.OpcodeActionFailed, "gate ActionFailed")
			if extra := c.ReadWithTimeout(300 * time.Millisecond); extra != nil {
				t.Fatalf("gate rejection extra opcode = %#x, want none", extra[0])
			}
			if got := srv.PlayerCurrentMP(t, objID); got != mpBefore {
				t.Fatalf("MP after rejected cast = %d, want unchanged %d", got, mpBefore)
			}
			if srv.PlayerCastingNow(t, objID) {
				t.Fatal("rejected cast is in flight")
			}
		})
	}
}

// TestWalkingInsufficientMPTargetedCastLeavesHeading pins a player's failed
// cost check on a long-hit-time targeted skill: the walk stops for the hit
// time, the reason is sent, and neither a MoveToPawn nor a heading toward
// the target follows.
func TestWalkingInsufficientMPTargetedCastLeavesHeading(t *testing.T) {
	t.Parallel()
	const skillID = 4
	srv := gameservertest.Boot(t,
		gameservertest.WithCharacter("Caster", 5, 0),
		gameservertest.WithWantChars(1),
		gameservertest.WithSkills(skillPersistence(t, []modelskill.Definition{{
			ID: skillID, Level: 1, Activation: modelskill.ActivationActive, Target: modelskill.TargetOne,
			HitTime: 500, ReuseDelay: 60_000, StaticHitTime: true, StaticReuse: true,
			CastRange: 600, MPConsume: 99_999, SkillType: "DUMMY",
		}})),
	)
	caster := srv.Client
	casterID := srv.SoleObjectID(t)
	patientID := srv.SeedCharacterFor(t, "player2", "Patient", 5, 0).ID
	patient := srv.DialClient(t, "player2", 1)
	seedKnownSkill(t, srv, casterID, skillID, 1)

	startInWorld(t, caster)
	startInWorldAmongPlayers(t, patient)
	drainUntilQuiet(t, caster)
	drainUntilQuiet(t, patient)

	px, py, pz := srv.PlayerPosition(t, patientID)
	caster.Send(encodeAction(patientID, int32(px), int32(py), int32(pz), false))
	drainUntilQuiet(t, caster)
	drainUntilQuiet(t, patient)

	caster.Send(encodeMoveBackwardToLocation(200, 70, 30))
	assertFrameOpcode(t, caster.Read(), serverpackets.OpcodeMoveToLocation, "walk")
	drainUntilQuiet(t, patient)
	headingBefore := playerHeading(t, srv, casterID)

	caster.Send(encodeRequestMagicSkillUse(skillID, false, false))
	assertFrameOpcode(t, caster.Read(), serverpackets.OpcodeStopMove, "cast stop")
	assertStaticSystemMessage(t, caster.Read(), serverpackets.SystemMessageNotEnoughMP)
	if extra := caster.ReadWithTimeout(300 * time.Millisecond); extra != nil {
		t.Fatalf("insufficient-MP rejection extra caster opcode = %#x, want no MoveToPawn", extra[0])
	}
	for {
		frame := patient.ReadWithTimeout(300 * time.Millisecond)
		if frame == nil {
			break
		}
		if frame[0] == serverpackets.OpcodeMoveToPawn {
			t.Fatal("observer received MoveToPawn for a player's rejected cast")
		}
	}
	if got := playerHeading(t, srv, casterID); got != headingBefore {
		t.Fatalf("caster heading after rejected cast = %d, want unchanged %d", got, headingBefore)
	}
}
