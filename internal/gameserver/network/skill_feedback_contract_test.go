package network

import (
	"testing"

	"github.com/fatal10110/acis_golang/internal/commons/wire"
	handlerskill "github.com/fatal10110/acis_golang/internal/gameserver/handler/skill"
	actorcast "github.com/fatal10110/acis_golang/internal/gameserver/model/actor/cast"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/world"
	"github.com/fatal10110/acis_golang/internal/testsupport"
)

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
