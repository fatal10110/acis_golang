package quest

import "github.com/fatal10110/acis_golang/internal/gameserver/script/quest/q610"

// q610Raid is the Varka's holy altar, which calls Soul of Water Ashutar for
// the Green Totem. Its tests are those of the other altars, in
// q604_q625_raid_call_test.go.
var q610Raid = raidQuest{
	name: "Q610_MagicalPowerOfWater_Part2", path: "quest.Q610_MagicalPowerOfWater_Part2", newQuest: q610.New,
	eye: 31372, altar: 31560, raid: 25316, summon: 7238, trophy: 7239, called: 61050, left: 61051,
	at: [3]int{104825, -36926, -1136},
}

// raidQuests are the quests whose altar calls a raid.
var raidQuests = []raidQuest{q604Raid, q625Raid, q610Raid, q616Raid}
