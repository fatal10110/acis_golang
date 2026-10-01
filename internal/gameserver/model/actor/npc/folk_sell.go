package npc

import (
	"strconv"
	"strings"
)

// soldPageDirs are the page folders of the types that buy items from
// players and thank them with a <npcId>-sold.htm page: every merchant-type
// NPC. A mercenary manager buys too but has no such page.
var soldPageDirs = map[InstanceKind]string{
	"Merchant":           "merchant",
	"Fisherman":          "fisherman",
	"CastleChamberlain":  "merchant",
	"ManorManagerNpc":    "merchant",
	"ClanHallManagerNpc": "merchant",
}

// BuysItems reports whether players may sell items to f: a merchant-type
// NPC or a mercenary manager.
func (f *Folk) BuysItems() bool {
	kind := hostileKind(f.Instance)
	_, ok := soldPageDirs[kind]
	return ok || kind == "MercenaryManagerNpc"
}

// SoldPage is the page f shows a player who just sold it items, with
// %objectId% naming f, when f's type and id have one.
func (f *Folk) SoldPage(pages Pages) (string, bool) {
	dir, ok := soldPageDirs[hostileKind(f.Instance)]
	if !ok {
		return "", false
	}
	page, ok := pages.Get("data/html/" + dir + "/" + strconv.Itoa(f.NpcID()) + "-sold.htm")
	if !ok {
		return "", false
	}
	return strings.ReplaceAll(page, "%objectId%", strconv.Itoa(int(f.ObjectID()))), true
}

// firstToken is command's first space-separated word.
func firstToken(command string) string {
	command = strings.TrimLeft(command, " ")
	if i := strings.IndexByte(command, ' '); i >= 0 {
		return command[:i]
	}
	return command
}
