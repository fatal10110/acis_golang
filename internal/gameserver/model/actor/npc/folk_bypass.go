package npc

import (
	"strconv"
	"strings"
	"unicode/utf16"

	"github.com/fatal10110/acis_golang/internal/commons"
	"github.com/fatal10110/acis_golang/internal/gameserver/schemebuffer"
)

// BypassOutcome is what a civilian NPC's dialog command answers with.
type BypassOutcome int

const (
	// BypassUnported names a command of a system not in place yet (quests,
	// observation, castles and the like), or one no dialog handles. It
	// answers nothing of its own.
	BypassUnported BypassOutcome = iota
	// BypassChatWindow opens HTML, then releases the client with
	// ActionFailed.
	BypassChatWindow
	// BypassPage opens HTML alone.
	BypassPage
	// BypassRefused answers nothing: a Link naming a path that climbs out of
	// the page tree, a shop command naming no buylist, or a merchant's
	// multisell command naming no list.
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
	// BypassMultisell opens the multisell list Multisell names, in its
	// inventory-only form when InventoryOnly is set.
	BypassMultisell
	// BypassWarehouse runs the warehouse keeper's storage command
	// Warehouse.
	BypassWarehouse
	// BypassSkillList opens this trainer's list of skills to learn.
	BypassSkillList
	// BypassEnchantSkillList opens this trainer's list of skills to enchant.
	BypassEnchantSkillList
	// BypassFishSkillList opens the fishing skills list.
	BypassFishSkillList
	// BypassAugmentMake opens the augmentation window.
	BypassAugmentMake
	// BypassAugmentCancel opens the augmentation removal window.
	BypassAugmentCancel
	// BypassReleased answers ActionFailed of the command's own, ahead of
	// the dispatcher's.
	BypassReleased
	// BypassTeleportList opens the list of this NPC's standard
	// destinations.
	BypassTeleportList
	// BypassTeleport takes the talker to destination Index of this NPC's
	// list.
	BypassTeleport
	// BypassInstantTeleport takes the talker to instant destination Index
	// of this NPC's list.
	BypassInstantTeleport
	// BypassQuestInfo opens the client's quest information window.
	BypassQuestInfo
	// BypassSubclass runs a village master's subclass command; see
	// ParseSubclassCommand.
	BypassSubclass
	// BypassCPRecovery runs an arena manager's paid CP restore; see
	// Folk.CPRecovery.
	BypassCPRecovery
	// BypassClan runs a village master's clan command; see
	// VillageMasterClanCommand.
	BypassClan
	// BypassSchemeBuffer runs a scheme buffer's own command; see
	// schemebuffer.Command.
	BypassSchemeBuffer
	// BypassLottery runs a lottery seller's "Loto <n>" command; see
	// lottery.Command.
	BypassLottery
)

// Talker is what a dialog command reads of the player sending it.
type Talker struct {
	Karma int
	Level int
	// LowLevelNewbie is a level 6 to 25 player who has made at most the
	// first occupation change.
	LowLevelNewbie bool
}

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
	// Multisell is the list name BypassMultisell opens.
	Multisell string
	// InventoryOnly opens the list on the talker's unworn armor and
	// weapons: only the entries taking one of them, one set per item.
	InventoryOnly bool
	// Warehouse is the storage command BypassWarehouse runs, and
	// FreightTarget the text after the command's last '_', which names the
	// character a FreightCharacter command opens.
	Warehouse     WarehouseCommand
	FreightTarget string
	// Index is the destination BypassTeleport and BypassInstantTeleport
	// name.
	Index int
}

// fishermanCommands are the fisherman's championship commands, run before
// the merchant karma gate it inherits; the championship is not in place
// yet.
var fishermanCommands = []string{"FishingChampionship", "FishingReward"}

// Bypass answers command, the part of an npc_<objectId>_<command> dialog
// link after the object id, sent by talker. A shop, fisherman, gatekeeper
// or warehouse keeper first applies its karma gate to every command,
// answering with its refusal page when that page exists. A fisherman's
// FishSkillList opens the fishing skills list. A symbol maker's Draw and
// RemoveList open its windows. A merchant or fisherman then answers its
// sell, multisell and shop commands, and a warehouse keeper its storage
// commands. The trainer commands follow: SkillList and EnchantSkillList
// open the skills to learn and to enchant, after an adventurer guildsman's
// raidInfo and questlist. Then the generic ones: Chat <n> opens chat page n
// (page 0 when n does not parse), Link <path> opens data/html/<path>,
// Loto <n> runs the lottery dialog, multisell <list> and exc_multisell
// <list> open a multisell list, Augment 1 and Augment 2 open the
// augmentation and removal windows,
// teleport_request opens the destination list, and teleport <index> and
// instant_teleport <index> take the talker to a destination, and
// CPRecovery has an arena manager restore the talker's CP for a fee and
// answers nothing at any other NPC. A village master's Subclass commands go
// to the subclass dialog, and a scheme buffer's own commands to its
// dialog. Every other command belongs to a system not in place yet.
func (f *Folk) Bypass(pages Pages, rules ChatRules, talker Talker, command string) BypassReply {
	karma := talker.Karma
	kind := hostileKind(f.Instance)
	reply := BypassReply{LeadingActionFailed: kind == "DungeonGatekeeper"}
	if _, ok := unportedFolkChats[kind]; ok {
		return reply
	}
	if f.VillageMaster() && strings.HasPrefix(command, "Subclass") {
		reply.Outcome = BypassSubclass
		return reply
	}
	if f.VillageMaster() && VillageMasterClanCommand(command) {
		reply.Outcome = BypassClan
		return reply
	}
	if kind == "SchemeBuffer" && schemebuffer.Command(command) {
		reply.Outcome = BypassSchemeBuffer
		return reply
	}
	chat := folkChats[kind]
	if page, refused := f.pkRefusal(pages, chat, rules, karma); refused {
		reply.Outcome, reply.HTML = BypassChatWindow, page
		return reply
	}
	if kind == "Fisherman" {
		if strings.HasPrefix(command, "FishSkillList") {
			reply.Outcome = BypassFishSkillList
			return reply
		}
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
	if reply.CancelEnchant {
		if cmd := warehouseCommand(command); cmd != WarehouseNoCommand {
			reply.Outcome, reply.Warehouse = BypassWarehouse, cmd
			reply.FreightTarget = command[strings.LastIndexByte(command, '_')+1:]
			return reply
		}
	}
	if kind == "Merchant" || kind == "Fisherman" {
		if out, ok := f.merchantMultisell(pages, talker, command, reply); ok {
			return out
		}
		if shop, ok := merchantCommand(reply, rules, command); ok {
			return shop
		}
	}
	if kind == "Adventurer" {
		if out, ok := f.adventurerCommand(pages, command, reply); ok {
			return out
		}
	}
	switch {
	case strings.HasPrefix(command, "SkillList"):
		reply.Outcome = BypassSkillList
		return reply
	case strings.HasPrefix(command, "EnchantSkillList"):
		reply.Outcome = BypassEnchantSkillList
		return reply
	case strings.EqualFold(command, "TerritoryStatus"), strings.HasPrefix(command, "Quest"):
		// TerritoryStatus belongs to castles and Quest to the quest engine
		// (#130): each is checked ahead of Chat, so neither falls through
		// to it.
		return reply
	case strings.HasPrefix(command, "Chat"):
		val := 0
		if arg, ok := commandChars(command, 5, -1); ok {
			if n, err := commons.ParseInt(arg, 32); err == nil {
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
	case strings.HasPrefix(command, "Loto"):
		reply.Outcome = BypassLottery
		return reply
	case strings.HasPrefix(command, "multisell"):
		reply.Outcome, reply.Multisell = BypassMultisell, strings.TrimFunc(command[len("multisell"):], javaSpace)
		return reply
	case strings.HasPrefix(command, "exc_multisell"):
		reply.Outcome, reply.Multisell, reply.InventoryOnly = BypassMultisell, strings.TrimFunc(command[len("exc_multisell"):], javaSpace), true
		return reply
	case strings.HasPrefix(command, "Augment"):
		// The choice is the one character after "Augment ": a command too
		// short to hold it, or one that is no digit, stops the handling.
		arg, ok := commandChars(command, 8, 9)
		if !ok {
			reply.Outcome = BypassAborted
			return reply
		}
		choice, err := commons.Atoi(strings.TrimFunc(arg, javaSpace))
		if err != nil {
			reply.Outcome = BypassAborted
			return reply
		}
		switch choice {
		case 1:
			reply.Outcome = BypassAugmentMake
		case 2:
			reply.Outcome = BypassAugmentCancel
		default:
			reply.Outcome = BypassRefused
		}
		return reply
	case command == "teleport_request":
		reply.Outcome = BypassTeleportList
		return reply
	case strings.HasPrefix(command, "teleport"):
		return teleportCommand(reply, BypassTeleport, command)
	case strings.HasPrefix(command, "instant_teleport"):
		return teleportCommand(reply, BypassInstantTeleport, command)
	}
	if strings.HasPrefix(command, "CPRecovery") {
		reply.Outcome = BypassRefused
		if arenaManager(f.NpcID()) {
			reply.Outcome = BypassCPRecovery
		}
	}
	return reply
}

// teleportCommand answers "<command> <index>", split on spaces: outcome
// names destination index. A command without an index, or whose index
// does not parse, is only released.
func teleportCommand(reply BypassReply, outcome BypassOutcome, command string) BypassReply {
	words := strings.FieldsFunc(command, func(r rune) bool { return r == ' ' })
	if len(words) < 2 {
		reply.Outcome = BypassReleased
		return reply
	}
	index, err := commons.ParseInt(words[1], 32)
	if err != nil {
		reply.Outcome = BypassReleased
		return reply
	}
	reply.Outcome, reply.Index = outcome, int(index)
	return reply
}

// adventurerCommand answers an adventurer guildsman's own commands:
// raidInfo <level> opens the raid boss page of that level, the overview
// for level 0, and questlist, in any case, opens the quest information
// window. The level is read from the tenth character on: a command too
// short to hold one, or whose level does not parse, aborts. ok is false
// for every other command.
func (f *Folk) adventurerCommand(pages Pages, command string, reply BypassReply) (BypassReply, bool) {
	switch {
	case strings.HasPrefix(command, "raidInfo"):
		arg, ok := commandChars(command, 9, -1)
		if !ok {
			reply.Outcome = BypassAborted
			return reply, true
		}
		level, err := commons.ParseInt(strings.TrimFunc(arg, javaSpace), 32)
		if err != nil {
			reply.Outcome = BypassAborted
			return reply, true
		}
		path := "data/html/adventurer_guildsman/raid_info/info.htm"
		if level != 0 {
			path = "data/html/adventurer_guildsman/raid_info/level" + strconv.FormatInt(level, 10) + ".htm"
		}
		reply.Outcome, reply.HTML = BypassChatWindow, f.page(pages, path)
		return reply, true
	case strings.EqualFold(command, "questlist"):
		reply.Outcome = BypassQuestInfo
		return reply, true
	}
	return reply, false
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
	id, err := commons.ParseInt(tokens[1], 32)
	if err != nil {
		reply.Outcome = BypassAborted
		return reply, true
	}
	reply.ListID = int(id)
	return reply, true
}

// merchantMultisell answers a merchant's own multisell commands, whose
// first word matches in any case: Multisell <list>, Exc_Multisell <list>,
// Newbie_Exc_Multisell <list>, which only a low-level newbie may open and
// others are told off for, and Multisell_Shadow, the shadow weapon page for
// the talker's level. A list command naming no list answers nothing. ok is
// false for every other command.
func (f *Folk) merchantMultisell(pages Pages, talker Talker, command string, reply BypassReply) (BypassReply, bool) {
	words := strings.FieldsFunc(command, func(r rune) bool { return r == ' ' })
	if len(words) == 0 {
		return reply, false
	}
	switch {
	case strings.EqualFold(words[0], "Multisell"), strings.EqualFold(words[0], "Exc_Multisell"), strings.EqualFold(words[0], "Newbie_Exc_Multisell"):
		if len(words) < 2 {
			reply.Outcome = BypassRefused
			return reply, true
		}
		if strings.EqualFold(words[0], "Newbie_Exc_Multisell") && !talker.LowLevelNewbie {
			reply.Outcome, reply.HTML = BypassChatWindow, f.page(pages, "data/html/exchangelvlimit.htm")
			return reply, true
		}
		reply.Outcome, reply.Multisell = BypassMultisell, words[1]
		reply.InventoryOnly = !strings.EqualFold(words[0], "Multisell")
		return reply, true
	case strings.EqualFold(words[0], "Multisell_Shadow"):
		page := "data/html/common/shadow_item_b.htm"
		switch {
		case talker.Level < 40:
			page = "data/html/common/shadow_item-lowlevel.htm"
		case talker.Level < 46:
			page = "data/html/common/shadow_item_mi_c.htm"
		case talker.Level < 52:
			page = "data/html/common/shadow_item_hi_c.htm"
		}
		reply.Outcome, reply.HTML = BypassPage, f.page(pages, page)
		return reply, true
	}
	return reply, false
}

// javaSpace is the set trimmed around a command argument: every character
// up to and including the space.
func javaSpace(r rune) bool { return r <= ' ' }

// commandChars returns command's characters from begin up to end, or to
// its end when end is negative, counting characters as the client encodes
// them: one per UTF-16 unit, so a character outside the Basic Multilingual
// Plane counts two. ok is false when command is too short for the range. A
// bound that splits such a character keeps its half as U+FFFD, which no
// number reads.
func commandChars(command string, begin, end int) (string, bool) {
	units := utf16.Encode([]rune(command))
	if end < 0 {
		end = len(units)
	}
	if begin > end || end > len(units) {
		return "", false
	}
	return string(utf16.Decode(units[begin:end])), true
}
