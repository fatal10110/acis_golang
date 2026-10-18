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
	// AllowManor is server.properties AllowManor: while it is off, the
	// castle blacksmiths and manor managers show the generic page.
	AllowManor bool
	// Wyvern is the npcs.properties wyvern ride settings the wyvern pages
	// name.
	Wyvern WyvernRules
}

// WyvernRules are the npcs.properties wyvern ride settings: the strider
// level RequiredStriderLevel and the B-grade crystal count
// RequiredCrystalsNumber a ride asks for.
type WyvernRules struct {
	StriderLevel int
	Crystals     int
}

// DefaultWyvernRules are the wyvern ride settings npcs.properties reads
// when it names none: a level 55 strider and 10 crystals.
func DefaultWyvernRules() WyvernRules { return WyvernRules{StriderLevel: 55, Crystals: 10} }

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
	// ChatPageOnly opens the returned page and releases nothing.
	ChatPageOnly
	// ChatReleasedOnly opens nothing and releases the client.
	ChatReleasedOnly
	// ChatSilent opens nothing and sends nothing.
	ChatSilent
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
	"ManorManagerNpc":      {dir: "merchant", fixed: "data/html/manormanager/manager.htm", pk: canShop},
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
	// A castle warehouse keeper keeps the warehouse keeper's karma gate;
	// its chat pages are its castle's, see residenceChatPage.
	"CastleWarehouseKeeper": {dir: "warehouse", pk: canUseWarehouse},
	// A chamberlain and a wyvern manager answer the commands their own
	// dialogs leave as a merchant does; their first chat page is their
	// residence's, see residenceChat.
	"CastleChamberlain": {dir: "merchant", pk: canShop},
	"WyvernManagerNpc":  {dir: "merchant", pk: canShop},
}

// unportedFolkChats are the civilian types whose chat window depends on
// state not in place yet: a siegable hall messenger's on the hall's siege
// (#244); a castle's messenger is answered before the chat window.
var unportedFolkChats = map[InstanceKind]struct{}{
	"SiegeNpc": {},
}

// ChatWindow resolves the first chat page this NPC shows a talker carrying
// karma, or names the wedding manager, whose own dialog greets: its page with %objectId% filled in, or, for a karma-gated type
// whose gate refuses the talker and whose refusal page exists, that page
// as is. A Seven Signs priest, festival guide or Olympiad manager reads
// its page from state, as does an NPC serving a castle or a clan hall; see
// signsChat and residenceChat. A missing page reads as a "My html
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
	if page, outcome, ok := f.residenceChat(pages, kind, rules, state); ok {
		return page, outcome
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
