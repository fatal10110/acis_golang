package network

import (
	"bytes"
	"testing"
	"time"

	"github.com/fatal10110/acis_golang/internal/commons/wire"
	handlerskill "github.com/fatal10110/acis_golang/internal/gameserver/handler/skill"
	skilltarget "github.com/fatal10110/acis_golang/internal/gameserver/handler/target"
	actorcast "github.com/fatal10110/acis_golang/internal/gameserver/model/actor/cast"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/cubic"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/item"
	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/clientpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	skillstate "github.com/fatal10110/acis_golang/internal/gameserver/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/world"
	"github.com/fatal10110/acis_golang/internal/testsupport"
)

type recordingSkillHandler struct {
	applied chan []handlerskill.Actor
}

func (h recordingSkillHandler) Types() []string { return []string{"TEST_AREA"} }
func (h recordingSkillHandler) Use(cast handlerskill.Cast) {
	h.applied <- append([]handlerskill.Actor(nil), cast.Targets...)
}

func TestGameClientLinkSendsCounterattackFeedbackToPlayerParticipants(t *testing.T) {
	attackerFrames := &testsupport.FrameCapture{}
	defenderFrames := &testsupport.FrameCapture{}
	attacker := newTestLivePlayer(t, 1, attackerFrames)
	attacker.Name = "Attacker"
	defender := newTestLivePlayer(t, 2, defenderFrames)
	defender.Name = "Defender"
	state := world.New()
	state.AddPlayer(attacker)
	state.AddPlayer(defender)
	link := &GameClientLink{world: state}

	link.sendSkillHandlerResult(attacker, actorcast.EffectResult{
		Counterattacks: []handlerskill.Counterattack{{
			AttackerID: attacker.ObjectID(), DefenderID: defender.ObjectID(),
		}},
		AttackFailed: 1,
	})

	if len(attackerFrames.Frames()) != 2 {
		t.Fatalf("attacker frames = %d, want 2", len(attackerFrames.Frames()))
	}
	assertSystemMessageStringFrame(t, attackerFrames.Frames()[0], serverpackets.SystemMessageS1PerformingCounterattack, defender.Name)
	assertStaticSystemMessageFrame(t, attackerFrames.Frames()[1], serverpackets.SystemMessageAttackFailed)
	if len(defenderFrames.Frames()) != 1 {
		t.Fatalf("defender frames = %d, want 1", len(defenderFrames.Frames()))
	}
	assertSystemMessageStringFrame(t, defenderFrames.Frames()[0], serverpackets.SystemMessageCounteredS1Attack, attacker.Name)
}

func TestGameClientLinkSendsLethalFeedbackToPlayerParticipants(t *testing.T) {
	attackerFrames := &testsupport.FrameCapture{}
	targetFrames := &testsupport.FrameCapture{}
	attacker := newTestLivePlayer(t, 1, attackerFrames)
	target := newTestLivePlayer(t, 2, targetFrames)
	state := world.New()
	state.AddPlayer(attacker)
	state.AddPlayer(target)
	link := &GameClientLink{world: state}

	link.sendSkillHandlerResult(attacker, actorcast.EffectResult{Lethals: []handlerskill.Lethal{{
		AttackerID: attacker.ObjectID(), TargetID: target.ObjectID(),
	}}})

	if len(attackerFrames.Frames()) != 1 {
		t.Fatalf("attacker frames = %d, want 1", len(attackerFrames.Frames()))
	}
	assertStaticSystemMessageFrame(t, attackerFrames.Frames()[0], 1668)
	if len(targetFrames.Frames()) != 1 {
		t.Fatalf("target frames = %d, want 1", len(targetFrames.Frames()))
	}
	assertStaticSystemMessageFrame(t, targetFrames.Frames()[0], 1667)
}

func TestGameClientLinkSendsResistedSkillFeedbackToCaster(t *testing.T) {
	frames := &testsupport.FrameCapture{}
	caster := newTestLivePlayer(t, 1, frames)
	link := &GameClientLink{}

	link.sendSkillHandlerResult(caster, actorcast.EffectResult{Resisted: []handlerskill.Resisted{{
		TargetName: "Target", SkillID: 123, SkillLevel: 1,
	}}})

	if len(frames.Frames()) != 1 {
		t.Fatalf("caster frames = %d, want 1", len(frames.Frames()))
	}
	frame := frames.Frames()[0]
	if frame[0] != serverpackets.OpcodeSystemMessage {
		t.Fatalf("opcode = %#x, want SystemMessage", frame[0])
	}
	r := wire.NewReader(frame[1:])
	if id := r.ReadInt32(); id != serverpackets.SystemMessageS1ResistedYourS2 {
		t.Fatalf("message ID = %d, want resisted skill", id)
	}
	if params := r.ReadInt32(); params != 2 {
		t.Fatalf("parameters = %d, want 2", params)
	}
	if typ := r.ReadInt32(); typ != serverpackets.SystemMessageParamText || r.ReadString() != "Target" {
		t.Fatalf("first parameter = text Target")
	}
	if typ := r.ReadInt32(); typ != serverpackets.SystemMessageParamSkillName || r.ReadInt32() != 123 || r.ReadInt32() != 1 {
		t.Fatalf("second parameter = skill 123 level 1")
	}
}

func TestGameClientLinkSendsBlowEvasionFeedbackToPlayerParticipants(t *testing.T) {
	attackerFrames := &testsupport.FrameCapture{}
	defenderFrames := &testsupport.FrameCapture{}
	attacker := newTestLivePlayer(t, 1, attackerFrames)
	attacker.Name = "Attacker"
	defender := newTestLivePlayer(t, 2, defenderFrames)
	defender.Name = "Defender"
	state := world.New()
	state.AddPlayer(attacker)
	state.AddPlayer(defender)
	link := &GameClientLink{world: state}

	link.sendSkillHandlerResult(attacker, actorcast.EffectResult{Dodges: []handlerskill.Dodge{{
		AttackerID: attacker.ObjectID(), DefenderID: defender.ObjectID(),
	}}})

	if len(attackerFrames.Frames()) != 1 || len(defenderFrames.Frames()) != 1 {
		t.Fatalf("evasion frames = attacker %d defender %d, want 1 each", len(attackerFrames.Frames()), len(defenderFrames.Frames()))
	}
	assertSystemMessageStringFrame(t, attackerFrames.Frames()[0], serverpackets.SystemMessageS1DodgesAttack, "Defender")
	assertSystemMessageStringFrame(t, defenderFrames.Frames()[0], serverpackets.SystemMessageAvoidedS1Attack, "Attacker")
}

func TestGameClientLinkSendsCounterattackFeedbackWithNonPlayerName(t *testing.T) {
	frames := &testsupport.FrameCapture{}
	attacker := newTestLivePlayer(t, 1, frames)
	attacker.Name = "Attacker"
	state := world.New()
	state.AddPlayer(attacker)
	link := &GameClientLink{world: state}

	link.sendSkillHandlerResult(attacker, actorcast.EffectResult{Counterattacks: []handlerskill.Counterattack{{
		AttackerID: attacker.ObjectID(), DefenderID: 2, DefenderName: "Countering NPC",
	}}})

	if len(frames.Frames()) != 1 {
		t.Fatalf("attacker frames = %d, want 1", len(frames.Frames()))
	}
	assertSystemMessageStringFrame(t, frames.Frames()[0], serverpackets.SystemMessageS1PerformingCounterattack, "Countering NPC")
}

func TestGameClientLinkMagicSkillUseRejectsMissingSkillItemsWithSkillName(t *testing.T) {
	capture := &testsupport.FrameCapture{}
	live := newTestLivePlayer(t, 7, capture)
	live.Character.SetSkillLevel(3, 1)
	link := &GameClientLink{skills: skillstate.NewPersistence(nil, modelskill.NewTable([]modelskill.Definition{{
		ID: 3, Level: 1, Activation: modelskill.ActivationActive, Target: modelskill.TargetSelf,
		ItemConsumeID: 57, ItemConsumeCount: 1,
	}}), nil)}

	link.handleMagicSkillUse(live, clientpackets.RequestMagicSkillUse{SkillID: 3})

	if got, want := testsupport.FrameOpcodes(capture.Frames()), []byte{serverpackets.OpcodeSystemMessage, serverpackets.OpcodeActionFailed}; !bytes.Equal(got, want) {
		t.Fatalf("missing-item cast opcodes = %#x, want SystemMessage then ActionFailed (%#x)", got, want)
	}
	assertSystemMessageSkillFrame(t, capture.Frames()[0], serverpackets.SystemMessageS1CannotBeUsed, 3, 1)
}

func TestGameClientLinkMagicSkillUseMissingOneTargetSendsActionFailedOnly(t *testing.T) {
	capture := &testsupport.FrameCapture{}
	live := newTestLivePlayer(t, 7, capture)
	live.Character.SetSkillLevel(3, 1)
	link := &GameClientLink{skills: skillstate.NewPersistence(nil, modelskill.NewTable([]modelskill.Definition{{
		ID: 3, Level: 1, Activation: modelskill.ActivationActive, Target: modelskill.TargetOne,
	}}), nil)}

	link.handleMagicSkillUse(live, clientpackets.RequestMagicSkillUse{SkillID: 3})

	if got, want := testsupport.FrameOpcodes(capture.Frames()), []byte{serverpackets.OpcodeActionFailed}; !bytes.Equal(got, want) {
		t.Fatalf("missing-target cast opcodes = %#x, want ActionFailed only (%#x)", got, want)
	}
}

type groundCastLOS bool

func (g groundCastLOS) CanSeeActor(int, int, int, float64, int, int, int, float64) bool {
	return bool(g)
}

type groundCastPeaceZone bool

func (g groundCastPeaceZone) EffectRangeInPeaceZone(int, int, int, int, int, int) bool {
	return bool(g)
}

func TestGameClientLinkMagicSkillUseGroundCastFailuresSendReason(t *testing.T) {
	tests := []struct {
		name      string
		configure func(*livePlayer)
		assert    func(*testing.T, []byte)
	}{
		{
			name: "line of sight",
			configure: func(live *livePlayer) {
				live.Character.SetLineOfSight(groundCastLOS(false))
			},
			assert: func(t *testing.T, frame []byte) {
				assertSystemMessageIDFrame(t, frame, serverpackets.SystemMessageCantSeeTarget)
			},
		},
		{
			name: "peace zone",
			configure: func(live *livePlayer) {
				live.Character.SetZones(groundCastPeaceZone(true))
			},
			assert: func(t *testing.T, frame []byte) {
				assertSystemMessageSkillFrame(t, frame, serverpackets.SystemMessageS1CannotBeUsed, 3, 1)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			capture := &testsupport.FrameCapture{}
			live := newTestLivePlayer(t, 7, capture)
			live.Character.SetSkillLevel(3, 1)
			live.Character.SetGroundTarget(800, 100, 0)
			tt.configure(live)
			link := &GameClientLink{
				skills: skillstate.NewPersistence(nil, modelskill.NewTable([]modelskill.Definition{{
					ID: 3, Level: 1, Activation: modelskill.ActivationActive, Target: modelskill.TargetGround, CastRange: 3000,
				}}), nil),
				targets: skilltarget.NewRegistry(nil),
			}

			link.handleMagicSkillUse(live, clientpackets.RequestMagicSkillUse{SkillID: 3})

			if got, want := testsupport.FrameOpcodes(capture.Frames()), []byte{serverpackets.OpcodeSystemMessage, serverpackets.OpcodeActionFailed}; !bytes.Equal(got, want) {
				t.Fatalf("ground cast rejection opcodes = %#x, want SystemMessage then ActionFailed (%#x)", got, want)
			}
			tt.assert(t, capture.Frames()[0])
		})
	}
}

func TestGameClientLinkMagicSkillUseGroundCastFacesPointAndBroadcastsPosition(t *testing.T) {
	capture := &testsupport.FrameCapture{}
	live := newTestLivePlayer(t, 7, capture)
	state := world.New()
	live.SetWorld(state)
	state.Spawn(live, 700, 0, 0, 0)
	state.AddPlayer(live)
	live.Character.SetSkillLevel(3, 1)
	live.Character.SetGroundTarget(700, 100, 0)
	link := &GameClientLink{
		skills: skillstate.NewPersistence(nil, modelskill.NewTable([]modelskill.Definition{{
			ID: 3, Level: 1, Activation: modelskill.ActivationActive, Target: modelskill.TargetGround, CastRange: 3000, HitTime: int(time.Hour / time.Millisecond),
		}}), nil),
		targets: skilltarget.NewRegistry(nil),
	}

	link.handleMagicSkillUse(live, clientpackets.RequestMagicSkillUse{SkillID: 3})

	frames := capture.Frames()
	if got, want := testsupport.FrameOpcodes(frames)[:2], []byte{serverpackets.OpcodeValidateLocation, serverpackets.OpcodeMagicSkillUse}; !bytes.Equal(got, want) {
		t.Fatalf("ground cast opening opcodes = %#x, want ValidateLocation then MagicSkillUse (%#x)", got, want)
	}
	if got := live.CurrentHeading(); got != 16384 {
		t.Fatalf("heading after ground cast = %d, want 16384", got)
	}
	live.cast.Stop()
}

func TestGameClientLinkMagicSkillUseAppliesAreaSkillToResolvedTargets(t *testing.T) {
	state := world.New()
	casterCapture := &testsupport.FrameCapture{}
	caster := newTestLivePlayer(t, 7, casterCapture)
	aimed := newTestHostileNPC(t, 8)
	nearby := newTestHostileNPC(t, 9)
	state.Spawn(caster, 0, 0, 0, 0)
	state.AddPlayer(caster)
	state.Spawn(aimed, 100, 0, 0, 0)
	state.Spawn(nearby, 200, 0, 0, 0)
	caster.Character.SetSkillLevel(3, 1)
	caster.SetTargetTracked(aimed)

	recorded := recordingSkillHandler{applied: make(chan []handlerskill.Actor, 1)}
	link := &GameClientLink{
		skills: skillstate.NewPersistence(nil, modelskill.NewTable([]modelskill.Definition{{
			ID: 3, Level: 1, Activation: modelskill.ActivationActive, Target: modelskill.TargetArea,
			Radius: 150, SkillType: "TEST_AREA",
		}}), nil),
		targets:       skilltarget.NewRegistry(skilltarget.WorldKnown{State: state}),
		skillHandlers: handlerskill.NewRegistry(recorded),
	}

	link.handleMagicSkillUse(caster, clientpackets.RequestMagicSkillUse{SkillID: 3})

	select {
	case targets := <-recorded.applied:
		got := map[int32]bool{}
		for _, target := range targets {
			got[target.(interface{ ObjectID() int32 }).ObjectID()] = true
		}
		if len(got) != 2 || !got[aimed.ObjectID()] || !got[nearby.ObjectID()] {
			t.Fatalf("area effect targets = %v, want %d and %d", got, aimed.ObjectID(), nearby.ObjectID())
		}
	case <-time.After(time.Second):
		t.Fatalf("area cast did not reach its skill handler; frames = %#x", testsupport.FrameOpcodes(casterCapture.Frames()))
	}

	for _, frame := range casterCapture.Frames() {
		if frame[0] != serverpackets.OpcodeMagicSkillLaunched {
			continue
		}
		r := wire.NewReader(frame[1:])
		r.ReadInt32() // caster
		r.ReadInt32() // skill ID
		r.ReadInt32() // level
		if count := r.ReadInt32(); count != 2 {
			t.Fatalf("MagicSkillLaunched target count = %d, want 2", count)
		}
		got := map[int32]bool{r.ReadInt32(): true, r.ReadInt32(): true}
		if !got[aimed.ObjectID()] || !got[nearby.ObjectID()] {
			t.Fatalf("MagicSkillLaunched targets = %v, want %d and %d", got, aimed.ObjectID(), nearby.ObjectID())
		}
		return
	}
	t.Fatal("area cast did not send MagicSkillLaunched")
}

func TestGameClientLinkMagicSkillUseMassCubicRefreshesEachRecipient(t *testing.T) {
	state := world.New()
	casterFrames := &testsupport.FrameCapture{}
	firstFrames := &testsupport.FrameCapture{}
	secondFrames := &testsupport.FrameCapture{}
	caster := newTestLivePlayer(t, 7, casterFrames)
	first := newTestLivePlayer(t, 8, firstFrames)
	second := newTestLivePlayer(t, 9, secondFrames)
	state.Spawn(caster, 0, 0, 0, 0)
	state.Spawn(first, 100, 0, 0, 0)
	state.Spawn(second, 200, 0, 0, 0)
	state.AddPlayer(caster)
	state.AddPlayer(first)
	state.AddPlayer(second)
	caster.Character.SetSkillLevel(10, 1)
	caster.SetTargetTracked(first)
	first.KarmaPoints = 1
	second.KarmaPoints = 1
	testsupport.ResetCapture(casterFrames)
	testsupport.ResetCapture(firstFrames)
	testsupport.ResetCapture(secondFrames)

	clock := &fakeCubicClock{}
	link := newCubicTestLink(t, clock, &attackStanceRecorder{}, modelskill.Definition{
		ID: 10, Level: 1, Activation: modelskill.ActivationActive, Target: modelskill.TargetArea,
		Radius: 150, SkillType: "SUMMON", IsCubic: true, NpcID: int(cubic.Storm),
		CubicActivationTime: 8, CubicActivationChance: 30, SummonTotalLifeTime: 1200000,
		HitTime: 1, StaticHitTime: true, StaticReuse: true,
	})
	link.world = state
	link.targets = skilltarget.NewRegistry(skilltarget.WorldKnown{State: state})

	link.handleMagicSkillUse(caster, clientpackets.RequestMagicSkillUse{SkillID: 10})
	var firstSent, secondSent, casterSent [][]byte
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		firstSent = firstFrames.Frames()
		secondSent = secondFrames.Frames()
		casterSent = casterFrames.Frames()
		if len(framesWithOpcode(firstSent, serverpackets.OpcodeUserInfo)) == 1 &&
			len(framesWithOpcode(secondSent, serverpackets.OpcodeUserInfo)) == 1 &&
			len(framesWithOpcode(casterSent, serverpackets.OpcodeCharInfo)) == 2 {
			break
		}
		time.Sleep(time.Millisecond)
	}

	for _, recipient := range []struct {
		name   string
		live   *livePlayer
		frames [][]byte
	}{{"first", first, firstSent}, {"second", second, secondSent}} {
		if ids := recipient.live.Character.CubicIDs(); len(ids) != 1 || ids[0] != int(cubic.Storm) {
			t.Fatalf("%s cubic IDs = %v, want [%d]", recipient.name, ids, cubic.Storm)
		}
		if got := framesWithOpcode(recipient.frames, serverpackets.OpcodeUserInfo); len(got) != 1 {
			t.Fatalf("%s UserInfo frames = %d, want 1", recipient.name, len(got))
		}
	}
	if got := framesWithOpcode(casterSent, serverpackets.OpcodeCharInfo); len(got) != 2 {
		t.Fatalf("caster CharInfo frames = %d, want 2 for the two cubic recipients", len(got))
	}
}

func TestGameClientLinkMagicSkillUseDefersUntilAttackFinishes(t *testing.T) {
	capture := &testsupport.FrameCapture{}
	live := newTestLivePlayer(t, 7, capture)
	live.Character.SetSkillLevel(3, 1)
	link := &GameClientLink{skills: skillstate.NewPersistence(nil, modelskill.NewTable([]modelskill.Definition{{
		ID: 3, Level: 1, Activation: modelskill.ActivationActive, Target: modelskill.TargetSelf,
		HitTime: 500, ReuseDelay: 1200, StaticHitTime: true, StaticReuse: true,
	}}), nil)}

	if err := live.attack.DoAttack(newTestHostileNPC(t, 100)); err != nil {
		t.Fatalf("start attack: %v", err)
	}
	testsupport.ResetCapture(capture)

	link.handleMagicSkillUse(live, clientpackets.RequestMagicSkillUse{SkillID: 3})
	if link.castController(live).CastingNow() {
		t.Fatal("cast started before the active attack finished")
	}
	if got, want := testsupport.FrameOpcodes(capture.Frames()), []byte{serverpackets.OpcodeActionFailed}; !bytes.Equal(got, want) {
		t.Fatalf("deferred cast opcodes = %#x, want ActionFailed (%#x)", got, want)
	}

	live.attack.Stop()
	testsupport.ResetCapture(capture)
	link.finishDeferredMagicSkill(live)
	if !link.castController(live).CastingNow() {
		t.Fatal("deferred cast did not start after the attack finished")
	}
	if got, want := testsupport.FrameOpcodes(capture.Frames()), []byte{serverpackets.OpcodeMagicSkillUse, serverpackets.OpcodeSystemMessage, serverpackets.OpcodeSetupGauge}; !bytes.Equal(got, want) {
		t.Fatalf("drained cast opcodes = %#x, want MagicSkillUse, SystemMessage, SetupGauge (%#x)", got, want)
	}
}

// TestAbortedCastSendsCancelAndActionFailed pins the two packets an aborted
// in-flight cast owes the client. The abort triggers themselves (damage,
// mute, death, ...) are wired separately, so this drives the funnel
// directly.
func TestAbortedCastSendsCancelAndActionFailed(t *testing.T) {
	capture := &testsupport.FrameCapture{}
	link := &GameClientLink{}
	live := newEquipTestLivePlayer(t, 7, capture, item.NewTable(nil), nil)
	controller := link.castController(live)

	def := modelskill.Definition{
		ID: 3, Level: 1, Activation: modelskill.ActivationActive, Target: modelskill.TargetSelf,
		HitTime: 5000, ReuseDelay: 1200, StaticHitTime: true, StaticReuse: true,
	}
	if _, err := controller.Start(time.Now(), live, def); err != nil {
		t.Fatalf("Start() error: %v", err)
	}
	testsupport.ResetCapture(capture)

	controller.Stop()

	if got, want := testsupport.FrameOpcodes(capture.Frames()), []byte{serverpackets.OpcodeMagicSkillCanceled, serverpackets.OpcodeActionFailed}; !bytes.Equal(got, want) {
		t.Fatalf("abort opcodes = %#x, want MagicSkillCanceled then ActionFailed (%#x)", got, want)
	}
	if caster := wire.NewReader(capture.Frames()[0][1:]).ReadInt32(); caster != live.ObjectID() {
		t.Fatalf("MagicSkillCanceled caster = %d, want %d", caster, live.ObjectID())
	}

	// An idle Stop still owes the caster ActionFailed: PlayerCast.stop()
	// calls clientActionFailed() unconditionally, after (and regardless of)
	// super.stop()'s isCastingNow()-gated cancel broadcast
	// (PlayerCast.java:381-387, PlayerAI.java:556-560).
	testsupport.ResetCapture(capture)
	controller.Stop()
	if got, want := testsupport.FrameOpcodes(capture.Frames()), []byte{serverpackets.OpcodeActionFailed}; !bytes.Equal(got, want) {
		t.Fatalf("idle Stop opcodes = %#x, want ActionFailed only (%#x)", got, want)
	}
}

// TestInterruptedCastSendsCancelCastingInterruptedAndActionFailed pins the
// interrupt() vs stop() distinction: an abort inside the interrupt window
// additionally sends CASTING_INTERRUPTED to the caster, matching
// CreatureCast.interrupt() vs the unconditional stop() TestAbortedCastSends
// CancelAndActionFailed covers.
func TestInterruptedCastSendsCancelCastingInterruptedAndActionFailed(t *testing.T) {
	capture := &testsupport.FrameCapture{}
	link := &GameClientLink{}
	live := newEquipTestLivePlayer(t, 7, capture, item.NewTable(nil), nil)
	controller := link.castController(live)

	def := modelskill.Definition{
		ID: 3, Level: 1, Activation: modelskill.ActivationActive, Target: modelskill.TargetSelf,
		HitTime: 5000, ReuseDelay: 1200, StaticHitTime: true, StaticReuse: true,
	}
	now := time.Now()
	if _, err := controller.Start(now, live, def); err != nil {
		t.Fatalf("Start() error: %v", err)
	}
	testsupport.ResetCapture(capture)

	if !controller.Interrupt(now.Add(100 * time.Millisecond)) {
		t.Fatal("Interrupt() = false inside the interrupt window, want true")
	}

	want := []byte{serverpackets.OpcodeMagicSkillCanceled, serverpackets.OpcodeSystemMessage, serverpackets.OpcodeActionFailed}
	if got := testsupport.FrameOpcodes(capture.Frames()); !bytes.Equal(got, want) {
		t.Fatalf("interrupt opcodes = %#x, want MagicSkillCanceled, SystemMessage, ActionFailed (%#x)", got, want)
	}
}
