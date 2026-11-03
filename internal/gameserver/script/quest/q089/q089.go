// Package q089 is the quest Saga of the Mystic Muse: the third class
// change of a Spellsinger, on the saga core.
package q089

import (
	"github.com/fatal10110/acis_golang/internal/gameserver/script"
	"github.com/fatal10110/acis_golang/internal/gameserver/script/quest/thirdclass"
)

// mysticMuse is the class the saga leads to.
const mysticMuse = 103

// New returns the saga, registered in f.
func New(f *thirdclass.Family) script.Script { return thirdclass.New(f, Saga()) }

// Saga returns what the saga sets on the core.
func Saga() thirdclass.Saga {
	return thirdclass.Saga{
		Name:    "Q089_SagaOfTheMysticMuse",
		QuestID: 89,
		Title:   "Saga of the Mystic Muse",
		ClassID: mysticMuse,

		ItemMain:        7083,
		ItemOptional:    0,
		ItemReward:      7530,
		ItemAmulet1st:   7287,
		ItemAmulet2nd:   7318,
		ItemAmulet3rd:   7349,
		ItemAmulet4th:   7380,
		ItemHalishaMark: 7504,
		ItemAmulet5th:   7411,
		ItemAmulet6th:   7442,

		NPCMain:      30174,
		NPC1st:       31283,
		NPC2nd:       31627,
		NPCTablet1st: 31646,
		NPCTablet2nd: 31648,
		NPCTablet3rd: 31651,
		NPC3rd:       31283,
		NPCTablet4th: 31654,
		NPC4th:       31283,
		NPCTablet5th: 31655,
		NPCTablet6th: 31658,
		NPCDefender:  31643,

		MobGuardian:  27215,
		MobCorrupted: 27251,
		MobHalisha:   27238,
		MobAttacker:  27255,

		LocCorrupted: script.Loc{X: 119518, Y: -28658, Z: -3811, Heading: -1},
		LocAttacker:  script.Loc{X: 181227, Y: 36703, Z: -4816, Heading: -1},
		LocDefender:  script.Loc{X: 181215, Y: 36676, Z: -4812, Heading: -1},

		MsgCorruptedSpawn:      8950,
		MsgCorruptedDespawn:    8951,
		MsgCorruptedKill:       8952,
		MsgHalishaSpawn:        8953,
		MsgHalishaDespawn:      8956,
		MsgHalishaKill:         8954,
		MsgHalishaKillOther:    8955,
		MsgAttackerSpawn:       8964,
		MsgAttackerDespawn:     8965,
		MsgAttackerAttack1:     8966,
		MsgAttackerAttack16:    8967,
		MsgDefenderSpawn:       8957,
		MsgDefenderDespawnWon:  8961,
		MsgDefenderDespawnLost: 8962,
		MsgDefenderCombat:      8958,
		MsgDefenderCombatIdle1: 8959,
		MsgDefenderCombatIdle2: 8960,
		MsgDefenderReward:      8963,
	}
}
