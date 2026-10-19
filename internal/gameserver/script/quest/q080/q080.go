// Package q080 is the quest Saga of the Wind Rider: the third class
// change of a Plains Walker, on the saga core.
package q080

import (
	"github.com/fatal10110/acis_golang/internal/gameserver/script"
	"github.com/fatal10110/acis_golang/internal/gameserver/script/quest/thirdclass"
)

// windRider is the class the saga leads to.
const windRider = 101

// New returns the saga, registered in f.
func New(f *thirdclass.Family) script.Script { return thirdclass.New(f, Saga()) }

// Saga returns what the saga sets on the core.
func Saga() thirdclass.Saga {
	return thirdclass.Saga{
		Name:    "Q080_SagaOfTheWindRider",
		QuestID: 80,
		Title:   "Saga of the Wind Rider",
		ClassID: windRider,

		ItemMain:        7103,
		ItemOptional:    0,
		ItemReward:      7517,
		ItemAmulet1st:   7278,
		ItemAmulet2nd:   7309,
		ItemAmulet3rd:   7340,
		ItemAmulet4th:   7371,
		ItemHalishaMark: 7495,
		ItemAmulet5th:   7402,
		ItemAmulet6th:   7433,

		NPCMain:      31603,
		NPC1st:       31284,
		NPC2nd:       31624,
		NPCTablet1st: 31646,
		NPCTablet2nd: 31648,
		NPCTablet3rd: 31652,
		NPC3rd:       31615,
		NPCTablet4th: 31654,
		NPC4th:       31616,
		NPCTablet5th: 31655,
		NPCTablet6th: 31659,
		NPCDefender:  31612,

		MobGuardian:  27215,
		MobCorrupted: 27300,
		MobHalisha:   27229,
		MobAttacker:  27303,

		LocCorrupted: script.Loc{X: 161719, Y: -92823, Z: -1893, Heading: -1},
		LocAttacker:  script.Loc{X: 124314, Y: 82155, Z: -2803, Heading: -1},
		LocDefender:  script.Loc{X: 124355, Y: 82155, Z: -2803, Heading: -1},

		MsgCorruptedSpawn:      8050,
		MsgCorruptedDespawn:    8051,
		MsgCorruptedKill:       8052,
		MsgHalishaSpawn:        8053,
		MsgHalishaDespawn:      8056,
		MsgHalishaKill:         8054,
		MsgHalishaKillOther:    8055,
		MsgAttackerSpawn:       8064,
		MsgAttackerDespawn:     8065,
		MsgAttackerAttack1:     8066,
		MsgAttackerAttack16:    8067,
		MsgDefenderSpawn:       8057,
		MsgDefenderDespawnWon:  8061,
		MsgDefenderDespawnLost: 8062,
		MsgDefenderCombat:      8058,
		MsgDefenderCombatIdle1: 8059,
		MsgDefenderCombatIdle2: 8060,
		MsgDefenderReward:      8063,
	}
}
