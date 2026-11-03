// Package q071 is the quest Saga of Eva's Templar: the third class
// change of a Temple Knight, on the saga core.
package q071

import (
	"github.com/fatal10110/acis_golang/internal/gameserver/script"
	"github.com/fatal10110/acis_golang/internal/gameserver/script/quest/thirdclass"
)

// evasTemplar is the class the saga leads to.
const evasTemplar = 99

// New returns the saga, registered in f.
func New(f *thirdclass.Family) script.Script { return thirdclass.New(f, Saga()) }

// Saga returns what the saga sets on the core.
func Saga() thirdclass.Saga {
	return thirdclass.Saga{
		Name:    "Q071_SagaOfEvasTemplar",
		QuestID: 71,
		Title:   "Saga of Eva's Templar",
		ClassID: evasTemplar,

		ItemMain:        7094,
		ItemOptional:    6482,
		ItemReward:      7535,
		ItemAmulet1st:   7269,
		ItemAmulet2nd:   7300,
		ItemAmulet3rd:   7331,
		ItemAmulet4th:   7362,
		ItemHalishaMark: 7486,
		ItemAmulet5th:   7393,
		ItemAmulet6th:   7424,

		NPCMain:      30852,
		NPC1st:       31278,
		NPC2nd:       31624,
		NPCTablet1st: 31646,
		NPCTablet2nd: 31648,
		NPCTablet3rd: 31651,
		NPC3rd:       30852,
		NPCTablet4th: 31654,
		NPC4th:       31281,
		NPCTablet5th: 31655,
		NPCTablet6th: 31658,
		NPCDefender:  31638,

		MobGuardian:  27215,
		MobCorrupted: 27287,
		MobHalisha:   27220,
		MobAttacker:  27279,

		LocCorrupted: script.Loc{X: 119518, Y: -28658, Z: -3811, Heading: -1},
		LocAttacker:  script.Loc{X: 181215, Y: 36676, Z: -4812, Heading: -1},
		LocDefender:  script.Loc{X: 181227, Y: 36703, Z: -4816, Heading: -1},

		MsgCorruptedSpawn:      7150,
		MsgCorruptedDespawn:    7151,
		MsgCorruptedKill:       7152,
		MsgHalishaSpawn:        7153,
		MsgHalishaDespawn:      7156,
		MsgHalishaKill:         7154,
		MsgHalishaKillOther:    7155,
		MsgAttackerSpawn:       7164,
		MsgAttackerDespawn:     7165,
		MsgAttackerAttack1:     7166,
		MsgAttackerAttack16:    7167,
		MsgDefenderSpawn:       7157,
		MsgDefenderDespawnWon:  7161,
		MsgDefenderDespawnLost: 7162,
		MsgDefenderCombat:      7158,
		MsgDefenderCombatIdle1: 7159,
		MsgDefenderCombatIdle2: 7160,
		MsgDefenderReward:      7163,
	}
}
