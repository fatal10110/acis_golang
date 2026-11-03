package quest

import "github.com/fatal10110/acis_golang/internal/gameserver/script/quest/q616"

// q616Raid is the Ketra's holy altar, which calls Soul of Fire Nastron for
// the Red Totem. Its tests are those of the other altars, in
// q604_q625_raid_call_test.go.
var q616Raid = raidQuest{
	name: "Q616_MagicalPowerOfFire_Part2", path: "quest.Q616_MagicalPowerOfFire_Part2", newQuest: q616.New,
	eye: 31379, altar: 31558, raid: 25306, summon: 7243, trophy: 7244, called: 61650, left: 61651,
	at: [3]int{142528, -82528, -6496},
}
