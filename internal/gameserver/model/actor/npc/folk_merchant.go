package npc

import (
	"strconv"

	"github.com/fatal10110/acis_golang/internal/gameserver/clan"
)

// merchantKinds are the civilian types built on the merchant: a buylist
// purchase or try-on may target any of them.
var merchantKinds = map[InstanceKind]struct{}{
	"Merchant":           {},
	"Fisherman":          {},
	"CastleChamberlain":  {},
	"ClanHallManagerNpc": {},
	"ManorManagerNpc":    {},
	"WyvernManagerNpc":   {},
}

// Merchant reports whether f is a merchant type, one a buylist purchase or
// try-on may target.
func (f *Folk) Merchant() bool {
	_, ok := merchantKinds[hostileKind(f.Instance)]
	return ok
}

// BuyRefused reports whether f's dialog, the one opening its buy lists,
// refuses talker r now, so that a purchase sent without it is refused as
// well (#5148): a castle chamberlain refuses whom its products command
// refuses, a mercenary manager whom its hire command does. Any other seller
// refuses no one here.
func (f *Folk) BuyRefused(r ResidenceTalk) bool {
	switch kind := hostileKind(f.Instance); kind {
	case "CastleChamberlain":
		return r.ChamberlainRefusal(kind, clan.PrivCastleFunctions) != ""
	case "MercenaryManagerNpc":
		return r.castleTalkCond(r.SiegeInProgress, clan.PrivCastleMercenary) != talkOwner || !r.SealValidation
	}
	return false
}

// MercenaryManager reports whether f is a castle mercenary manager, which
// a buylist purchase may also target.
func (f *Folk) MercenaryManager() bool {
	return hostileKind(f.Instance) == "MercenaryManagerNpc"
}

// BoughtPage is the page f shows after a purchase, data/html/<dir>/<npcId>-bought.htm
// in its fisherman or merchant folder, and whether f has such a folder: a
// mercenary manager has none.
func (f *Folk) BoughtPage() (string, bool) {
	dir := "merchant"
	switch {
	case hostileKind(f.Instance) == "Fisherman":
		dir = "fisherman"
	case !f.Merchant():
		return "", false
	}
	return "data/html/" + dir + "/" + strconv.Itoa(f.NpcID()) + "-bought.htm", true
}
