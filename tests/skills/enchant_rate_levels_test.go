package skills

import (
	"fmt"
	"testing"

	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
)

// TestEnchantRateFollowsTheCharacterLevel gives the enchant node a distinct
// success rate per character-level column (76 through 80) and walks a
// character of each level 77..80: the trainer's quote carries that level's
// rate, and the roll succeeds on the rate itself and fails one above it.
func TestEnchantRateFollowsTheCharacterLevel(t *testing.T) {
	t.Parallel()
	tree := func() *modelskill.Trees {
		return &modelskill.Trees{Enchant: []modelskill.EnchantSkill{{
			ID: 1, Level: 101, Rate76: 11, Rate77: 22, Rate78: 33, Rate79: 44, Rate80: 55,
		}}}
	}
	rates := map[int]int{76: 11, 77: 22, 78: 33, 79: 44, 80: 55}
	for charLevel := 76; charLevel <= 80; charLevel++ {
		rate := rates[charLevel]
		for _, tc := range []struct {
			name    string
			roll    int
			applied int
		}{
			{"roll on the rate succeeds", rate, 101},
			{"roll above the rate fails", rate + 1, 1},
		} {
			t.Run(fmt.Sprintf("level %d %s", charLevel, tc.name), func(t *testing.T) {
				t.Parallel()
				roll := tc.roll
				srv, c, objID := seedEnchanterAt(t, charLevel, 1,
					gameservertest.WithSkillTrees(tree()),
					gameservertest.WithSkillEnchantRoll(func() int { return roll }),
				)
				startInWorld(t, c)
				selectTrainer(t, srv, c, objID, duelistParentClass)

				c.Send(encodeRequestExEnchantSkillInfo(1, 101))
				frame := c.Read()
				assertFrameOpcode(t, frame, serverpackets.OpcodeExtended, "ExEnchantSkillInfo")
				r := wireReader(frame[1:])
				r.ReadUint16()
				r.ReadInt32()
				r.ReadInt32()
				r.ReadInt32()
				r.ReadInt64()
				if got := int(r.ReadInt32()); got != rate {
					t.Fatalf("quoted enchant rate at character level %d = %d, want %d", charLevel, got, rate)
				}

				c.Send(encodeRequestExEnchantSkill(1, 101))
				drainUntilQuiet(t, c)
				assertKnownSkills(t, srv, objID, map[int]int{1: tc.applied})
			})
		}
	}
}
