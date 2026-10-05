package network

import (
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/fatal10110/acis_golang/internal/commons"
	"github.com/fatal10110/acis_golang/internal/gameserver/clan"
	"github.com/fatal10110/acis_golang/internal/gameserver/clanhall"
	skillhandler "github.com/fatal10110/acis_golang/internal/gameserver/handler/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/event"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/npc"
	hallmodel "github.com/fatal10110/acis_golang/internal/gameserver/model/residence/clanhall"
	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/travel"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
)

// The clan hall manager's pages and the date its function pages show.
const (
	hallManagerPages  = "data/html/clanHallManager/"
	hallFeeDateLayout = "02-01-2006 15:04"
	hallFunctionNone  = "none"
)

// supportCastWeight is the weight of a clan hall manager's support magic
// cast desire.
const supportCastWeight = 1_000_000

// The messages a clan hall manager's support command answers with.
const (
	supportCursedMessage  = "The wielder of a cursed weapon cannot receive outside heals or buffs"
	supportInvalidMessage = "Invalid skill, contact your server support."
)

// The skill a clan hall manager buffs itself with: 4367 without a support
// magic function, 4366 plus the function's level with one.
const (
	hallManagerBuffBase    = 4366
	hallManagerBuffDefault = 4367
)

// The change links of the function pages, by hall grade; the _SCH forms are
// a siegable hall's.
const (
	removeHP     = `[<a action="bypass -h npc_%objectId%_manage recovery hp_cancel">Remove</a>]`
	hpGrade1     = `[<a action="bypass -h npc_%objectId%_manage recovery edit_hp 2">40%</a>][<a action="bypass -h npc_%objectId%_manage recovery edit_hp 5">100%</a>][<a action="bypass -h npc_%objectId%_manage recovery edit_hp 8">160%</a>]`
	hpGrade2     = `[<a action="bypass -h npc_%objectId%_manage recovery edit_hp 4">80%</a>][<a action="bypass -h npc_%objectId%_manage recovery edit_hp 7">140%</a>][<a action="bypass -h npc_%objectId%_manage recovery edit_hp 10">200%</a>][<a action="bypass -h npc_%objectId%_manage recovery edit_hp 260">260%</a>]`
	hpGrade3     = `[<a action="bypass -h npc_%objectId%_manage recovery edit_hp 4">80%</a>][<a action="bypass -h npc_%objectId%_manage recovery edit_hp 6">120%</a>][<a action="bypass -h npc_%objectId%_manage recovery edit_hp 9">180%</a>][<a action="bypass -h npc_%objectId%_manage recovery edit_hp 12">240%</a>][<a action="bypass -h npc_%objectId%_manage recovery edit_hp 15">300%</a>]`
	hpGrade2SCH  = `[<a action="bypass -h npc_%objectId%_manage recovery edit_hp 25">300%</a>]`
	hpGrade3SCH  = `[<a action="bypass -h npc_%objectId%_manage recovery edit_hp 25">300%</a>][<a action="bypass -h npc_%objectId%_manage recovery edit_hp 30">400%</a>]`
	removeExp    = `[<a action="bypass -h npc_%objectId%_manage recovery exp_cancel">Remove</a>]`
	expGrade1    = `[<a action="bypass -h npc_%objectId%_manage recovery edit_exp 1">5%</a>][<a action="bypass -h npc_%objectId%_manage recovery edit_exp 3">15%</a>][<a action="bypass -h npc_%objectId%_manage recovery edit_exp 6">30%</a>]`
	expGrade2    = `[<a action="bypass -h npc_%objectId%_manage recovery edit_exp 1">5%</a>][<a action="bypass -h npc_%objectId%_manage recovery edit_exp 3">15%</a>][<a action="bypass -h npc_%objectId%_manage recovery edit_exp 5">25%</a>][<a action="bypass -h npc_%objectId%_manage recovery edit_exp 8">40%</a>]`
	expGrade3    = `[<a action="bypass -h npc_%objectId%_manage recovery edit_exp 3">15%</a>][<a action="bypass -h npc_%objectId%_manage recovery edit_exp 5">25%</a>][<a action="bypass -h npc_%objectId%_manage recovery edit_exp 7">35%</a>][<a action="bypass -h npc_%objectId%_manage recovery edit_exp 10">50%</a>]`
	expGrade2SCH = `[<a action="bypass -h npc_%objectId%_manage recovery edit_exp 19">45%</a>]`
	expGrade3SCH = `[<a action="bypass -h npc_%objectId%_manage recovery edit_exp 19">45%</a>][<a action="bypass -h npc_%objectId%_manage recovery edit_exp 20">50%</a>]`
	removeMP     = `[<a action="bypass -h npc_%objectId%_manage recovery mp_cancel">Remove</a>]`
	mpGrade1     = `[<a action="bypass -h npc_%objectId%_manage recovery edit_mp 1">5%</a>][<a action="bypass -h npc_%objectId%_manage recovery edit_mp 3">15%</a>][<a action="bypass -h npc_%objectId%_manage recovery edit_mp 5">25%</a>]`
	mpGrade2     = `[<a action="bypass -h npc_%objectId%_manage recovery edit_mp 1">5%</a>][<a action="bypass -h npc_%objectId%_manage recovery edit_mp 3">15%</a>][<a action="bypass -h npc_%objectId%_manage recovery edit_mp 6">30%</a>]`
	mpGrade3     = `[<a action="bypass -h npc_%objectId%_manage recovery edit_mp 1">5%</a>][<a action="bypass -h npc_%objectId%_manage recovery edit_mp 3">15%</a>][<a action="bypass -h npc_%objectId%_manage recovery edit_mp 6">30%</a>][<a action="bypass -h npc_%objectId%_manage recovery edit_mp 8">40%</a>]`
	mpGrade2SCH  = `[<a action="bypass -h npc_%objectId%_manage recovery edit_mp 18">40%</a>]`
	mpGrade3SCH  = `[<a action="bypass -h npc_%objectId%_manage recovery edit_mp 18">40%</a>][<a action="bypass -h npc_%objectId%_manage recovery edit_mp 20">50%</a>]`

	removeSupport    = `[<a action="bypass -h npc_%objectId%_manage other support_cancel">Remove</a>]`
	supportGrade1    = `[<a action="bypass -h npc_%objectId%_manage other edit_support 1">Level 1</a>][<a action="bypass -h npc_%objectId%_manage other edit_support 2">Level 2</a>][<a action="bypass -h npc_%objectId%_manage other edit_support 4">Level 4</a>]`
	supportGrade2    = `[<a action="bypass -h npc_%objectId%_manage other edit_support 3">Level 3</a>][<a action="bypass -h npc_%objectId%_manage other edit_support 4">Level 4</a>][<a action="bypass -h npc_%objectId%_manage other edit_support 5">Level 5</a>]`
	supportGrade3    = `[<a action="bypass -h npc_%objectId%_manage other edit_support 3">Level 3</a>][<a action="bypass -h npc_%objectId%_manage other edit_support 5">Level 5</a>][<a action="bypass -h npc_%objectId%_manage other edit_support 7">Level 7</a>][<a action="bypass -h npc_%objectId%_manage other edit_support 8">Level 8</a>]`
	supportGrade2SCH = `[<a action="bypass -h npc_%objectId%_manage other edit_support 15">Level 5</a>]`
	supportGrade3SCH = `[<a action="bypass -h npc_%objectId%_manage other edit_support 15">Level 5</a>][<a action="bypass -h npc_%objectId%_manage other edit_support 18">Level 8</a>]`
	removeItem       = `[<a action="bypass -h npc_%objectId%_manage other item_cancel">Remove</a>]`
	itemLevels       = `[<a action="bypass -h npc_%objectId%_manage other edit_item 1">Level 1</a>][<a action="bypass -h npc_%objectId%_manage other edit_item 2">Level 2</a>][<a action="bypass -h npc_%objectId%_manage other edit_item 3">Level 3</a>]`
	itemLevelsSCH    = `[<a action="bypass -h npc_%objectId%_manage other edit_item 11">Level 1</a>][<a action="bypass -h npc_%objectId%_manage other edit_item 12">Level 2</a>][<a action="bypass -h npc_%objectId%_manage other edit_item 13">Level 3</a>]`
	removeTele       = `[<a action="bypass -h npc_%objectId%_manage other tele_cancel">Remove</a>]`
	teleLevels       = `[<a action="bypass -h npc_%objectId%_manage other edit_tele 1">Level 1</a>][<a action="bypass -h npc_%objectId%_manage other edit_tele 2">Level 2</a>]`
	teleLevelsSCH    = `[<a action="bypass -h npc_%objectId%_manage other edit_tele 11">Level 1</a>][<a action="bypass -h npc_%objectId%_manage other edit_tele 12">Level 2</a>]`
	removeCurtains   = `[<a action="bypass -h npc_%objectId%_manage deco curtains_cancel">Remove</a>]`
	curtainLevels    = `[<a action="bypass -h npc_%objectId%_manage deco edit_curtains 1">Level 1</a>][<a action="bypass -h npc_%objectId%_manage deco edit_curtains 2">Level 2</a>]`
	removeFixtures   = `[<a action="bypass -h npc_%objectId%_manage deco fixtures_cancel">Remove</a>]`
	fixtureLevels    = `[<a action="bypass -h npc_%objectId%_manage deco edit_fixtures 1">Level 1</a>][<a action="bypass -h npc_%objectId%_manage deco edit_fixtures 2">Level 2</a>]`
)

// hallFunctionDayMs is one day of a function's term, in milliseconds.
const hallFunctionDayMs = int64(24 * time.Hour / time.Millisecond)

// gradeLinks are a function's change links for each hall grade: plain,
// then siegable. A grade without links leaves the placeholder as is.
type gradeLinks struct{ plain, siegable map[int]string }

var (
	hpLinks = gradeLinks{
		plain:    map[int]string{1: hpGrade1, 2: hpGrade2, 3: hpGrade3},
		siegable: map[int]string{1: hpGrade1, 2: hpGrade2SCH, 3: hpGrade3SCH},
	}
	expLinks = gradeLinks{
		plain:    map[int]string{1: expGrade1, 2: expGrade2, 3: expGrade3},
		siegable: map[int]string{1: expGrade1, 2: expGrade2SCH, 3: expGrade3SCH},
	}
	mpLinks = gradeLinks{
		plain:    map[int]string{1: mpGrade1, 2: mpGrade2, 3: mpGrade3},
		siegable: map[int]string{1: mpGrade1, 2: mpGrade2SCH, 3: mpGrade3SCH},
	}
	supportLinks = gradeLinks{
		plain:    map[int]string{1: supportGrade1, 2: supportGrade2, 3: supportGrade3},
		siegable: map[int]string{1: supportGrade1, 2: supportGrade2SCH, 3: supportGrade3SCH},
	}
)

// hallDialog is one clan hall manager command: the talker live, a member
// of cl, the clan owning hall, at its manager f.
type hallDialog struct {
	l    *GameClientLink
	live *livePlayer
	f    *npc.Folk
	hall *hallmodel.Hall
	cl   *clan.Clan
}

// hallOfManager is the clan hall whose NPCs include npcID, the first one
// listing it; an NPC a castle lists belongs to no hall.
func (l *GameClientLink) hallOfManager(npcID int) (*hallmodel.Hall, bool) {
	if _, ok := l.castles.ByNPC(npcID); ok {
		return nil, false
	}
	for _, h := range l.clanHallData.All() {
		if slices.Contains(h.NPCs, npcID) {
			return h, true
		}
	}
	return nil, false
}

// clanHallManagerBypass runs command, a clan hall manager's dialog command,
// for live at f. Only a member of the clan owning f's hall is answered.
// functions opens the function services, manage the function rental,
// support casts support magic, support_back and list_back go back to the
// support list and the main page; a member lacking the privilege a command
// needs is told so. teleport and instant_teleport take a member allowed
// the functions to a destination. A malformed number answers nothing.
func (l *GameClientLink) clanHallManagerBypass(live *livePlayer, f *npc.Folk, command string) {
	hall, ok := l.hallOfManager(f.NpcID())
	if !ok {
		return
	}
	cl, ok := l.clanService().ClanOf(live.Character)
	if !ok {
		return
	}
	if v, ok := l.halls.View(int32(hall.ID)); !ok || v.OwnerID != cl.ID() {
		return
	}
	tokens := strings.FieldsFunc(command, func(r rune) bool { return r == ' ' })
	if len(tokens) == 0 {
		return
	}
	d := hallDialog{l: l, live: live, f: f, hall: hall, cl: cl}
	val := ""
	if len(tokens) > 1 {
		val = tokens[1]
	}
	switch actual := tokens[0]; {
	case strings.EqualFold(actual, "functions"):
		d.functions(val, tokens)
	case strings.EqualFold(actual, "manage"):
		d.manage(val, tokens)
	case strings.EqualFold(actual, "support"):
		d.support(val, tokens)
	case strings.EqualFold(actual, "list_back"):
		d.send(fillPage(d.page("chamberlain.htm"), "%npcname%", f.CharacterName(), "%objectId%", objID(f)))
	case strings.EqualFold(actual, "support_back"):
		d.supportBack()
	case strings.EqualFold(actual, "banish_foreigner"), strings.EqualFold(actual, "manage_vault"), strings.EqualFold(actual, "door"),
		strings.EqualFold(actual, "WithdrawC"), strings.EqualFold(actual, "DepositC"):
		// ponytail: the door, banishment, vault and clan warehouse commands
		// are ported with the hall's doors and banishment (#3364) in #3429.
		l.log.Debug().Int("npc_id", f.NpcID()).Str("command", command).Msg("bypass: clan hall manager command not modeled")
	default:
		d.npcCommand(command, tokens)
	}
}

// npcCommand runs the commands every NPC answers that a clan hall manager
// takes part in: teleport_request lists the standard destinations, and
// teleport <index> and instant_teleport <index> take a member allowed the
// functions to a destination. An index that is missing or does not parse
// only releases the client.
func (d hallDialog) npcCommand(command string, tokens []string) {
	switch {
	case command == "teleport_request":
		d.l.showTeleportList(d.live, d.f)
	case strings.HasPrefix(command, "teleport"), strings.HasPrefix(command, "instant_teleport"):
		if len(tokens) < 2 {
			d.live.SendFrame(serverpackets.FrameActionFailed())
			return
		}
		index, err := commons.ParseInt(tokens[1], 32)
		if err != nil {
			d.live.SendFrame(serverpackets.FrameActionFailed())
			return
		}
		if !d.allowed(clan.PrivHallFunctions) {
			return
		}
		if strings.HasPrefix(command, "instant_teleport") {
			d.l.departFromNpc(d.live, d.l.gatekeeper.Instant(d.f.NpcID(), int(index)))
			return
		}
		d.l.departFromNpc(d.live, d.l.gatekeeper.Teleport(d.live.Character, d.f.NpcID(), int(index)))
	default:
		d.l.log.Debug().Int("npc_id", d.f.NpcID()).Str("command", command).Msg("bypass: clan hall manager command not modeled")
	}
}

func (d hallDialog) hallID() int32 { return int32(d.hall.ID) }

// page is the manager page file, set as an HTML window takes it.
func (d hallDialog) page(file string) string { return d.l.setPage(hallManagerPages + file) }

// send opens page as the manager's window.
func (d hallDialog) send(page string) { sendFilledHTML(d.live, d.f.ObjectID(), page, 0) }

// sendPage opens the manager page file with its object id filled.
func (d hallDialog) sendPage(file string) {
	d.send(fillPage(d.page(file), "%objectId%", objID(d.f)))
}

// allowed reports whether the talker holds privilege p in its clan, and
// otherwise opens the refusal page.
func (d hallDialog) allowed(p clan.Privilege) bool {
	if d.cl.HasPrivilege(d.live.ObjectID(), p) {
		return true
	}
	d.send(d.page("not_authorized.htm"))
	return false
}

// function is the hall's function of type funcType.
func (d hallDialog) function(funcType int) (clanhall.Function, bool) {
	return d.l.hallFunctions.Get(d.hallID(), funcType)
}

// functions answers "functions [service]": the teleport list, the item
// creation buy window, the support magic list, or the overview of the
// recovery levels. A service the hall does not rent opens the disabled
// page.
func (d hallDialog) functions(val string, tokens []string) {
	if !d.allowed(clan.PrivHallFunctions) {
		return
	}
	switch {
	case strings.EqualFold(val, "tele"):
		fn, ok := d.function(hallmodel.FuncTeleport)
		if !ok {
			d.sendPage("functions-disabled.htm")
			return
		}
		kind := travel.KindClanHallFunctionLevel1
		if fn.Level == 2 {
			kind = travel.KindClanHallFunctionLevel2
		}
		if page, ok := d.l.gatekeeper.Window(d.f.ObjectID(), d.f.NpcID(), kind, d.live.ObjectID()); ok {
			sendValidatedHTML(d.live, d.f.ObjectID(), page, 0)
		}
	case strings.EqualFold(val, "item_creation"):
		if len(tokens) < 3 {
			return
		}
		fn, ok := d.function(hallmodel.FuncCreateItem)
		if !ok {
			d.sendPage("functions-disabled.htm")
			return
		}
		list, err := commons.ParseInt(tokens[2], 32)
		if err != nil {
			return
		}
		d.l.showBuyWindow(d.live, d.f, int(int32(list)+int32(fn.Level)*100000))
	case strings.EqualFold(val, "support"):
		fn, ok := d.function(hallmodel.FuncSupportMagic)
		if !ok {
			d.sendPage("functions-disabled.htm")
			return
		}
		d.send(fillPage(d.page("support"+strconv.Itoa(fn.Level)+".htm"),
			"%mp%", strconv.Itoa(int(d.f.MPValue())), "%objectId%", objID(d.f)))
	default:
		level := func(funcType int) string {
			return strconv.Itoa(d.l.hallFunctions.Level(d.hallID(), funcType))
		}
		d.send(fillPage(d.page("functions.htm"),
			"%npcId%", strconv.Itoa(d.f.NpcID()), "%objectId%", objID(d.f),
			"%hp_regen%", level(hallmodel.FuncRestoreHP), "%mp_regen%", level(hallmodel.FuncRestoreMP),
			"%xp_regen%", level(hallmodel.FuncRestoreExp)))
	}
}

// support answers "support <skill> [level]": the manager casts the skill on
// the talker on its next AI tick, once the hall rents support magic. A
// cursed weapon holder is refused with a notice; a skill or level that does
// not parse is answered with a notice, and an unknown one with nothing.
func (d hallDialog) support(val string, tokens []string) {
	if !d.allowed(clan.PrivHallFunctions) {
		return
	}
	if fn, ok := d.function(hallmodel.FuncSupportMagic); !ok || fn.Level == 0 {
		return
	}
	if d.live.CursedWeaponEquipped() {
		sendText(d.live, supportCursedMessage)
		return
	}
	id, err := commons.ParseInt(val, 32)
	if err != nil {
		sendText(d.live, supportInvalidMessage)
		return
	}
	level := int64(0)
	if len(tokens) > 2 {
		if level, err = commons.ParseInt(tokens[2], 32); err != nil {
			sendText(d.live, supportInvalidMessage)
			return
		}
	}
	d.f.AddCastDesire(d.live.Character, modelskill.Ref{ID: modelskill.ID(id), Level: int(level)}, supportCastWeight)
}

// supportBack opens the support magic list again, once the hall rents
// support magic.
func (d hallDialog) supportBack() {
	if !d.allowed(clan.PrivHallFunctions) {
		return
	}
	fn, ok := d.function(hallmodel.FuncSupportMagic)
	if !ok || fn.Level == 0 {
		return
	}
	d.send(fillPage(d.page("support"+strconv.Itoa(fn.Level)+".htm"),
		"%mp%", strconv.Itoa(int(d.f.MPValue())), "%objectId%", objID(d.f)))
}

// manage answers "manage [group ...]", for a member allowed to set the
// functions: the recovery, other and decoration groups each list their
// functions or run a change; back opens the main page; anything else the
// management menu.
func (d hallDialog) manage(val string, tokens []string) {
	if !d.allowed(clan.PrivHallSetFuncs) {
		return
	}
	switch {
	case strings.EqualFold(val, "recovery"):
		if len(tokens) > 2 {
			d.recoveryCommand(tokens[2:])
			return
		}
		d.recoveryPage()
	case strings.EqualFold(val, "other"):
		if len(tokens) > 2 {
			d.otherCommand(tokens[2:])
			return
		}
		d.otherPage()
	case strings.EqualFold(val, "deco"):
		if len(tokens) > 2 {
			d.decoCommand(tokens[2:])
			return
		}
		d.decoPage()
	case strings.EqualFold(val, "back"):
		d.live.SendFrame(serverpackets.FrameActionFailed())
		d.sendPage("chamberlain.htm")
	default:
		file := "manage.htm"
		if d.hall.IsSiegable() {
			file = "manage_sch.htm"
		}
		d.sendPage(file)
	}
}

// hallChange is one function a management group rents out: its type, how
// a level named in a link becomes the level it is rented at, and how its
// pages describe it.
type hallChange struct {
	funcType int
	// over is the link level above which 10 is taken off; mult turns the
	// result into the rented level.
	over, mult int
	// name and use describe the function on its offer page; use reads the
	// shown level.
	name string
	use  func(shown int) string
	// stageUsed names the function's current level on the unchanged page
	// as "Stage <command word>" instead of the shown level and a percent.
	stageUsed bool
	// stageShown is "Stage <shown level>" on the unchanged page.
	stageShown bool
}

var (
	hpChange = hallChange{funcType: hallmodel.FuncRestoreHP, over: 20, mult: 20, name: "Fireplace (HP Recovery Device)", use: func(shown int) string {
		return `Provides additional HP recovery for clan members in the clan hall.<font color="00FFFF">` + strconv.Itoa(shown*20) + "%</font>"
	}}
	mpChange = hallChange{funcType: hallmodel.FuncRestoreMP, over: 10, mult: 5, name: "Carpet (MP Recovery)", use: func(shown int) string {
		return `Provides additional MP recovery for clan members in the clan hall.<font color="00FFFF">` + strconv.Itoa(shown*5) + "%</font>"
	}}
	expChange = hallChange{funcType: hallmodel.FuncRestoreExp, over: 10, mult: 5, name: "Chandelier (EXP Recovery Device)", use: func(shown int) string {
		return `Restores the Exp of any clan member who is resurrected in the clan hall.<font color="00FFFF">` + strconv.Itoa(shown*5) + "%</font>"
	}}
	itemChange = hallChange{funcType: hallmodel.FuncCreateItem, over: 10, mult: 1, name: "Magic Equipment (Item Production Facilities)", stageUsed: true, use: func(int) string {
		return "Allow the purchase of special items at fixed intervals."
	}}
	supportChange = hallChange{funcType: hallmodel.FuncSupportMagic, over: 10, mult: 1, name: "Insignia (Supplementary Magic)", stageUsed: true, use: func(int) string {
		return "Enables the use of supplementary magic."
	}}
	teleChange = hallChange{funcType: hallmodel.FuncTeleport, over: 10, mult: 1, name: "Mirror (Teleportation Device)", stageShown: true, use: func(shown int) string {
		return `Teleports clan members in a clan hall to the target <font color="00FFFF">Stage ` + strconv.Itoa(shown) + "</font> staging area"
	}}
	curtainsChange = hallChange{funcType: hallmodel.FuncDecoCurtains, over: 10, mult: 1, name: "Curtains (Decoration)", stageUsed: true, use: func(int) string {
		return "These curtains can be used to decorate the clan hall."
	}}
	fixturesChange = hallChange{funcType: hallmodel.FuncDecoFixtures, over: 10, mult: 1, name: "Front Platform (Decoration)", stageUsed: true, use: func(int) string {
		return "Used to decorate the clan hall."
	}}
)

// shown is the level a link level names once 10 is taken off one above
// c.over.
func (c hallChange) shown(link int) int {
	if link > c.over {
		return link - 10
	}
	return link
}

// recoveryCommand runs "manage recovery <command> [level]".
func (d hallDialog) recoveryCommand(args []string) {
	switch cmd := args[0]; {
	case strings.EqualFold(cmd, "hp_cancel"):
		d.cancelPage("recovery hp 0")
	case strings.EqualFold(cmd, "mp_cancel"):
		d.cancelPage("recovery mp 0")
	case strings.EqualFold(cmd, "exp_cancel"):
		d.cancelPage("recovery exp 0")
	case strings.EqualFold(cmd, "edit_hp"):
		d.offer(hpChange, "recovery hp ", args)
	case strings.EqualFold(cmd, "edit_mp"):
		d.offer(mpChange, "recovery mp ", args)
	case strings.EqualFold(cmd, "edit_exp"):
		d.offer(expChange, "recovery exp ", args)
	case strings.EqualFold(cmd, "hp"):
		d.change(hpChange, args)
	case strings.EqualFold(cmd, "mp"):
		d.change(mpChange, args)
	case strings.EqualFold(cmd, "exp"):
		// The reference takes 10 off an experience recovery level above 20
		// here, where its offer page does so above 10.
		c := expChange
		c.over = 20
		d.change(c, args)
	}
}

// otherCommand runs "manage other <command> [level]".
func (d hallDialog) otherCommand(args []string) {
	switch cmd := args[0]; {
	case strings.EqualFold(cmd, "item_cancel"):
		d.cancelPage("other item 0")
	case strings.EqualFold(cmd, "tele_cancel"):
		d.cancelPage("other tele 0")
	case strings.EqualFold(cmd, "support_cancel"):
		d.cancelPage("other support 0")
	case strings.EqualFold(cmd, "edit_item"):
		d.offer(itemChange, "other item ", args)
	case strings.EqualFold(cmd, "edit_support"):
		d.offer(supportChange, "other support ", args)
	case strings.EqualFold(cmd, "edit_tele"):
		d.offer(teleChange, "other tele ", args)
	case strings.EqualFold(cmd, "item"):
		d.change(itemChange, args)
	case strings.EqualFold(cmd, "tele"):
		d.change(teleChange, args)
	case strings.EqualFold(cmd, "support"):
		d.change(supportChange, args)
	}
}

// decoCommand runs "manage deco <command> [level]".
func (d hallDialog) decoCommand(args []string) {
	switch cmd := args[0]; {
	case strings.EqualFold(cmd, "curtains_cancel"):
		d.cancelPage("deco curtains 0")
	case strings.EqualFold(cmd, "fixtures_cancel"):
		d.cancelPage("deco fixtures 0")
	case strings.EqualFold(cmd, "edit_curtains"):
		d.offer(curtainsChange, "deco curtains ", args)
	case strings.EqualFold(cmd, "edit_fixtures"):
		d.offer(fixturesChange, "deco fixtures ", args)
	case strings.EqualFold(cmd, "curtains"):
		d.change(curtainsChange, args)
	case strings.EqualFold(cmd, "fixtures"):
		d.change(fixturesChange, args)
	}
}

// cancelPage asks to confirm a function's removal through apply.
func (d hallDialog) cancelPage(apply string) {
	d.send(fillPage(d.page("functions-cancel.htm"), "%apply%", apply, "%objectId%", objID(d.f)))
}

// linkLevel is the level args[1] names, false when it is missing or does
// not parse.
func linkLevel(args []string) (int, bool) {
	if len(args) < 2 {
		return 0, false
	}
	n, err := commons.ParseInt(args[1], 32)
	if err != nil {
		return 0, false
	}
	return int(n), true
}

// offer opens the page offering c at the level args names: its name, its
// fee and term at that level, what it does, and the command renting it.
func (d hallDialog) offer(c hallChange, apply string, args []string) {
	link, ok := linkLevel(args)
	if !ok {
		return
	}
	decos := d.l.hallFunctions.Decos()
	cost := strconv.Itoa(decos.Fee(c.funcType, link)) + "</font> Adena / " + strconv.Itoa(decos.Days(c.funcType, link)) + " day(s)</font>)"
	d.send(fillPage(d.page("functions-apply.htm"),
		"%name%", c.name, "%cost%", cost, "%use%", c.use(c.shown(link)),
		"%apply%", apply+strconv.Itoa(link), "%objectId%", objID(d.f)))
}

// change rents, changes or, at level 0, cancels c at the level args names,
// the fee and term of that level per clanHallDeco.xml. A function already
// at that level is left as is. The first fee comes out of the talker's
// adena; short of it, the low adena page opens. A change shows the hall's
// new decorations to everyone inside it; one to support magic has the
// manager check its own buff on its next idle tick.
func (d hallDialog) change(c hallChange, args []string) {
	link, ok := linkLevel(args)
	if !ok {
		return
	}
	decos := d.l.hallFunctions.Decos()
	days, cost := decos.Days(c.funcType, link), decos.Fee(c.funcType, link)
	shown := c.shown(link)
	level := shown * c.mult
	if fn, ok := d.function(c.funcType); ok && fn.Level == level {
		used := strconv.Itoa(shown) + "%"
		switch {
		case c.stageUsed:
			used = "Stage " + args[0]
		case c.stageShown:
			used = "Stage " + strconv.Itoa(shown)
		}
		d.send(fillPage(d.page("functions-used.htm"), "%val%", used, "%objectId%", objID(d.f)))
		return
	}
	file, fee := "functions-apply_confirmed.htm", cost
	if level == 0 {
		file, fee = "functions-cancel_confirmed.htm", 0
	} else if _, priced := decos.Get(c.funcType, link); !priced {
		// The reference rents a level clanHallDeco.xml does not price (the
		// grade 2 hall's "260%" fireplace link) for free, with a term of no
		// length; it is refused here.
		d.l.log.Warn().Int32("hall_id", d.hallID()).Int("type", c.funcType).Int("level", link).Msg("clan hall: function level without a fee refused")
		return
	}
	if fee > 0 && !reduceAdena(d.live, fee) {
		file = "low_adena.htm"
	} else {
		d.l.hallFunctions.Update(d.hallID(), c.funcType, level, fee, int64(days)*hallFunctionDayMs)
		if c.funcType == hallmodel.FuncSupportMagic {
			d.f.ResetSupportBuffCheck()
		}
		d.l.broadcastHallDecoration(d.hallID())
	}
	d.sendPage(file)
}

// rentedLine is how a function page shows fn: prefix (the recovery percent
// or "Stage <level>"), its lease and term, sep between the level and the
// lease; and when its next fee is due.
func (d hallDialog) rentedLine(fn clanhall.Function, prefix, sep string) (rented, period string) {
	days := d.l.hallFunctions.Decos().Days(fn.Type, clanhall.DecoLevel(fn.Type, fn.Level, d.hall.IsSiegable()))
	rented = prefix + "</font>" + sep + `(<font color="FFAABB">` + strconv.Itoa(fn.Lease) + "</font> Adena / " + strconv.Itoa(days) + " day(s))"
	return rented, "Next fee at " + auctionTime(fn.EndTime, hallFeeDateLayout)
}

// fillFunction fills key's three placeholders on page for the hall's
// function of type funcType: value, what it rents (none without one), its
// period,
// and its change links, after a removal link when rented. links is "" for
// a grade without links, which leaves the change placeholder as is.
func (d hallDialog) fillFunction(page, key, value string, funcType int, percent bool, sep, remove, links string) string {
	fn, ok := d.function(funcType)
	if !ok {
		page = fillPage(page, value, hallFunctionNone, "%"+key+"_period%", hallFunctionNone)
	} else {
		prefix := "Stage " + strconv.Itoa(fn.Level)
		if percent {
			prefix = strconv.Itoa(fn.Level) + "%"
		}
		rented, period := d.rentedLine(fn, prefix, sep)
		page = fillPage(page, value, rented, "%"+key+"_period%", period)
		if links != "" {
			links = remove + links
		}
	}
	if links == "" {
		return page
	}
	return fillPage(page, "%change_"+key+"%", links)
}

// gradeLinks is links for the hall's grade, "" for a grade without any.
func (d hallDialog) gradeLinks(links gradeLinks) string {
	if d.hall.IsSiegable() {
		return links.siegable[d.hall.Grade]
	}
	return links.plain[d.hall.Grade]
}

// siegeForm is plain, or siegable for a siegable hall.
func (d hallDialog) siegeForm(plain, siegable string) string {
	if d.hall.IsSiegable() {
		return siegable
	}
	return plain
}

// recoveryPage lists the HP, experience and MP recovery functions.
func (d hallDialog) recoveryPage() {
	page := d.page("edit_recovery.htm")
	page = d.fillFunction(page, "hp", "%hp_recovery%", hallmodel.FuncRestoreHP, true, " ", removeHP, d.gradeLinks(hpLinks))
	page = d.fillFunction(page, "exp", "%exp_recovery%", hallmodel.FuncRestoreExp, true, " ", removeExp, d.gradeLinks(expLinks))
	page = d.fillFunction(page, "mp", "%mp_recovery%", hallmodel.FuncRestoreMP, true, " ", removeMP, d.gradeLinks(mpLinks))
	d.send(fillPage(page, "%objectId%", objID(d.f)))
}

// otherPage lists the teleport, support magic and item creation functions.
func (d hallDialog) otherPage() {
	page := d.page("edit_other.htm")
	page = d.fillFunction(page, "tele", "%tele%", hallmodel.FuncTeleport, false, " ", removeTele, d.siegeForm(teleLevels, teleLevelsSCH))
	page = d.fillFunction(page, "support", "%support%", hallmodel.FuncSupportMagic, false, " ", removeSupport, d.gradeLinks(supportLinks))
	page = d.fillFunction(page, "item", "%item%", hallmodel.FuncCreateItem, false, " ", removeItem, d.siegeForm(itemLevels, itemLevelsSCH))
	d.send(fillPage(page, "%objectId%", objID(d.f)))
}

// decoPage lists the curtains and front platform decorations.
func (d hallDialog) decoPage() {
	page := d.page("deco.htm")
	page = d.fillFunction(page, "curtain", "%curtain%", hallmodel.FuncDecoCurtains, false, "&nbsp;", removeCurtains, curtainLevels)
	page = d.fillFunction(page, "fixture", "%fixture%", hallmodel.FuncDecoFixtures, false, "&nbsp;", removeFixtures, fixtureLevels)
	d.send(fillPage(page, "%objectId%", objID(d.f)))
}

// hallManagerSelfBuff lands on manager f the buff its idle AI checks once
// every five minutes: 4366 plus the level of its hall's support magic
// function, 4367 without one. A manager of no hall, or a buff with no
// skill data, lands nothing.
func (l *GameClientLink) hallManagerSelfBuff(f *npc.Folk) {
	hall, ok := l.hallOfManager(f.NpcID())
	if !ok || l.skills == nil {
		return
	}
	id := hallManagerBuffDefault
	if fn, ok := l.hallFunctions.Get(int32(hall.ID), hallmodel.FuncSupportMagic); ok {
		id = hallManagerBuffBase + fn.Level
	}
	def, ok := l.skills.Definition(modelskill.Ref{ID: modelskill.ID(id), Level: 1})
	if !ok {
		return
	}
	skillhandler.LandEffects(f, f, def)
}

// hallSupportAnswer tells the player a clan hall manager cast support magic
// on, when still in the world, how it went and the manager's MP.
func (l *GameClientLink) hallSupportAnswer(f *npc.Folk, e event.HallSupportCast) {
	live, ok := l.livePlayerByID(e.PlayerID)
	if !ok {
		return
	}
	file := "support-done.htm"
	if e.NoMana {
		file = "support-no_mana.htm"
	}
	page := fillPage(l.setPage(hallManagerPages+file), "%mp%", strconv.Itoa(e.MP), "%objectId%", objID(f))
	sendFilledHTML(live, f.ObjectID(), page, 0)
}

// broadcastHallDecoration shows hall hallID's decorations to every player
// inside its grounds.
func (l *GameClientLink) broadcastHallDecoration(hallID int32) {
	for _, za := range l.hallOccupants(hallID) {
		l.showClanHallInterior(hallID, za)
	}
}
