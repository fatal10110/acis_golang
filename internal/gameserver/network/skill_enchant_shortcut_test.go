package network

import (
	"context"
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/shortcut"
	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/clientpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	skillstate "github.com/fatal10110/acis_golang/internal/gameserver/skill"
	"github.com/fatal10110/acis_golang/internal/testsupport"
)

func TestApplyEnchantSkillRefreshesShortcuts(t *testing.T) {
	for _, tc := range []struct {
		name                                 string
		requested, current, rate, roll, want int
	}{
		{"success", 101, 1, 100, 0, 101},
		{"failure resets shortcut", 102, 101, 0, 1, 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			frames := &testsupport.FrameCapture{}
			live := newTestLivePlayer(t, 1, frames)
			live.Character.ClassID = 88
			live.Character.CharLevel = 76
			live.Character.SP = 1
			live.Character.Exp = 1
			live.Character.SetSkillLevel(1, tc.current)
			live.shortcuts = shortcut.NewList([]shortcut.Shortcut{{
				Slot: 0, Page: 0, Type: shortcut.Skill, ID: 1, Level: int32(tc.current), CharacterType: 1,
			}})
			gcl := &GameClientLink{
				levels: levelRefreshTable(t),
				skills: skillstate.NewPersistence(nil, skillTable(
					modelskill.Definition{ID: 1, Level: 1},
					modelskill.Definition{ID: 1, Level: 101},
					modelskill.Definition{ID: 1, Level: 102},
				)),
				skillTrees:       &modelskill.Trees{Enchant: []modelskill.EnchantSkill{{ID: 1, Level: tc.requested, Rate76: tc.rate}}},
				skillEnchantRoll: func() int { return tc.roll },
			}

			gcl.applyEnchantSkill(context.Background(), live, clientpackets.RequestExEnchantSkill{SkillID: 1, SkillLevel: int32(tc.requested)})

			if got := live.shortcuts.All()[0].Level; got != int32(tc.want) {
				t.Fatalf("shortcut level = %d, want %d", got, tc.want)
			}
			registerAt, messageAt := -1, -1
			for i, frame := range frames.Frames() {
				if frame[0] == serverpackets.OpcodeShortCutRegister {
					registerAt = i
				}
				if frame[0] == serverpackets.OpcodeSystemMessage {
					messageAt = i
				}
			}
			if registerAt < 0 {
				t.Fatal("ShortCutRegister not sent after an enchant attempt")
			}
			if messageAt < 0 || registerAt > messageAt {
				t.Fatalf("shortcut register index = %d, system message index = %d; want shortcut first", registerAt, messageAt)
			}
		})
	}
}
