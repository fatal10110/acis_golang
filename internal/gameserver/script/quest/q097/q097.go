// Package q097 is the quest Saga of the Shillien Templar: the third class
// change of a Shillien Knight, on the saga core.
package q097

import (
	"github.com/fatal10110/acis_golang/internal/gameserver/script"
	"github.com/fatal10110/acis_golang/internal/gameserver/script/quest/thirdclass"
)

// shillienTemplar is the class the saga leads to.
const shillienTemplar = 106

// New returns the saga, registered in f.
func New(f *thirdclass.Family) script.Script { return thirdclass.New(f, Saga()) }

// Saga returns what the saga sets on the core.
func Saga() thirdclass.Saga {
	return thirdclass.Saga{
		Name:    "Q097_SagaOfTheShillienTemplar",
		QuestID: 97,
		Title:   "Saga of the Shillien Templar",
		ClassID: shillienTemplar,

		ItemMain:        7091,
		ItemOptional:    0,
		ItemReward:      7526,
		ItemAmulet1st:   7295,
		ItemAmulet2nd:   7326,
		ItemAmulet3rd:   7357,
		ItemAmulet4th:   7388,
		ItemHalishaMark: 7512,
		ItemAmulet5th:   7419,
		ItemAmulet6th:   7450,

		NPCMain:      31580,
		NPC1st:       31285,
		NPC2nd:       31623,
		NPCTablet1st: 31646,
		NPCTablet2nd: 31648,
		NPCTablet3rd: 31652,
		NPC3rd:       31285,
		NPCTablet4th: 31654,
		NPC4th:       31285,
		NPCTablet5th: 31655,
		NPCTablet6th: 31659,
		NPCDefender:  31610,

		MobGuardian:  27215,
		MobCorrupted: 27271,
		MobHalisha:   27246,
		MobAttacker:  27273,

		LocCorrupted: script.Loc{X: 161719, Y: -92823, Z: -1893, Heading: -1},
		LocAttacker:  script.Loc{X: 124355, Y: 82155, Z: -2803, Heading: -1},
		LocDefender:  script.Loc{X: 124376, Y: 82127, Z: -2796, Heading: -1},

		MsgCorruptedSpawn:      9750,
		MsgCorruptedDespawn:    9751,
		MsgCorruptedKill:       9752,
		MsgHalishaSpawn:        9753,
		MsgHalishaDespawn:      9756,
		MsgHalishaKill:         9754,
		MsgHalishaKillOther:    9755,
		MsgAttackerSpawn:       9764,
		MsgAttackerDespawn:     9765,
		MsgAttackerAttack1:     9766,
		MsgAttackerAttack16:    9767,
		MsgDefenderSpawn:       9757,
		MsgDefenderDespawnWon:  9761,
		MsgDefenderDespawnLost: 9762,
		MsgDefenderCombat:      9758,
		MsgDefenderCombatIdle1: 9759,
		MsgDefenderCombatIdle2: 9760,
		MsgDefenderReward:      9763,
	}
}
