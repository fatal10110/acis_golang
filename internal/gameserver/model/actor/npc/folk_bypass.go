package npc

import (
	"strconv"
	"strings"
)

// BypassOutcome is what a civilian NPC's dialog command answers with.
type BypassOutcome int

const (
	// BypassUnported names a command of a system not in place yet (skill
	// lists, quests, shops, warehouses, teleports, lottery, observation,
	// castles and the like), or one no dialog handles. It answers nothing
	// of its own.
	BypassUnported BypassOutcome = iota
	// BypassChatWindow opens HTML, then releases the client with
	// ActionFailed.
	BypassChatWindow
	// BypassPage opens HTML alone.
	BypassPage
	// BypassRefused answers nothing: a Link naming a path that climbs out of
	// the page tree, or a shop command naming no buylist.
	BypassRefused
	// BypassAborted is a command so malformed its handling stops outright:
	// nothing is sent, not even the dispatcher's closing ActionFailed.
	BypassAborted
	// BypassSellList opens the merchant's sell window on the talker's
	// sellable items. With none to offer, HTML, when set, is shown instead.
	BypassSellList
	// BypassBuyList opens the buy window of buylist ListID.
	BypassBuyList
	// BypassWearList opens the try-on window of buylist ListID.
	BypassWearList
	// BypassHennaDraw opens the symbol maker's draw window.
	BypassHennaDraw
	// BypassHennaRemoveList opens the symbol maker's deletion window, or
	// says the talker wears no symbol.
	BypassHennaRemoveList
)

// BypassReply is a civilian NPC's answer to one dialog command.
type BypassReply struct {
	Outcome BypassOutcome
	// HTML is the page BypassChatWindow and BypassPage open, or the page
	// BypassSellList shows when there is nothing to sell.
	HTML string
	// LeadingActionFailed releases the client before anything else: a
	// dungeon gatekeeper does so for every command.
	LeadingActionFailed bool
	// CancelEnchant drops the talker's enchant scroll selection before the
	// command runs: a warehouse keeper does so for every command its karma
	// gate lets through. Its refusal of a talker tied up in a trade needs
	// no field: the interact gate ahead of every dialog command already
	// refuses one.
	CancelEnchant bool
	// ListID is the buylist BypassBuyList and BypassWearList name.
	ListID int
}

// fishermanCommands are the fisherman's own commands, run before the
// merchant karma gate it inherits.
var fishermanCommands = []string{"FishSkillList", "FishingChampionship", "FishingReward"}

// Bypass answers command, the part of an npc_<objectId>_<command> dialog
// link after the object id, sent by a talker carrying karma. A shop,
// fisherman, gatekeeper or warehouse keeper first applies its karma gate
// to every command, answering with its refusal page when that page exists.
// A symbol maker's Draw and RemoveList open its windows. The generic
// commands follow: Chat <n> opens chat page n (page 0 when n
// does not parse), Link <path> opens data/html/<path>. Every other command
// belongs to a system not in place yet.
func (f *Folk) Bypass(pages Pages, rules ChatRules, karma int, command string) BypassReply {
	kind := hostileKind(f.Instance)
	reply := BypassReply{LeadingActionFailed: kind == "DungeonGatekeeper"}
	if _, ok := unportedFolkChats[kind]; ok {
		return reply
	}
	chat := folkChats[kind]
	if page, refused := f.pkRefusal(pages, chat, rules, karma); refused {
		reply.Outcome, reply.HTML = BypassChatWindow, page
		return reply
	}
	if kind == "Fisherman" {
		for _, own := range fishermanCommands {
			if strings.HasPrefix(command, own) {
				return reply
			}
		}
		if page, refused := f.pkRefusal(pages, folkChats["Merchant"], rules, karma); refused {
			reply.Outcome, reply.HTML = BypassChatWindow, page
			return reply
		}
	}
	// A merchant or fisherman keeps its sale's empty page in its own folder.
	if (kind == "Merchant" || kind == "Fisherman") && strings.EqualFold(firstToken(command), "Sell") {
		reply.Outcome = BypassSellList
		if page, ok := pages.Get("data/html/" + chat.dir + "/" + strconv.Itoa(f.NpcID()) + "-empty.htm"); ok {
			reply.HTML = strings.ReplaceAll(page, "%objectId%", strconv.Itoa(int(f.ObjectID())))
		}
		return reply
	}
	if kind == "SymbolMaker" {
		switch command {
		case "Draw":
			reply.Outcome = BypassHennaDraw
			return reply
		case "RemoveList":
			reply.Outcome = BypassHennaRemoveList
			return reply
		}
	}
	reply.CancelEnchant = kind == "WarehouseKeeper"
	if kind == "Merchant" || kind == "Fisherman" {
		if shop, ok := merchantCommand(reply, rules, command); ok {
			return shop
		}
	}
	switch {
	case strings.HasPrefix(command, "SkillList"), strings.HasPrefix(command, "EnchantSkillList"),
		strings.EqualFold(command, "TerritoryStatus"), strings.HasPrefix(command, "Quest"):
		// The skill lists belong to the trainer flow, TerritoryStatus to
		// castles, and Quest to the quest engine (#130): each is checked
		// ahead of Chat, so none falls through to it.
		return reply
	case strings.HasPrefix(command, "Chat"):
		val := 0
		if len(command) >= 5 {
			if n, err := strconv.ParseInt(command[5:], 10, 32); err == nil {
				val = int(n)
			}
		}
		reply.Outcome, reply.HTML = BypassChatWindow, f.chatPage(pages, chat, val)
		return reply
	case strings.HasPrefix(command, "Link"):
		if len(command) < 5 {
			reply.Outcome = BypassAborted
			return reply
		}
		path := strings.TrimFunc(command[5:], func(r rune) bool { return r <= ' ' })
		if strings.Contains(path, "..") {
			reply.Outcome = BypassRefused
			return reply
		}
		reply.Outcome, reply.HTML = BypassPage, f.page(pages, "data/html/"+path)
		return reply
	}
	return reply
}

// merchantCommand answers a merchant's shop commands, split on spaces with
// the command name matched in any case: Buy <listId> opens a buy window
// and, while the server allows it, Wear <listId> a try-on window. A shop
// command naming no list answers nothing of its own, and one whose list id
// does not parse aborts. It reports false for any other command.
func merchantCommand(reply BypassReply, rules ChatRules, command string) (BypassReply, bool) {
	tokens := strings.FieldsFunc(command, func(r rune) bool { return r == ' ' })
	if len(tokens) == 0 {
		return reply, false
	}
	switch {
	case strings.EqualFold(tokens[0], "Buy"):
		reply.Outcome = BypassBuyList
	case strings.EqualFold(tokens[0], "Wear") && rules.AllowWear:
		reply.Outcome = BypassWearList
	default:
		return reply, false
	}
	if len(tokens) < 2 {
		reply.Outcome = BypassRefused
		return reply, true
	}
	id, err := strconv.ParseInt(tokens[1], 10, 32)
	if err != nil {
		reply.Outcome = BypassAborted
		return reply, true
	}
	reply.ListID = int(id)
	return reply, true
}
