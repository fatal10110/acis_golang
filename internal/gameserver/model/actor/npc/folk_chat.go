package npc

import (
	"strconv"
	"strings"
)

// Pages resolves a datapack HTML page by its data/html path.
type Pages interface {
	Get(path string) (string, bool)
}

// ChatRules are the players.properties gates a karma-carrying talker meets
// at a shop, gatekeeper or warehouse dialog.
type ChatRules struct {
	KarmaCanShop         bool
	KarmaCanUseGK        bool
	KarmaCanUseWarehouse bool
}

// ChatOutcome is what an interact with a civilian NPC opens.
type ChatOutcome int

const (
	// ChatShown opens the returned page.
	ChatShown ChatOutcome = iota
	// ChatUnported names an NPC type whose dialog reads state of a system
	// not in place yet (castles, clan halls, Seven Signs, Olympiad, manor,
	// weddings); it opens nothing.
	ChatUnported
)

// folkChat is one type's chat window: the data/html folder its pages live
// in (named <npcId>.htm), a fixed page instead, and the karma gate whose
// refusal page, <folder>/<npcId>-pk.htm, replaces it.
type folkChat struct {
	dir   string
	fixed string
	pk    func(ChatRules) bool
}

func canShop(r ChatRules) bool         { return r.KarmaCanShop }
func canUseGK(r ChatRules) bool        { return r.KarmaCanUseGK }
func canUseWarehouse(r ChatRules) bool { return r.KarmaCanUseWarehouse }

// folkChats maps each civilian type with a page-only chat window to where
// it reads it. A type absent here and from unportedFolkChats uses the
// default page.
var folkChats = map[InstanceKind]folkChat{
	"Adventurer":           {dir: "adventurer_guildsman"},
	"ClassMaster":          {dir: "mods/classmaster"},
	"DungeonGatekeeper":    {dir: "gatekeeper"},
	"Fisherman":            {dir: "fisherman", pk: canShop},
	"Gatekeeper":           {dir: "gatekeeper", pk: canUseGK},
	"Merchant":             {dir: "merchant", pk: canShop},
	"SchemeBuffer":         {dir: "mods/buffer"},
	"SymbolMaker":          {fixed: "data/html/symbolmaker/SymbolMaker.htm"},
	"Trainer":              {dir: "trainer"},
	"VillageMaster":        {dir: "villagemaster"},
	"VillageMasterDElf":    {dir: "villagemaster"},
	"VillageMasterDwarf":   {dir: "villagemaster"},
	"VillageMasterFighter": {dir: "villagemaster"},
	"VillageMasterMystic":  {dir: "villagemaster"},
	"VillageMasterOrc":     {dir: "villagemaster"},
	"VillageMasterPriest":  {dir: "villagemaster"},
	"WarehouseKeeper":      {dir: "warehouse", pk: canUseWarehouse},
}

// unportedFolkChats are the civilian types whose chat window depends on
// castle, clan hall, Seven Signs, Olympiad, manor or wedding state.
var unportedFolkChats = map[InstanceKind]struct{}{
	"Auctioneer":            {},
	"CastleBlacksmith":      {},
	"CastleChamberlain":     {},
	"CastleDoorman":         {},
	"CastleGatekeeper":      {},
	"CastleMagician":        {},
	"CastleWarehouseKeeper": {},
	"ClanHallDoorman":       {},
	"ClanHallManagerNpc":    {},
	"DawnPriest":            {},
	"Doorman":               {},
	"DuskPriest":            {},
	"FestivalGuide":         {},
	"ManorManagerNpc":       {},
	"MercenaryManagerNpc":   {},
	"OlympiadManagerNpc":    {},
	"SiegeNpc":              {},
	"SignsPriest":           {},
	"WeddingManagerNpc":     {},
	"WyvernManagerNpc":      {},
}

// ChatWindow resolves the first chat page this NPC shows a talker carrying
// karma: its page with %objectId% filled in, or, for a karma-gated type
// whose gate refuses the talker and whose refusal page exists, that page
// as is. A missing page reads as a "My html is missing" notice naming it.
func (f *Folk) ChatWindow(pages Pages, rules ChatRules, karma int) (string, ChatOutcome) {
	kind := hostileKind(f.Instance)
	if _, ok := unportedFolkChats[kind]; ok {
		return "", ChatUnported
	}
	id := strconv.Itoa(f.NpcID())
	chat, ok := folkChats[kind]
	if ok && chat.pk != nil && karma > 0 && !chat.pk(rules) {
		if page, ok := pages.Get("data/html/" + chat.dir + "/" + id + "-pk.htm"); ok {
			return page, ChatShown
		}
	}
	path := "data/html/default/" + id + ".htm"
	switch {
	case chat.fixed != "":
		path = chat.fixed
	case chat.dir != "":
		path = "data/html/" + chat.dir + "/" + id + ".htm"
	default:
		if _, ok := pages.Get(path); !ok {
			path = "data/html/npcdefault.htm"
		}
	}
	page, ok := pages.Get(path)
	if !ok {
		page = "<html><body>My html is missing:<br>" + path + "</body></html>"
	}
	return strings.ReplaceAll(page, "%objectId%", strconv.Itoa(int(f.ObjectID()))), ChatShown
}
