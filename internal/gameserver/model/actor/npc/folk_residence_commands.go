package npc

import (
	"strconv"
	"strings"
	"time"

	"github.com/fatal10110/acis_golang/internal/commons"
	"github.com/fatal10110/acis_golang/internal/gameserver/clan"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/player"
	"github.com/fatal10110/acis_golang/internal/gameserver/sevensigns"
)

// DoorScope says which doors a doorman's door command reaches.
type DoorScope int

const (
	// DoorsOfWorld are the doors of the world, by id: a plain doorman's.
	// An id naming no door stops the handling.
	DoorsOfWorld DoorScope = iota
	// DoorsOfResidence are the gates of the castle or siegable hall the
	// doorman serves, by id: an id naming no gate of it is passed over.
	DoorsOfResidence
	// DoorsOfHall are every gate of the clan hall the doorman serves.
	DoorsOfHall
)

// Mass teleport delays: a mass gatekeeper's defenders leave after
// massTeleportDelay, or after massTeleportSiegeDelay while its castle's
// siege runs with every life control tower down.
const (
	massTeleportDelay      = 30
	massTeleportSiegeDelay = 480
)

// MassTeleportShout is what a mass gatekeeper shouts to its region when it
// sends the defenders of castle into the inner castle during its siege.
func MassTeleportShout(castle string) string {
	return "The defenders of " + castle + " castle have been teleported to the inner castle."
}

// ClanGateRefusal is what a castle magician tells a talker it cannot send to
// the clan leader's portal.
const ClanGateRefusal = "Couldn't teleport to clan leader. The requirements was not meet."

// ClanGateLeader is what a castle magician reads of the clan leader whose
// portal a talker asks to go to.
type ClanGateLeader struct {
	// Busy is set while the leader is dead or playing dead, runs a private
	// store, is rooted, fights, plays in the Olympiad or the festival,
	// observes, or stands where no friend may be summoned.
	Busy bool
	// Cabal is the leader's Seven Signs cabal.
	Cabal sevensigns.Cabal
}

// ClanGateTalker is what a castle magician reads of the talker and the
// Seven Signs to send it to its leader's portal.
type ClanGateTalker struct {
	// InDungeon is set while the talker stands in a Seven Signs dungeon.
	InDungeon bool
	// SealValidation is set during the seal validation period, and Winning
	// is the cabal that won the last competition.
	SealValidation bool
	Winning        sevensigns.Cabal
}

// ClanGateAllowed reports whether a talker may go to leader's portal: never
// while the leader is busy; from a Seven Signs dungeon, only to a leader of
// the winning cabal during seal validation (a tie leaves both without a
// cabal, which lets a leader of none through), and to a leader of any cabal
// otherwise.
func ClanGateAllowed(leader ClanGateLeader, talker ClanGateTalker) bool {
	if leader.Busy {
		return false
	}
	if !talker.InDungeon {
		return true
	}
	if talker.SealValidation {
		return leader.Cabal == talker.Winning
	}
	return leader.Cabal != sevensigns.NoCabal
}

// residence reads the talker's standing at the residence f serves.
func (t Talker) residence() ResidenceTalk {
	if t.Residence == nil {
		return ResidenceTalk{}
	}
	return t.Residence()
}

// residenceCommand answers the own dialog commands of a castle or clan hall
// NPC, those it reads before the generic ones; ok is false for a command it
// leaves to them, and at any other NPC.
func (f *Folk) residenceCommand(pages Pages, kind InstanceKind, rules ChatRules, talker Talker, command string, reply BypassReply) (BypassReply, bool) {
	switch kind {
	case "CastleBlacksmith":
		return f.blacksmithCommand(pages, rules, talker.residence(), command, reply)
	case "CastleMagician":
		return f.magicianCommand(pages, talker.residence(), command, reply)
	case "CastleDoorman", "ClanHallDoorman", "Doorman":
		return f.doormanCommand(pages, kind, rules, talker, command, reply)
	case "MercenaryManagerNpc":
		return f.mercenaryCommand(pages, talker.residence(), command, reply)
	case "CastleGatekeeper":
		return f.massGatekeeperCommand(pages, talker.residence(), command, reply)
	case "WyvernManagerNpc":
		return f.wyvernManagerCommand(pages, rules, talker, command, reply)
	case "CastleChamberlain":
		return f.chamberlainBypass(pages, kind, talker.residence(), command, reply)
	}
	return reply, false
}

// chatVal reads a Chat command's page number, 0 when it has none or it does
// not parse.
func chatVal(command string) int {
	if arg, ok := commandChars(command, 5, -1); ok {
		if n, err := commons.ParseInt(arg, 32); err == nil {
			return int(n)
		}
	}
	return 0
}

// valFile is page val of base: base.htm for 0, else base-<val>.htm.
func valFile(base string, val int) string {
	if val == 0 {
		return base + ".htm"
	}
	return base + "-" + strconv.Itoa(val) + ".htm"
}

// residenceChatPage answers the Chat command of a type whose numbered chat
// pages are its residence's: a castle warehouse keeper's, by the talker's
// standing, after ActionFailed, and a clan hall doorman's first page whatever
// the number. ok is false for any other type.
func (f *Folk) residenceChatPage(pages Pages, kind InstanceKind, r ResidenceTalk, val int) (string, ChatOutcome, bool) {
	switch kind {
	case "CastleWarehouseKeeper":
		page := f.castlePage(pages, r.castleTalkCond(r.SiegeInProgress, clan.PrivNone), "data/html/castlewarehouse/",
			"castlewarehouse-no.htm", "castlewarehouse-busy.htm", valFile("castlewarehouse", val))
		return strings.ReplaceAll(page, "%npcname%", f.CharacterName()), ChatShownReleased, true
	case "ClanHallDoorman":
		page, outcome := f.hallDoormanChat(pages, r)
		return page, outcome, true
	}
	return "", ChatShown, false
}

// blacksmithCommand answers every command at a castle blacksmith: the
// generic page alone while the manor is off; nothing for a talker not
// allowed to run its castle's manor or while its siege runs; Chat <n> opens
// its page n alone. Any other command is left to the generic ones.
func (f *Folk) blacksmithCommand(pages Pages, rules ChatRules, r ResidenceTalk, command string, reply BypassReply) (BypassReply, bool) {
	if !rules.AllowManor {
		reply.Outcome = BypassPage
		reply.HTML = strings.ReplaceAll(f.page(pages, "data/html/npcdefault.htm"), "%npcname%", f.CharacterName())
		return reply, true
	}
	if r.castleTalkCond(r.SiegeInProgress, clan.PrivCastleManor) != talkOwner {
		reply.Outcome = BypassRefused
		return reply, true
	}
	if !strings.HasPrefix(command, "Chat") {
		return reply, false
	}
	page := f.page(pages, "data/html/castleblacksmith/"+valFile("castleblacksmith", chatVal(command)))
	page = strings.ReplaceAll(page, "%npcname%", f.CharacterName())
	reply.Outcome, reply.HTML = BypassPage, strings.ReplaceAll(page, "%castleid%", strconv.Itoa(r.CastleID))
	return reply, true
}

// magicianCommand answers a castle magician's own commands: Chat <n> opens
// its page n by the talker's standing, after ActionFailed, and gotoleader
// sends a clan member to its leader's portal, magician-nogate.htm naming the
// portal's absence; a clanless talker is answered nothing. Any other command
// is left to the generic ones.
func (f *Folk) magicianCommand(pages Pages, r ResidenceTalk, command string, reply BypassReply) (BypassReply, bool) {
	switch {
	case strings.HasPrefix(command, "Chat"):
		reply.Outcome, reply.Chat = BypassChatAs, ChatShownReleased
		reply.HTML = f.castlePage(pages, r.castleTalkCond(r.BattlefieldActive, clan.PrivNone), "data/html/castlemagician/",
			"magician-no.htm", "magician-busy.htm", valFile("magician", chatVal(command)))
		return reply, true
	case command == "gotoleader":
		reply.Outcome = BypassRefused
		if r.ClanID != 0 {
			reply.Outcome, reply.HTML = BypassClanGate, f.page(pages, "data/html/castlemagician/magician-nogate.htm")
		}
		return reply, true
	}
	return reply, false
}

// doormanOwner reports whether a doorman of kind lets the talker work its
// gates and teleports: a plain doorman lets anyone; a castle doorman a
// member of the clan owning its siegable hall or castle holding the right
// to its gates; a clan hall doorman a member of the clan owning its hall.
func doormanOwner(kind InstanceKind, r ResidenceTalk) bool {
	switch kind {
	case "CastleDoorman":
		switch {
		case r.ClanID == 0:
			return false
		case r.HallID != 0 && r.Siegable:
			return r.owns() && r.holds(clan.PrivHallEntryExit)
		case r.CastleID != 0:
			return r.owns() && r.holds(clan.PrivCastleEntryExit)
		}
		return false
	case "ClanHallDoorman":
		return r.HallID != 0 && r.owns()
	}
	return true
}

// doormanCommand answers a doorman's own commands. An Aden clan hall
// doorman's wyvern_info and wyvern_help open its wyvern pages 1 and 7 for a
// member of the owning clan, and answer nothing to anyone else. open_doors
// and close_doors work the gates they name for a talker doormanOwner lets
// through, and answer nothing to anyone else; while a castle doorman's
// battlefield is active they open its busy page instead. Any other command
// is left to the generic ones.
//
// ponytail: a siegable hall's siege is not run yet (#244), so its doorman
// never reads as busy.
func (f *Folk) doormanCommand(pages Pages, kind InstanceKind, rules ChatRules, talker Talker, command string, reply BypassReply) (BypassReply, bool) {
	r := talker.residence()
	if kind == "ClanHallDoorman" {
		if out, ok := f.hallWyvernCommand(pages, rules, talker, r, command, reply); ok {
			return out, true
		}
	}
	open := strings.HasPrefix(command, "open_doors")
	if !open && !strings.HasPrefix(command, "close_doors") {
		return reply, false
	}
	reply.Outcome = BypassRefused
	if !doormanOwner(kind, r) {
		return reply, true
	}
	if kind == "CastleDoorman" && r.HallID == 0 && r.BattlefieldActive {
		reply.Outcome, reply.HTML = BypassDoorsBusy, f.doormanBusyPage(pages)
		return reply, true
	}
	reply.Outcome, reply.OpenDoors = BypassDoors, open
	switch kind {
	case "ClanHallDoorman":
		page := "data/html/clanHallDoormen/doormen-closed.htm"
		if open {
			page = "data/html/clanHallDoormen/doormen-opened.htm"
		}
		reply.DoorScope, reply.HTML = DoorsOfHall, f.page(pages, page)
	case "CastleDoorman":
		reply.DoorScope, reply.doorCommand = DoorsOfResidence, command
	default:
		reply.DoorScope, reply.doorCommand = DoorsOfWorld, command
	}
	return reply, true
}

// DoorIDs are the doors a BypassDoors reply names, in order; malformed is
// set when its list stops the handling after them.
func (r BypassReply) DoorIDs() (ids []int, malformed bool) {
	switch r.DoorScope {
	case DoorsOfWorld:
		return worldDoorIDs(r.doorCommand, r.OpenDoors)
	case DoorsOfResidence:
		return residenceDoorIDs(r.doorCommand, r.OpenDoors)
	case DoorsOfHall:
	}
	return nil, false
}

// doormanBusyPage is the page a doorman refuses its gates with during a
// siege: doormen/<npcId>-busy.htm, else doormen/busy.htm, sent as read.
func (f *Folk) doormanBusyPage(pages Pages) string {
	if page, ok := pages.Get("data/html/doormen/" + strconv.Itoa(f.NpcID()) + "-busy.htm"); ok {
		return page
	}
	return rawPage(pages, "data/html/doormen/busy.htm")
}

// rawPage reads path as is, with no placeholder filled; a missing page
// reads as missingHTML.
func rawPage(pages Pages, path string) string {
	page, ok := pages.Get(path)
	if !ok {
		page = missingHTML(path)
	}
	return page
}

// missingHTML is the "My html is missing" notice naming path.
func missingHTML(path string) string {
	return "<html><body>My html is missing:<br>" + path + "</body></html>"
}

// doorSeparator reports whether r splits a plain doorman's door list.
func doorSeparator(r rune) bool { return r == ',' || r == ' ' }

// worldDoorIDs reads a plain doorman's door command: the ids after the
// command name, split on commas and spaces, whose first one is skipped.
// malformed is set when the list is empty or an id does not parse; ids
// holds those read before.
func worldDoorIDs(command string, open bool) (ids []int, malformed bool) {
	start := len("close_doors")
	if open {
		start = len("open_doors")
	}
	tokens := strings.FieldsFunc(command[start:], doorSeparator)
	if len(tokens) == 0 {
		return nil, true
	}
	return parseDoorIDs(tokens[1:])
}

// residenceDoorIDs reads a castle doorman's door command: the ids after the
// command name and one character, split on ", ". malformed is set when the
// command is too short to hold the list or an id does not parse; ids holds
// those read before.
func residenceDoorIDs(command string, open bool) (ids []int, malformed bool) {
	start := len("close_doors ")
	if open {
		start = len("open_doors ")
	}
	if len(command) < start {
		return nil, true
	}
	return parseDoorIDs(splitTrailing(command[start:], ", "))
}

// splitTrailing splits s around sep and drops the empty pieces at its end,
// keeping one empty piece for an empty s.
func splitTrailing(s, sep string) []string {
	parts := strings.Split(s, sep)
	if s == "" {
		return parts
	}
	for len(parts) > 0 && parts[len(parts)-1] == "" {
		parts = parts[:len(parts)-1]
	}
	return parts
}

// parseDoorIDs reads each token as a door id up to the first that does not
// parse, reporting it malformed.
func parseDoorIDs(tokens []string) (ids []int, malformed bool) {
	for _, token := range tokens {
		id, err := commons.ParseInt(token, 32)
		if err != nil {
			return ids, true
		}
		ids = append(ids, int(id))
	}
	return ids, false
}

// hallWyvernCommand answers an Aden clan hall doorman's wyvern commands,
// matched by prefix, for a member of the clan owning its hall; anyone else,
// and any other hall's doorman, is answered nothing. wyvern_info and
// wyvern_help open the wyvern pages 1 and 7, and wyvern_ride lets the clan
// leader ride; see wyvernRide. ok is false for every other command.
func (f *Folk) hallWyvernCommand(pages Pages, rules ChatRules, talker Talker, r ResidenceTalk, command string, reply BypassReply) (BypassReply, bool) {
	var val string
	switch {
	case strings.HasPrefix(command, "wyvern_info"):
		val = "1"
	case strings.HasPrefix(command, "wyvern_help"):
		val = "7"
	case strings.HasPrefix(command, "wyvern_ride"):
	default:
		return reply, false
	}
	reply.Outcome = BypassRefused
	if !wyvernHall(r.HallID) || !doormanOwner("ClanHallDoorman", r) {
		return reply, true
	}
	if val == "" {
		if r.ClanLeader {
			reply = f.wyvernRide(pages, rules, talker, r, "clanHallDoormen", true, reply)
		}
		return reply, true
	}
	reply.Outcome, reply.Chat = BypassChatAs, ChatShown
	reply.HTML = f.wyvernPage(pages, rules, "data/html/clanHallDoormen/wyvernmanager-"+val+".htm")
	return reply, true
}

// wyvernRide answers a clan leader's request to ride a wyvern, whose pages
// are those of dir: page 3 while Dusk owns the Seal of Strife; on no
// strider, the message saying a wyvern is ridden only from one with page 1,
// except that a hall doorman answers page 8 alone while the talker's mount
// level is short of the ride's strider level; page 6 for a strider of too
// low a level; else the ride itself, page 4, or page 5 for a talker short of
// the crystals. Each page then releases the client.
func (f *Folk) wyvernRide(pages Pages, rules ChatRules, talker Talker, r ResidenceTalk, dir string, doorman bool, reply BypassReply) BypassReply {
	page := func(val string) string {
		return f.wyvernPage(pages, rules, "data/html/"+dir+"/wyvernmanager-"+val+".htm")
	}
	strider := player.MountTypeOf(talker.MountNPCID) == player.MountTypeStrider
	short := talker.MountLevel < rules.Wyvern.StriderLevel
	reply.Outcome = BypassChatWindow
	switch {
	case r.StrifeOwner == sevensigns.Dusk:
		reply.HTML = page("3")
	case !strider && doorman && short:
		reply.HTML = page("8")
	case !strider:
		reply.Outcome, reply.HTML = BypassWyvernNoStrider, page("1")
	case short:
		reply.HTML = page("6")
	default:
		reply.Outcome, reply.HTML, reply.ShortHTML = BypassWyvernRide, page("4"), page("5")
		reply.Count = rules.Wyvern.Crystals
	}
	return reply
}

// teleportAllowed reports whether an NPC of kind lets the talker use its
// teleports: a castle or clan hall doorman only the talker doormanOwner
// lets through, any other NPC everyone.
func teleportAllowed(kind InstanceKind, talker Talker) bool {
	switch kind {
	case "CastleDoorman", "ClanHallDoorman":
		return doormanOwner(kind, talker.residence())
	}
	return true
}

// mercenaryCommand answers every command at a mercenary manager for a
// talker allowed to hire its castle's mercenaries while no siege runs, and
// nothing to anyone else. back reopens its first page, how_to opens
// mseller005.htm, merc_limit the castle's mercenary limits and hire <n> the
// mercenaries of buylist <npcId><n>, outside the seal validation period
// refused with msellerdenial.htm. Any other command is left to the generic
// ones.
func (f *Folk) mercenaryCommand(pages Pages, r ResidenceTalk, command string, reply BypassReply) (BypassReply, bool) {
	const dir = "data/html/mercmanager/"
	if r.castleTalkCond(r.SiegeInProgress, clan.PrivCastleMercenary) != talkOwner {
		reply.Outcome = BypassRefused
		return reply, true
	}
	switch {
	case strings.HasPrefix(command, "back"):
		reply.Outcome, reply.HTML = BypassPage, f.mercenaryManagerChat(pages, r)
	case strings.HasPrefix(command, "how_to"):
		reply.Outcome, reply.HTML = BypassPage, f.page(pages, dir+"mseller005.htm")
	case strings.HasPrefix(command, "hire"):
		return f.mercenaryHire(pages, r, command, reply), true
	case strings.HasPrefix(command, "merc_limit"):
		file := "msellerLimit.htm"
		if r.CastleID == 5 {
			file = "aden_msellerLimit.htm"
		}
		reply.Outcome = BypassPage
		reply.HTML = strings.ReplaceAll(f.page(pages, dir+file), "%castleName%", r.CastleName)
	default:
		return reply, false
	}
	return reply, true
}

// mercenaryHire answers "hire <n>": outside the seal validation period the
// refusal page; otherwise the buy window of buylist <npcId><n>, untaxed,
// followed by mseller004.htm as read. A command naming no list, or one whose
// list id does not parse, aborts.
func (f *Folk) mercenaryHire(pages Pages, r ResidenceTalk, command string, reply BypassReply) BypassReply {
	if !r.SealValidation {
		reply.Outcome, reply.HTML = BypassPage, f.page(pages, "data/html/mercmanager/msellerdenial.htm")
		return reply
	}
	tokens := strings.FieldsFunc(command, func(r rune) bool { return r == ' ' })
	if len(tokens) < 2 {
		reply.Outcome = BypassAborted
		return reply
	}
	id, err := commons.ParseInt(strconv.Itoa(f.NpcID())+tokens[1], 32)
	if err != nil {
		reply.Outcome = BypassAborted
		return reply
	}
	reply.Outcome, reply.ListID = BypassMercenaryHire, int(id)
	reply.HTML = rawPage(pages, "data/html/mercmanager/mseller004.htm")
	return reply
}

// massTeleportSeconds is how long a mass gatekeeper's defenders wait for
// their teleport.
func massTeleportSeconds(r ResidenceTalk) int {
	if r.SiegeInProgress && r.LifeTowers == 0 {
		return massTeleportSiegeDelay
	}
	return massTeleportDelay
}

// massGatekeeperChat is a mass gatekeeper's chat window, its page alone:
// while its teleport is pending MassGK-1.htm naming the delay, else
// MassGK-2.htm while its castle's siege runs with every life control tower
// down, else MassGK.htm. A mass gatekeeper of no castle opens nothing.
func (f *Folk) massGatekeeperChat(pages Pages, r ResidenceTalk) (string, ChatOutcome) {
	const dir = "data/html/castleteleporter/"
	switch {
	case r.CastleID == 0:
		return "", ChatSilent
	case f.MassTeleportPending():
		return strings.ReplaceAll(f.page(pages, dir+"MassGK-1.htm"), "%delay%", strconv.Itoa(massTeleportSeconds(r))), ChatPageOnly
	case r.SiegeInProgress && r.LifeTowers == 0:
		return f.page(pages, dir+"MassGK-2.htm"), ChatPageOnly
	}
	return f.page(pages, dir+"MassGK.htm"), ChatPageOnly
}

// massGatekeeperCommand answers a mass gatekeeper's tele: the defenders'
// teleport armed unless one is pending, then MassGK-1.htm naming the delay,
// sent with its object id unfilled. At a mass gatekeeper of no castle it
// aborts. Any other command is left to the generic ones.
func (f *Folk) massGatekeeperCommand(pages Pages, r ResidenceTalk, command string, reply BypassReply) (BypassReply, bool) {
	if !strings.HasPrefix(command, "tele") {
		return reply, false
	}
	if r.CastleID == 0 {
		reply.Outcome = BypassAborted
		return reply, true
	}
	delay := massTeleportSeconds(r)
	reply.Outcome, reply.Delay = BypassMassTeleport, time.Duration(delay)*time.Second
	reply.HTML = strings.ReplaceAll(rawPage(pages, "data/html/castleteleporter/MassGK-1.htm"), "%delay%", strconv.Itoa(delay))
	return reply, true
}

// MassTeleportPending reports whether this mass gatekeeper's teleport is
// armed.
func (f *Folk) MassTeleportPending() bool { return f.massTeleport.Load() }

// ArmMassTeleport arms this mass gatekeeper's teleport, unless one is
// pending: oust runs on the NPC's queue delay from now, after which a new
// one can be armed. It reports whether it armed one.
func (f *Folk) ArmMassTeleport(delay time.Duration, oust func()) bool {
	if f.queue == nil || !f.massTeleport.CompareAndSwap(false, true) {
		return false
	}
	f.queue.After(delay, func() {
		oust()
		f.massTeleport.Store(false)
	})
	return true
}

// wyvernManagerChat is a wyvern manager's chat window: page 4 for the
// leader of the clan owning its residence while flying, page 0 while not;
// page 2 for another member of that clan; page 0a for anyone else.
func (f *Folk) wyvernManagerChat(pages Pages, rules ChatRules, r ResidenceTalk) string {
	val := "0a"
	switch {
	case r.owns() && r.ClanLeader && r.Flying:
		val = "4"
	case r.owns() && r.ClanLeader:
		val = "0"
	case r.owns():
		val = "2"
	}
	return f.wyvernPage(pages, rules, "data/html/wyvernmanager/wyvernmanager-"+val+".htm")
}

// wyvernPage is a wyvern page naming this NPC and the ride's strider level
// and crystal count.
func (f *Folk) wyvernPage(pages Pages, rules ChatRules, path string) string {
	return strings.NewReplacer(
		"%npcname%", f.CharacterName(),
		"%wyvern_level%", strconv.Itoa(rules.Wyvern.StriderLevel),
		"%needed_crystals%", strconv.Itoa(rules.Wyvern.Crystals),
	).Replace(f.page(pages, path))
}

// wyvernManagerCommand answers a wyvern manager's commands; it heeds only
// the talker who last selected it. Chat <val> opens page val, page 1 when
// the command has none, and a val climbing out of the wyvern pages reads as
// a missing page; RideWyvern lets the leader of the clan owning its
// residence ride, see wyvernRide, and opens page 2 for anyone else. Every
// other command reaches the chamberlain's dialog, with the wyvern manager's
// own standing; ok is false for a command that dialog leaves to the
// merchant's.
func (f *Folk) wyvernManagerCommand(pages Pages, rules ChatRules, talker Talker, command string, reply BypassReply) (BypassReply, bool) {
	switch {
	case !talker.CurrentFolk:
		reply.Outcome = BypassRefused
	case strings.HasPrefix(command, "RideWyvern"):
		r := talker.residence()
		if !r.owns() || !r.ClanLeader {
			reply.Outcome, reply.HTML = BypassChatWindow, f.wyvernPage(pages, rules, "data/html/wyvernmanager/wyvernmanager-2.htm")
			break
		}
		reply = f.wyvernRide(pages, rules, talker, r, "wyvernmanager", false, reply)
	case strings.HasPrefix(command, "Chat"):
		val, ok := commandChars(command, 5, -1)
		if !ok {
			val = "1"
		}
		reply.Outcome, reply.Chat = BypassChatAs, ChatShown
		path := "data/html/wyvernmanager/wyvernmanager-" + val + ".htm"
		reply.HTML = f.wyvernPage(pages, rules, path)
		if strings.Contains(val, "..") {
			// A page outside the wyvern pages is never read from a typed value.
			reply.HTML = missingHTML(path)
		}
	default:
		return f.chamberlainBypass(pages, "WyvernManagerNpc", talker.residence(), command, reply)
	}
	return reply, true
}
