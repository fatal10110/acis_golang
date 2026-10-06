package npc

import (
	"strconv"
	"strings"
	"unicode/utf16"

	"github.com/fatal10110/acis_golang/internal/commons"
	"github.com/fatal10110/acis_golang/internal/gameserver/classmaster"
	"github.com/fatal10110/acis_golang/internal/gameserver/derby"
	"github.com/fatal10110/acis_golang/internal/gameserver/schemebuffer"
	"github.com/fatal10110/acis_golang/internal/gameserver/sevensigns"
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
	// BypassQuest opens the quest windows the "Quest [name]" command asks
	// for.
	BypassQuest
	// BypassSubclass runs a village master's subclass command; see
	// ParseSubclassCommand.
	BypassSubclass
	// BypassCPRecovery runs an arena manager's paid CP restore; see
	// Folk.CPRecovery.
	BypassCPRecovery
	// BypassClan runs a village master's clan command; see
	// VillageMasterClanCommand.
	BypassClan
	// BypassClassMaster runs a class manager's own command; see
	// classmaster.ParseCommand.
	BypassClassMaster
	// BypassSchemeBuffer runs a scheme buffer's own command; see
	// schemebuffer.Command.
	BypassSchemeBuffer
	// BypassWedding runs any command on a wedding manager, whose own
	// dialog answers every command.
	BypassWedding
	// BypassLottery runs a lottery seller's "Loto <n>" command; see
	// lottery.Command.
	BypassLottery
	// BypassDerby runs a race manager's own command; see derby.Command.
	BypassDerby
	// BypassObserveGroup lists the viewpoints of group Index.
	BypassObserveGroup
	// BypassObserve takes the talker to watch from viewpoint Index.
	BypassObserve
	// BypassFishingChampionship opens a fisherman's fishing championship
	// winners page.
	BypassFishingChampionship
	// BypassFishingReward claims a fishing championship prize at a
	// fisherman.
	BypassFishingReward
	// BypassHeroList shows the heroes of the running era.
	BypassHeroList
	// BypassHeroClaim has an elected hero claim the hero status.
	BypassHeroClaim
	// BypassAuction runs any command on an auctioneer, whose own dialog
	// answers every command.
	BypassAuction
	// BypassOlympiadNoble runs an Olympiad manager's "OlympiadNoble <n>"
	// service Index for a talker its gates let through: 1 leaves the
	// waiting list, 2 shows its size, 3 the talker's points, 4 and 5 register
	// for non-classed and classed matches, 6 offers the points' trade for
	// passes, 7 opens the passes' multisell and 10 makes the trade; any
	// other answers nothing.
	BypassOlympiadNoble
	// BypassClassRanking shows the month's ranking of class Index.
	BypassClassRanking
	// BypassSevenSigns runs a Seven Signs priest's or Mammon NPC's
	// "SevenSigns <n> ..." command for a talker who selected it; see
	// signspriest.Service.Command. DawnPriest names a Priest of Dawn.
	BypassSevenSigns
	// BypassSignsChat answers a Seven Signs priest's or Mammon NPC's Chat
	// command the way its chat window answers an interact: Chat says how,
	// and HTML is the page.
	BypassSignsChat
	// BypassTerritoryStatus shows the territory status of this NPC's
	// castle.
	BypassTerritoryStatus
	// BypassClanHallManager runs any command on a clan hall manager, whose
	// own dialog answers every command.
	BypassClanHallManager
)

// Talker is what a dialog command reads of the player sending it.
type Talker struct {
	Karma int
	Level int
	// LowLevelNewbie is a level 6 to 25 player who has made at most the
	// first occupation change.
	LowLevelNewbie bool
	// InactiveHero is a player elected hero who has not claimed the status
	// yet.
	InactiveHero bool
	// Noble and Hero are the talker's noble and hero status.
	Noble, Hero bool
	// CursedWeapon is set while the talker holds a cursed weapon.
	CursedWeapon bool
	// SubclassActive is set while the talker plays one of its subclasses.
	SubclassActive bool
	// ThirdClass is set when the talker's class is a third occupation.
	ThirdClass bool
	// CurrentFolk is set when the NPC is the one the talker last selected.
	CurrentFolk bool
	// SevenSigns reads the talker's Record of Seven Signs; nil reads an
	// empty record.
	SevenSigns func() sevensigns.Record
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
	// name, the viewpoint group BypassObserveGroup names, or the viewpoint
	// BypassObserve names.
	Index int
	// Chat is how BypassSignsChat answers.
	Chat ChatOutcome
	// DawnPriest is set when BypassSevenSigns runs at a Priest of Dawn.
	DawnPriest bool
}

// Bypass answers command, the part of an npc_<objectId>_<command> dialog
// link after the object id, sent by talker. A shop, fisherman, gatekeeper
// or warehouse keeper first applies its karma gate to every command,
// answering with its refusal page when that page exists. A fisherman's
// FishSkillList opens the fishing skills list, and its FishingChampionship
// and FishingReward go to the fishing championship. A symbol maker's Draw and
// RemoveList open its windows. A merchant or fisherman then answers its
// sell, multisell and shop commands, and a warehouse keeper its storage
// commands. The trainer commands follow: SkillList and EnchantSkillList
// open the skills to learn and to enchant, after an adventurer guildsman's
// raidInfo and questlist. Then the generic ones: Chat <n> opens chat page n
// (page 0 when n does not parse), Link <path> opens data/html/<path>,
// Loto <n> runs the lottery dialog, multisell <list> and exc_multisell
// <list> open a multisell list, Augment 1 and Augment 2 open the
// augmentation and removal windows,
// observe_group <id> lists a viewpoint group and observe <id> sends the
// talker to watch from a viewpoint,
// teleport_request opens the destination list, and teleport <index> and
// instant_teleport <index> take the talker to a destination, and
// CPRecovery has an arena manager restore the talker's CP for a fee and
// answers nothing at any other NPC. A village master's Subclass commands go
// to the subclass dialog, a class manager's own commands to its dialog,
// a scheme buffer's own commands to its dialog, a race manager's own
// commands to the race track, and every command on a wedding manager to
// its dialog, as does every command on an auctioneer or a clan hall
// manager. A Seven Signs priest's and a Mammon NPC's own commands go to
// signsPriestBypass. Every other command belongs to a system not in place
// yet.
func (f *Folk) Bypass(pages Pages, rules ChatRules, talker Talker, command string) BypassReply {
	karma := talker.Karma
	kind := hostileKind(f.Instance)
	reply := BypassReply{LeadingActionFailed: kind == "DungeonGatekeeper"}
	if kind == weddingManager {
		reply.Outcome = BypassWedding
		return reply
	}
	if kind == auctioneer {
		reply.Outcome = BypassAuction
		return reply
	}
	if kind == clanHallManager {
		reply.Outcome = BypassClanHallManager
		return reply
	}
	if _, ok := unportedFolkChats[kind]; ok {
		return reply
	}
	if kind == "OlympiadManagerNpc" {
		switch {
		case strings.HasPrefix(command, "OlympiadNoble"):
			return f.olympiadNobleCommand(pages, talker, command, reply)
		case strings.HasPrefix(command, "Olympiad"):
			return f.olympiadCommand(pages, talker, command, reply)
		}
	}
	if out, ok := f.signsPriestBypass(pages, kind, talker, command, reply); ok {
		return out
	}
	if _, ok := unportedFolkCommands[kind]; ok {
		return reply
	}
	if _, ok := classmaster.ParseCommand(command); kind == "ClassMaster" && ok {
		reply.Outcome = BypassClassMaster
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
	if f.DerbyTrackManager() && derby.Command(command) {
		reply.Outcome = BypassDerby
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
		// The championship commands run before the merchant karma gate the
		// fisherman inherits.
		if strings.HasPrefix(command, "FishingChampionship") {
			reply.Outcome = BypassFishingChampionship
			return reply
		}
		if strings.HasPrefix(command, "FishingReward") {
			reply.Outcome = BypassFishingReward
			return reply
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
	case strings.EqualFold(command, "TerritoryStatus"):
		reply.Outcome = BypassTerritoryStatus
		return reply
	case strings.HasPrefix(command, "Quest"):
		reply.Outcome = BypassQuest
		return reply
	case strings.HasPrefix(command, "Chat"):
		val := 0
		if arg, ok := commandChars(command, 5, -1); ok {
			if n, err := commons.ParseInt(arg, 32); err == nil {
				val = int(n)
			}
		}
		reply.Outcome = BypassChatWindow
		if kind == "OlympiadManagerNpc" {
			reply.HTML = f.olympiadChat(pages, val, talker.Noble, talker.Hero, talker.InactiveHero)
		} else {
			reply.HTML = f.chatPage(pages, chat, val)
		}
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
	case strings.HasPrefix(command, "observe_group"):
		return observeCommand(reply, BypassObserveGroup, command)
	case strings.HasPrefix(command, "observe"):
		return observeCommand(reply, BypassObserve, command)
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

// observeCommand answers "<command> <id>", split on whitespace: outcome
// names group or viewpoint id. A command without an id, or whose id does
// not parse, aborts.
func observeCommand(reply BypassReply, outcome BypassOutcome, command string) BypassReply {
	words := strings.FieldsFunc(command, func(r rune) bool {
		return r == ' ' || r == '\t' || r == '\n' || r == '\r' || r == '\f'
	})
	if len(words) < 2 {
		reply.Outcome = BypassAborted
		return reply
	}
	id, err := commons.ParseInt(words[1], 32)
	if err != nil {
		reply.Outcome = BypassAborted
		return reply
	}
	reply.Outcome, reply.Index = outcome, int(id)
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

// olympiadCommand answers an Olympiad manager's "Olympiad <n>" command,
// whose choice is the one character after "Olympiad ": a command too short
// to hold it, or one that is no digit, stops the handling. 2_<class> shows
// the month's ranking of a class from 88 to 118, answering nothing for any
// other, and stops the handling when the class is missing or does not
// parse; 4 shows the heroes; 5 asks an elected hero to confirm its claim
// and 6 claims the status, answering nothing to anyone else; 7 opens the
// Monument of Heroes' main page. The stadium list (3) needs the matches
// (#3340); any other choice answers nothing.
func (f *Folk) olympiadCommand(pages Pages, talker Talker, command string, reply BypassReply) BypassReply {
	arg, ok := commandChars(command, 9, 10)
	if !ok {
		reply.Outcome = BypassAborted
		return reply
	}
	choice, err := commons.Atoi(arg)
	if err != nil {
		reply.Outcome = BypassAborted
		return reply
	}
	switch choice {
	case 2:
		return classRankingCommand(reply, command)
	case 3:
		reply.Outcome = BypassUnported
	case 4:
		reply.Outcome = BypassHeroList
	case 5:
		reply.Outcome = BypassRefused
		if talker.InactiveHero {
			reply.Outcome, reply.HTML = BypassPage, f.page(pages, olympiadPages+"hero_confirm.htm")
		}
	case 6:
		reply.Outcome = BypassHeroClaim
	case 7:
		reply.Outcome, reply.HTML = BypassPage, f.heroMainPage(pages, talker.InactiveHero)
	default:
		reply.Outcome = BypassRefused
	}
	return reply
}
