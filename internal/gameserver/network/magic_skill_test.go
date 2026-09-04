package network

import (
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/geo/block"
	skilltarget "github.com/fatal10110/acis_golang/internal/gameserver/handler/target"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/door"
	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/testsupport"
)

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
}
