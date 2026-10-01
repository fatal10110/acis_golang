package npc

import "strconv"

// merchantKinds are the civilian types built on the merchant: a buylist
// purchase or try-on may target any of them.
var merchantKinds = map[InstanceKind]struct{}{
	"Merchant":           {},
	"Fisherman":          {},
	"CastleChamberlain":  {},
	"ClanHallManagerNpc": {},
	"ManorManagerNpc":    {},
}

// Merchant reports whether f is a merchant type, one a buylist purchase or
// try-on may target.
func (f *Folk) Merchant() bool {
	_, ok := merchantKinds[hostileKind(f.Instance)]
	return ok
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
