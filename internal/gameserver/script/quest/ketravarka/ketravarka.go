// Package ketravarka holds the quests of the war between the Ketra Orcs and
// the Varka Silenos. An ally of either tribe raises its alliance level with
// badges of the other tribe's warriors, brings the heads of the other tribe's
// commanders and steals the other tribe's totem; the Ketra and the Varka
// quest of each kind share their mechanics and differ in the tribe. The
// quests are one family in one package, whose literals and calls are checked
// as the union of its quests (#4712); each quest keeps its own dialogue in
// its own file.
package ketravarka

import "github.com/fatal10110/acis_golang/internal/gameserver/script"

// NPCs.
const (
	kadun  = 31370 // Kadun Zu Ketra
	wahkan = 31371
	asefa  = 31372
	ashas  = 31377 // Ashas Varka Durai
	naran  = 31378 // Naran Ashanuk
	udan   = 31379
)

// The war quests, whose trophies the alliance quests drop.
const (
	q606Name = "Q606_WarWithVarkaSilenos"
	q612Name = "Q612_WarWithKetraOrcs"
)

// Items.
const (
	ketraAlliance1 = 7211
	ketraAlliance2 = 7212
	ketraAlliance3 = 7213
	ketraAlliance4 = 7214
	ketraAlliance5 = 7215

	varkaBadgeSoldier = 7216
	varkaBadgeOfficer = 7217
	varkaBadgeCaptain = 7218

	totemOfValor  = 7219
	totemOfWisdom = 7220

	varkaAlliance1 = 7221
	varkaAlliance2 = 7222
	varkaAlliance3 = 7223
	varkaAlliance4 = 7224
	varkaAlliance5 = 7225

	ketraBadgeSoldier = 7226
	ketraBadgeOfficer = 7227
	ketraBadgeCaptain = 7228

	featherOfValor  = 7229
	featherOfWisdom = 7230

	varkaMane  = 7233
	ketraMolar = 7234
)

// tribe is the side a quest's taker is allied with.
type tribe struct {
	// sign is the sign of the alliance level of the tribe's allies.
	sign int32
	// marks are the tribe's alliance marks, from level 1 to level 5.
	marks [5]int32
}

func ketra() tribe {
	return tribe{sign: 1, marks: [5]int32{ketraAlliance1, ketraAlliance2, ketraAlliance3, ketraAlliance4, ketraAlliance5}}
}

func varka() tribe {
	return tribe{sign: -1, marks: [5]int32{varkaAlliance1, varkaAlliance2, varkaAlliance3, varkaAlliance4, varkaAlliance5}}
}

// allied reports whether p is an ally of the tribe at alliance level level
// or more.
func (t tribe) allied(p *script.Player, level int32) bool {
	return t.sign*p.VarkaKetraAlliance() >= level
}
