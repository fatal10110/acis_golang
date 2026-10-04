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
// at a shop, gatekeeper or warehouse dialog, and the server.properties
// AllowWear gate on a merchant's try-on window.
type ChatRules struct {
	KarmaCanShop         bool
	KarmaCanUseGK        bool
	KarmaCanUseWarehouse bool
	AllowWear            bool
}

// ChatOutcome is what an interact with a civilian NPC opens.
type ChatOutcome int

const (
	// ChatShown opens the returned page.
	ChatShown ChatOutcome = iota
	// ChatUnported names an NPC type whose dialog reads state of a system
	// not in place yet (castles, clan halls, Seven Signs, Olympiad, manor);
	// it opens nothing.
	ChatUnported
	// ChatWedding is a wedding manager: its own dialog greets the talker.
	ChatWedding
	// ChatShownReleased releases the client with ActionFailed first, then
	// opens the returned page.
	ChatShownReleased
	// ChatDawnOnly refuses a talker who does not belong to the Dawn: the
	// winning cabal owning the seal the NPC serves. It says so, then
	// releases the client.
	ChatDawnOnly
	// ChatDuskOnly is ChatDawnOnly for the Dusk.
	ChatDuskOnly
	// ChatCompetitionOnly refuses every talker while no cabal leads: it says
	// the NPC serves only in the quest event period, and releases nothing.
	ChatCompetitionOnly
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
// castle, clan hall, Seven Signs, Olympiad or manor state.
var unportedFolkChats = map[InstanceKind]struct{}{
	"CastleBlacksmith":      {},
	"CastleChamberlain":     {},
	"CastleDoorman":         {},
	"CastleGatekeeper":      {},
	"CastleMagician":        {},
	"CastleWarehouseKeeper": {},
	"ClanHallDoorman":       {},
	"ClanHallManagerNpc":    {},
	"Doorman":               {},
	"ManorManagerNpc":       {},
	"MercenaryManagerNpc":   {},
	"SiegeNpc":              {},
	"WyvernManagerNpc":      {},
}

// unportedFolkCommands are the civilian types whose first chat page is in
// place but whose dialog commands still need festival behavior that is not
// (#223).
var unportedFolkCommands = map[InstanceKind]struct{}{
	"FestivalGuide": {},
}

// ChatWindow resolves the first chat page this NPC shows a talker carrying
// karma, or names the wedding manager, whose own dialog greets: its page with %objectId% filled in, or, for a karma-gated type
// whose gate refuses the talker and whose refusal page exists, that page
// as is. A Seven Signs priest, festival guide or Olympiad manager reads
// its page from state; see signsChat. A missing page reads as a "My html
// is missing" notice naming it.
func (f *Folk) ChatWindow(pages Pages, rules ChatRules, karma int, state ChatState) (string, ChatOutcome) {
	kind := hostileKind(f.Instance)
	if kind == weddingManager {
		return "", ChatWedding
	}
	if kind == auctioneer {
		return f.AuctioneerChat(pages), ChatShown
	}
	if _, ok := unportedFolkChats[kind]; ok {
		return "", ChatUnported
	}
	if page, outcome, ok := f.signsChat(pages, kind, state); ok {
		return page, outcome
	}
	chat := folkChats[kind]
	if page, refused := f.pkRefusal(pages, chat, rules, karma); refused {
		return page, ChatShown
	}
	return f.chatPage(pages, chat, 0), ChatShown
}

// auctioneer is the clan hall auctioneer's type.
const auctioneer InstanceKind = "Auctioneer"

// AuctioneerChat is an auctioneer's chat window: auction/auction.htm with
// %objectId%, %npcId% and %npcname% naming this NPC.
func (f *Folk) AuctioneerChat(pages Pages) string {
	page := f.page(pages, "data/html/auction/auction.htm")
	page = strings.ReplaceAll(page, "%npcId%", strconv.Itoa(f.NpcID()))
	return strings.ReplaceAll(page, "%npcname%", f.CharacterName())
}

// pkRefusal is the karma gate of chat's type: the refusal page
// <dir>/<npcId>-pk.htm, when the gate refuses a talker carrying karma and
// that page exists. The page is sent as is.
func (f *Folk) pkRefusal(pages Pages, chat folkChat, rules ChatRules, karma int) (string, bool) {
	if chat.pk == nil || karma <= 0 || chat.pk(rules) {
		return "", false
	}
	return pages.Get("data/html/" + chat.dir + "/" + strconv.Itoa(f.NpcID()) + "-pk.htm")
}

// chatPage resolves chat page val of this NPC: <npcId>.htm for 0, else
// <npcId>-<val>.htm, in its type's folder. A type without a folder reads
// data/html/default and falls back to npcdefault.htm when the page is not
// there.
func (f *Folk) chatPage(pages Pages, chat folkChat, val int) string {
	name := strconv.Itoa(f.NpcID())
	if val != 0 {
		name += "-" + strconv.Itoa(val)
	}
	path := "data/html/default/" + name + ".htm"
	switch {
	case chat.fixed != "":
		path = chat.fixed
	case chat.dir != "":
		path = "data/html/" + chat.dir + "/" + name + ".htm"
	default:
		if _, ok := pages.Get(path); !ok {
			path = "data/html/npcdefault.htm"
		}
	}
	return f.page(pages, path)
}

// page reads path with %objectId% naming this NPC; a missing page reads as
// a "My html is missing" notice naming it.
func (f *Folk) page(pages Pages, path string) string {
	page, ok := pages.Get(path)
	if !ok {
		page = "<html><body>My html is missing:<br>" + path + "</body></html>"
	}
	return strings.ReplaceAll(page, "%objectId%", strconv.Itoa(int(f.ObjectID())))
}
