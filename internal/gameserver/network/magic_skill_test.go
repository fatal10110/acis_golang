package network

import (
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/geo/block"
	handlerskill "github.com/fatal10110/acis_golang/internal/gameserver/handler/skill"
	skilltarget "github.com/fatal10110/acis_golang/internal/gameserver/handler/target"
	actorcast "github.com/fatal10110/acis_golang/internal/gameserver/model/actor/cast"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/door"
	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/world"
	"github.com/fatal10110/acis_golang/internal/testsupport"
)

// TestSendSkillHandlerResultDeliversTargetMessagesWithNilCaster pins the
// nil-live-safe split (issue #2350): a caster with no live connection (a
// hostile NPC's AIController.OnHitResult) still delivers target-addressed
// messages resolved by ID lookup, while caster-addressed messages, which
// have nowhere to go, are silently skipped instead of panicking on the nil
// receiver.
func TestSendSkillHandlerResultDeliversTargetMessagesWithNilCaster(t *testing.T) {
	frames := &testsupport.FrameCapture{}
	target := newTestLivePlayer(t, 42, frames)
	state := world.New()
	state.AddPlayer(target)
	l := &GameClientLink{world: state}

	l.sendSkillHandlerResult(nil, actorcast.EffectResult{
		MagicResists:      []handlerskill.MagicResist{{TargetID: 42, AttackerName: "Orc"}},
		ManaDrains:        []handlerskill.ManaDrain{{TargetID: 42, CasterName: "Orc", MP: 30}},
		Resisted:          []handlerskill.Resisted{{TargetName: "Orc", SkillID: 1, SkillLevel: 1}},
		AttackFailed:      1,
		ManaDamageMissed:  1,
		OpponentMPReduced: []int32{5},
	})

	got := frames.Frames()
	opcodes := testsupport.FrameOpcodes(got)
	if len(opcodes) != 2 || opcodes[0] != serverpackets.OpcodeSystemMessage || opcodes[1] != serverpackets.OpcodeSystemMessage {
		t.Fatalf("opcodes = %x, want two SystemMessage frames (MagicResist, ManaDrain) and nothing else", opcodes)
	}
}

// TestDeliverHitResultForwardsToSendSkillHandlerResult pins the exported
// wrapper boot wiring uses to give a hostile NPC's OnHitResult a delivery
// path with no live caster connection (issue #2350).
func TestDeliverHitResultForwardsToSendSkillHandlerResult(t *testing.T) {
	frames := &testsupport.FrameCapture{}
	target := newTestLivePlayer(t, 43, frames)
	state := world.New()
	state.AddPlayer(target)
	l := &GameClientLink{world: state}

	l.DeliverHitResult(actorcast.EffectResult{
		ManaDrains: []handlerskill.ManaDrain{{TargetID: 43, CasterName: "Orc", MP: 12}},
	})

	if opcodes := testsupport.FrameOpcodes(frames.Frames()); len(opcodes) != 1 || opcodes[0] != serverpackets.OpcodeSystemMessage {
		t.Fatalf("opcodes = %x, want one SystemMessage frame (ManaDrain)", opcodes)
	}
}

func TestTargetCastRejectionsSendMessageBeforeActionFailed(t *testing.T) {
	frames := &testsupport.FrameCapture{}
	live := newTestLivePlayer(t, 1, frames)

	for _, tt := range []struct {
		name      string
		rejection skilltarget.CastRejection
		def       modelskill.Definition
		message   int
	}{
		{"aura", skilltarget.CastRejectCantAttackPeaceZone, modelskill.Definition{}, serverpackets.SystemMessageCantAtkPeacezone},
		{"front aura", skilltarget.CastRejectCantAttackPeaceZone, modelskill.Definition{}, serverpackets.SystemMessageCantAtkPeacezone},
		{"behind aura", skilltarget.CastRejectCantAttackPeaceZone, modelskill.Definition{}, serverpackets.SystemMessageCantAtkPeacezone},
		{"one invalid", skilltarget.CastRejectInvalidTarget, modelskill.Definition{}, serverpackets.SystemMessageInvalidTarget},
		{"one target in peace", skilltarget.CastRejectTargetInPeaceZone, modelskill.Definition{}, serverpackets.SystemMessageTargetInPeacezone},
		{"corpse pet non-pet", skilltarget.CastRejectCannotUseSkill, modelskill.Definition{ID: 2179, Level: 1}, serverpackets.SystemMessageS1CannotBeUsed},
	} {
		t.Run(tt.name, func(t *testing.T) {
			testsupport.ResetCapture(frames)
			sendTargetCastRejection(live, tt.rejection, tt.def)
			sendMagicActionFailed(live)

			got := frames.Frames()
			if opcodes := testsupport.FrameOpcodes(got); len(opcodes) != 2 || opcodes[0] != serverpackets.OpcodeSystemMessage || opcodes[1] != serverpackets.OpcodeActionFailed {
				t.Fatalf("opcodes = %x, want [SystemMessage, ActionFailed]", opcodes)
			}
			if tt.def.ID != 0 {
				assertSystemMessageSkillFrame(t, got[0], tt.message, int32(tt.def.ID), int32(tt.def.Level))
				return
			}
			assertStaticSystemMessageFrame(t, got[0], tt.message)
		})
	}
}

type magicSkillDoorShape struct{}

func (magicSkillDoorShape) GeoX() int               { return 0 }
func (magicSkillDoorShape) GeoY() int               { return 0 }
func (magicSkillDoorShape) GeoZ() int               { return 0 }
func (magicSkillDoorShape) Height() int             { return 0 }
func (magicSkillDoorShape) GeoData() [][]block.NSWE { return [][]block.NSWE{{block.AllDirections}} }

func TestResolveMagicSkillTargetKeepsLockedDoorForSilentRejection(t *testing.T) {
	frames := &testsupport.FrameCapture{}
	live := newTestLivePlayer(t, 1, frames)
	locked, err := door.NewObject(2, &door.Template{ID: 2, OpenKind: door.OpenClick}, magicSkillDoorShape{})
	if err != nil {
		t.Fatal(err)
	}
	l := &GameClientLink{targets: skilltarget.NewRegistry(nil)}
	target, rejection := l.resolveMagicSkillTarget(live.Character, locked, modelskill.Definition{Target: modelskill.TargetUnlockable}, false)
	if target != locked {
		t.Fatalf("target = %T, want locked door", target)
	}
	if rejection != skilltarget.CastRejectSilent {
		t.Fatalf("rejection = %v, want silent", rejection)
	}
	testsupport.ResetCapture(frames)
	sendTargetCastRejection(live, rejection, modelskill.Definition{})
	if got := testsupport.FrameOpcodes(frames.Frames()); len(got) != 0 {
		t.Fatalf("silent rejection opcodes = %x, want none", got)
	}
	testsupport.ResetCapture(frames)
	l.rejectMagicCast(live, modelskill.Definition{HitTime: 500}, locked)
	if got := testsupport.FrameOpcodes(frames.Frames()); len(got) != 1 || got[0] != serverpackets.OpcodeMoveToPawn {
		t.Fatalf("silent rejection opcodes = %x, want [MoveToPawn]", got)
	}
}

// TestFusionTargetClearsOnlyTheChannelThatSetIt pins the clear-if-still-mine
// rule the fusion target accessors carry: handleMagicSkillUse installs the
// channel's target (magic_skill.go:129) and its finishFusion clears it by the
// same id (magic_skill.go:132). When an earlier channel finishes after the
// caster has started a new one, clearing must be a no-op, or
// abortFusionTargeting (magic_skill.go:344) stops recognizing the caster as
// fusing its current target.
func TestFusionTargetClearsOnlyTheChannelThatSetIt(t *testing.T) {
	const (
		first  = int32(11)
		second = int32(22)
	)

	var live livePlayer
	live.setFusionTarget(first)
	live.setFusionTarget(second)

	live.clearFusionTarget(first)
	if !live.fusesTarget(second) {
		t.Fatal("fusesTarget(second) = false after the superseded channel cleared its own target, want true")
	}
	if live.fusesTarget(first) {
		t.Fatal("fusesTarget(first) = true, want the superseded target gone")
	}

	live.clearFusionTarget(second)
	if live.fusesTarget(second) {
		t.Fatal("fusesTarget(second) = true after its own channel cleared it, want false")
	}
}
