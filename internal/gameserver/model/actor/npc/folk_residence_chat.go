package npc

import (
	"strconv"
	"strings"

	"github.com/fatal10110/acis_golang/internal/gameserver/clan"
	"github.com/fatal10110/acis_golang/internal/gameserver/sevensigns"
)

// ResidenceTalk is what the chat window of an NPC serving a castle or a
// clan hall reads: that residence, its owner and its siege, and the
// talker's place in the clan it belongs to.
type ResidenceTalk struct {
	// CastleID is the castle the NPC serves and HallID the clan hall; both
	// are 0 for an NPC serving neither. Siegable marks a siegable hall, and
	// HallName names the hall, CastleName the castle.
	CastleID, HallID int
	Siegable         bool
	HallName         string
	CastleName       string
	// OwnerID is the clan owning the residence, 0 for none. OwnerName names
	// it, and OwnerLeader its leader, empty when the clan has none.
	OwnerID     int32
	OwnerName   string
	OwnerLeader string
	// SiegeInProgress is set while the castle's siege, or the siegable
	// hall's, is under way; BattlefieldActive while the castle's
	// battlefield is.
	SiegeInProgress   bool
	BattlefieldActive bool
	// LifeTowers is the number of the castle's life control towers still
	// standing.
	LifeTowers int
	// ClanID is the talker's clan, 0 for none; ClanLeader is set when the
	// talker leads it, and Privileges are the clan privileges it holds.
	ClanID     int32
	ClanLeader bool
	Privileges clan.Privilege
	// StrifeOwner is the cabal owning the Seal of Strife, and
	// SealValidation is set during the seal validation period.
	StrifeOwner    sevensigns.Cabal
	SealValidation bool
	// Flying is set while the talker flies a wyvern.
	Flying bool
}

// owns reports whether the talker's clan owns the residence.
func (r ResidenceTalk) owns() bool { return r.ClanID != 0 && r.ClanID == r.OwnerID }

// holds reports whether the talker holds privilege p in its clan.
func (r ResidenceTalk) holds(p clan.Privilege) bool { return r.Privileges&p != 0 }

// castleTalk is how a castle NPC answers the talker: as a stranger, while
// its castle's siege runs, or as a member of the clan owning it.
type castleTalk int

const (
	talkStranger castleTalk = iota
	talkUnderSiege
	talkOwner
)

// castleTalkCond is the talker's standing at a castle NPC. Only a clan
// member at an NPC of a castle is answered as more than a stranger: while
// busy reports a siege, as under siege; as the owner when its clan owns the
// castle and, when need is not PrivNone, holds that privilege.
func (r ResidenceTalk) castleTalkCond(busy bool, need clan.Privilege) castleTalk {
	if r.CastleID == 0 || r.ClanID == 0 {
		return talkStranger
	}
	if busy {
		return talkUnderSiege
	}
	if r.owns() && (need == clan.PrivNone || r.holds(need)) {
		return talkOwner
	}
	return talkStranger
}

// residenceChat resolves the first chat page of an NPC serving a castle or
// a clan hall; ok is false for any other type.
func (f *Folk) residenceChat(pages Pages, kind InstanceKind, rules ChatRules, state ChatState) (page string, outcome ChatOutcome, ok bool) {
	switch kind {
	case "CastleBlacksmith", "CastleChamberlain", "CastleDoorman", "CastleGatekeeper", "CastleMagician", "CastleWarehouseKeeper",
		"ClanHallDoorman", "ClanHallManagerNpc", "Doorman", "ManorManagerNpc", "MercenaryManagerNpc", "WyvernManagerNpc":
	default:
		return "", ChatShown, false
	}
	if kind == "Doorman" {
		return f.page(pages, "data/html/doormen/"+strconv.Itoa(f.NpcID())+".htm"), ChatShown, true
	}
	r := state.Residence()
	switch kind {
	case "CastleBlacksmith":
		page, outcome = f.blacksmithChat(pages, rules, r)
	case "CastleChamberlain":
		page, outcome = f.castlePage(pages, r.castleTalkCond(r.SiegeInProgress, clan.PrivNone), "data/html/chamberlain/", "no.htm", "busy.htm", "chamberlain.htm"), ChatShownReleased
	case "CastleDoorman":
		page, outcome = f.castleDoormanChat(pages, r), ChatShown
	case "CastleGatekeeper":
		page, outcome = f.massGatekeeperChat(pages, r)
	case "CastleMagician":
		page, outcome = f.castlePage(pages, r.castleTalkCond(r.BattlefieldActive, clan.PrivNone), "data/html/castlemagician/", "magician-no.htm", "magician-busy.htm", "magician.htm"), ChatShownReleased
	case "CastleWarehouseKeeper":
		page = f.castlePage(pages, r.castleTalkCond(r.SiegeInProgress, clan.PrivNone), "data/html/castlewarehouse/", "castlewarehouse-no.htm", "castlewarehouse-busy.htm", "castlewarehouse.htm")
		page, outcome = strings.ReplaceAll(page, "%npcname%", f.CharacterName()), ChatShownReleased
	case "ClanHallDoorman":
		page, outcome = f.hallDoormanChat(pages, r)
	case "ClanHallManagerNpc":
		file := "chamberlain-no.htm"
		if r.HallID != 0 && r.owns() {
			file = "chamberlain.htm"
		}
		page, outcome = f.page(pages, "data/html/clanHallManager/"+file), ChatShownReleased
	case "ManorManagerNpc":
		page, outcome = f.manorManagerChat(pages, rules, r), ChatShown
	case "MercenaryManagerNpc":
		page, outcome = f.mercenaryManagerChat(pages, r), ChatPageOnly
	case "WyvernManagerNpc":
		page, outcome = f.wyvernManagerChat(pages, rules, r), ChatShown
	}
	return page, outcome, true
}

// castlePage is the page of dir a castle NPC opens for a talker of
// standing cond: stranger, underSiege or owner.
func (f *Folk) castlePage(pages Pages, cond castleTalk, dir, stranger, underSiege, owner string) string {
	file := stranger
	switch cond {
	case talkUnderSiege:
		file = underSiege
	case talkOwner:
		file = owner
	case talkStranger:
	}
	return f.page(pages, dir+file)
}

// blacksmithChat is a castle blacksmith's chat window, sent alone: the
// generic page while the manor is off; otherwise castleblacksmith.htm for a
// member of the owning clan allowed to run the manor, the busy page while
// the siege runs, and the refusal page for anyone else. A blacksmith of no
// castle opens nothing.
func (f *Folk) blacksmithChat(pages Pages, rules ChatRules, r ResidenceTalk) (string, ChatOutcome) {
	if !rules.AllowManor {
		return strings.ReplaceAll(f.page(pages, "data/html/npcdefault.htm"), "%npcname%", f.CharacterName()), ChatPageOnly
	}
	if r.CastleID == 0 {
		return "", ChatSilent
	}
	page := f.castlePage(pages, r.castleTalkCond(r.SiegeInProgress, clan.PrivCastleManor), "data/html/castleblacksmith/",
		"castleblacksmith-no.htm", "castleblacksmith-busy.htm", "castleblacksmith.htm")
	page = strings.ReplaceAll(page, "%npcname%", f.CharacterName())
	return strings.ReplaceAll(page, "%castleid%", strconv.Itoa(r.CastleID)), ChatPageOnly
}

// castleDoormanChat is a castle or siegable hall doorman's chat window:
// doormen/<npcId>.htm for a member of the owning clan allowed to open its
// gates, doormen/<npcId>-no.htm for anyone else.
func (f *Folk) castleDoormanChat(pages Pages, r ResidenceTalk) string {
	file := strconv.Itoa(f.NpcID()) + "-no.htm"
	if doormanOwner("CastleDoorman", r) {
		file = strconv.Itoa(f.NpcID()) + ".htm"
	}
	return f.page(pages, "data/html/doormen/"+file)
}

// wyvernHall reports whether the doormen of clan hall id offer the wyvern
// service: Aden's halls do.
func wyvernHall(id int) bool { return id >= 36 && id <= 41 }

// hallDoormanChat is a clan hall doorman's chat window, after
// ActionFailed: its welcome, with the wyvern service at an Aden hall, for
// a member of the owning clan; for anyone else the page naming the owner
// and its leader, or the empty hall's page when no clan with a leader owns
// it. A doorman of no hall only releases the client.
func (f *Folk) hallDoormanChat(pages Pages, r ResidenceTalk) (string, ChatOutcome) {
	if r.HallID == 0 {
		return "", ChatReleasedOnly
	}
	const dir = "data/html/clanHallDoormen/"
	switch {
	case r.owns():
		file := "doormen.htm"
		if wyvernHall(r.HallID) {
			file = "doormen_wyvern.htm"
		}
		return strings.ReplaceAll(f.page(pages, dir+file), "%clanname%", r.OwnerName), ChatShownReleased
	case r.OwnerID != 0 && r.OwnerLeader != "":
		page := strings.ReplaceAll(f.page(pages, dir+"doormen-no.htm"), "%leadername%", r.OwnerLeader)
		return strings.ReplaceAll(page, "%clanname%", r.OwnerName), ChatShownReleased
	default:
		return strings.ReplaceAll(f.page(pages, dir+"emptyowner.htm"), "%hallname%", r.HallName), ChatShownReleased
	}
}

// manorManagerChat is a manor manager's chat window: the generic page
// while the manor is off, manager-lord.htm for the leader of the clan
// owning its castle, manager.htm for anyone else.
func (f *Folk) manorManagerChat(pages Pages, rules ChatRules, r ResidenceTalk) string {
	switch {
	case !rules.AllowManor:
		return f.page(pages, "data/html/npcdefault.htm")
	case r.CastleID != 0 && r.owns() && r.ClanLeader:
		return f.page(pages, "data/html/manormanager/manager-lord.htm")
	default:
		return f.page(pages, "data/html/manormanager/manager.htm")
	}
}

// mercenaryManagerChat is a mercenary manager's chat window: the refusal
// page for a stranger, the busy page while the siege runs, and for a member
// of the owning clan allowed to hire mercenaries the page of the cabal
// owning the Seal of Strife.
func (f *Folk) mercenaryManagerChat(pages Pages, r ResidenceTalk) string {
	const dir = "data/html/mercmanager/"
	owner := "mseller001.htm"
	switch r.StrifeOwner {
	case sevensigns.Dawn:
		owner = "mseller001_dawn.htm"
	case sevensigns.Dusk:
		owner = "mseller001_dusk.htm"
	case sevensigns.NoCabal:
	}
	return f.castlePage(pages, r.castleTalkCond(r.SiegeInProgress, clan.PrivCastleMercenary), dir, "mseller002.htm", "mseller003.htm", owner)
}
