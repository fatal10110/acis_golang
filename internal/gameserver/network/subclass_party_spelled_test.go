package network

import (
	"encoding/binary"
	"testing"

	"github.com/rs/zerolog"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/player"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/shortcut"
	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/party"
	"github.com/fatal10110/acis_golang/internal/gameserver/skill/effect"
	"github.com/fatal10110/acis_golang/internal/testsupport"
)

// TestClassSwitchRefreshesPartyIcons pins the icon pass a class switch
// ends with (Player.setActiveClass: restoreEffects, then updateEffectIcons,
// then EtcStatusUpdate, Player.java:5886-5888). A party member whose list
// once held an effect sends itself an AbnormalStatusUpdate right before
// the EtcStatusUpdate, and its party, itself included, a PartySpelled of
// it (EffectList.java:778-864); one whose list never held an effect sends
// neither (EffectList.java:428-431).
func TestClassSwitchRefreshesPartyIcons(t *testing.T) {
	for _, tt := range []struct {
		name  string
		buffs bool
	}{
		{name: "never buffed"},
		{name: "buffed", buffs: true},
	} {
		t.Run(tt.name, func(t *testing.T) {
			link := &GameClientLink{log: zerolog.Nop(), templates: testTemplates(t), parties: party.NewRegistry[*livePlayer](nil)}
			switcherOut, otherOut := &testsupport.FrameCapture{}, &testsupport.FrameCapture{}
			switcher := newTestLivePlayer(t, 1, switcherOut)
			other := newTestLivePlayer(t, 2, otherOut)
			switcher.shortcuts = shortcut.NewList(nil)
			if !switcher.AddSubclass(player.SubClass{ClassID: switcher.BaseClassID(), Index: 1, Level: 40}) {
				t.Fatal("add subclass refused")
			}
			if status, _ := link.parties.BeginInvite(switcher.ObjectID(), 0); status != party.InviteReady {
				t.Fatalf("BeginInvite = %v", status)
			}
			link.parties.Answer(switcher, other, true)
			if tt.buffs {
				e, err := effect.New(effect.Skill{ID: 1068, Level: 1, SkillType: "BUFF"}, modelskill.EffectTemplate{Name: "Buff", Time: 60, Icon: true})
				if err != nil {
					t.Fatalf("effect.New: %v", err)
				}
				e.Effector, e.Effected = switcher.Character, switcher.Character
				switcher.EffectList().Add(e)
			}
			switcherSkip, otherSkip := len(switcherOut.Frames()), len(otherOut.Frames())

			link.switchClass(switcher, 1, classChangeRows{})

			own := switcherOut.Frames()[switcherSkip:]
			mate := otherOut.Frames()[otherSkip:]
			ownSpelled := partySpelledOf(own, switcher.ObjectID())
			mateSpelled := partySpelledOf(mate, switcher.ObjectID())
			status := -1
			for i, f := range own {
				if f[0] == serverpackets.OpcodeAbnormalStatusUpdate {
					status = i
				}
			}
			if !tt.buffs {
				if status >= 0 || ownSpelled+mateSpelled != 0 {
					t.Fatalf("never-buffed switch sent icons: own opcodes %x, mate opcodes %x", opcodesOf(own), opcodesOf(mate))
				}
				return
			}
			if status < 0 || status+2 >= len(own) || own[status+1][0] != serverpackets.OpcodePartySpelled || own[status+2][0] != serverpackets.OpcodeEtcStatusUpdate {
				t.Fatalf("switcher opcodes = %x, want AbnormalStatusUpdate, PartySpelled, EtcStatusUpdate", opcodesOf(own))
			}
			if ownSpelled != 1 || mateSpelled != 1 {
				t.Fatalf("PartySpelled of the switcher: own %d, mate %d, want 1 each", ownSpelled, mateSpelled)
			}
		})
	}
}

// partySpelledOf counts the player PartySpelled frames of objectID.
func partySpelledOf(frames [][]byte, objectID int32) int {
	n := 0
	for _, f := range frames {
		if len(f) >= 9 && f[0] == serverpackets.OpcodePartySpelled &&
			binary.LittleEndian.Uint32(f[1:]) == uint32(serverpackets.PartySpelledPlayer) &&
			int32(binary.LittleEndian.Uint32(f[5:])) == objectID {
			n++
		}
	}
	return n
}

func opcodesOf(frames [][]byte) []byte {
	out := make([]byte, len(frames))
	for i, f := range frames {
		out[i] = f[0]
	}
	return out
}
