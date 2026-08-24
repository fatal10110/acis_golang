package network

import (
	"bytes"
	"testing"
	"time"

	skilltarget "github.com/fatal10110/acis_golang/internal/gameserver/handler/target"
	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/clientpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	skillstate "github.com/fatal10110/acis_golang/internal/gameserver/skill"
	"github.com/fatal10110/acis_golang/internal/testsupport"
)

func TestGameClientLinkCorpseCastFailuresSendReason(t *testing.T) {
	tests := []struct {
		name      string
		skillType string
		guard     bool
		deadline  time.Time
		messageID int
	}{
		{"harvest on guard", "HARVEST", true, time.Now().Add(time.Hour), serverpackets.SystemMessageHarvestFailedSeedNotSown},
		{"sweep on guard", "SWEEP", true, time.Now().Add(time.Hour), serverpackets.SystemMessageSweeperFailedTargetNotSpoiled},
		{"too old corpse", "DUMMY", false, time.Now().Add(time.Second), serverpackets.SystemMessageCorpseTooOldSkillNotUsed},
	}

	for skillID, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			capture := &testsupport.FrameCapture{}
			live := newTestLivePlayer(t, 1, capture)
			hostile := newTestHostileNPC(t, 2)
			hostile.Instance.Template.CorpseTime = 10
			if tt.guard {
				hostile.Instance.Kind = "Guard"
			}
			if !hostile.Die(nil, nil) {
				t.Fatal("hostile did not die")
			}
			hostile.SetCorpseDeadline(tt.deadline)
			live.Character.SetTargetTracked(hostile)

			id := 900 + skillID
			live.SetSkillLevel(id, 1)
			store := newMemorySkillSaveStore()
			link := &GameClientLink{
				skills: skillstate.NewPersistence(store, modelskill.NewTable([]modelskill.Definition{{
					ID: modelskill.ID(id), Level: 1, Activation: modelskill.ActivationActive,
					Target: modelskill.TargetCorpseMob, SkillType: tt.skillType,
				}}), store),
				targets: skilltarget.NewRegistry(nil),
			}

			link.handleMagicSkillUse(live, clientpackets.RequestMagicSkillUse{SkillID: int32(id)})

			if got, want := testsupport.FrameOpcodes(capture.Frames()), []byte{serverpackets.OpcodeSystemMessage, serverpackets.OpcodeActionFailed}; !bytes.Equal(got, want) {
				t.Fatalf("opcodes = %x, want SystemMessage then ActionFailed", got)
			}
			assertSystemMessageIDFrame(t, capture.Frames()[0], tt.messageID)
		})
	}
}
