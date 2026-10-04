package npc

import (
	"strconv"
	"strings"

	"github.com/fatal10110/acis_golang/internal/gameserver/sevensigns"
)

// ChatState is what the chat windows of the Seven Signs priests, the
// festival guides and the Olympiad managers read of the talker and the
// world.
type ChatState interface {
	// SevenSigns is the talker's Record of Seven Signs.
	SevenSigns() sevensigns.Record
	// FestivalNotice is the festival guides' line on when the next
	// festival begins.
	FestivalNotice() string
	// Noble reports whether the talker is a noble.
	Noble() bool
	// Hero reports whether the talker is a hero, and inactive whether it
	// was elected hero and has not claimed the status yet.
	Hero() (hero, inactive bool)
}

const (
	sevenSignsPages = "data/html/seven_signs/"
	olympiadPages   = "data/html/olympiad/"
)

// The Mammon NPCs, the festival guides and witches of each oracle, the
// Grand Olympiad Manager and the Monuments of Heroes, by NPC id.
const (
	blackMarketeerOfMammon = 31092
	merchantOfMammon       = 31113
	blacksmithOfMammon     = 31126
	grandOlympiadManager   = 31688
)

var (
	dawnFestivalGuides = idSet(31127, 31128, 31129, 31130, 31131)
	duskFestivalGuides = idSet(31137, 31138, 31139, 31140, 31141)
	festivalWitches    = idSet(31132, 31133, 31134, 31135, 31136, 31142, 31143, 31144, 31145, 31146)
	monuments          = idSet(31690, 31769, 31770, 31771, 31772)
)

func idSet(ids ...int) map[int]struct{} {
	set := make(map[int]struct{}, len(ids))
	for _, id := range ids {
		set[id] = struct{}{}
	}
	return set
}

// signsChat resolves the first chat page of a Seven Signs priest, a
// festival guide or an Olympiad manager; ok is false for any other type.
func (f *Folk) signsChat(pages Pages, kind InstanceKind, state ChatState) (page string, outcome ChatOutcome, ok bool) {
	switch kind {
	case "DawnPriest":
		return f.page(pages, sevenSignsPages+priestPage("dawn", sevensigns.Dawn, state.SevenSigns())), ChatShownReleased, true
	case "DuskPriest":
		return f.page(pages, sevenSignsPages+priestPage("dusk", sevensigns.Dusk, state.SevenSigns())), ChatShownReleased, true
	case "SignsPriest":
		page, outcome = f.mammonChat(pages, state.SevenSigns())
		return page, outcome, true
	case "FestivalGuide":
		page = f.page(pages, festivalGuidePath(f.NpcID()))
		return strings.ReplaceAll(page, "%festivalMins%", state.FestivalNotice()), ChatShown, true
	case "OlympiadManagerNpc":
		hero, inactive := state.Hero()
		return f.olympiadChat(pages, 0, state.Noble(), hero, inactive), ChatShown, true
	}
	return "", ChatShown, false
}

// olympiadChat is an Olympiad manager's chat page val: a Monument of
// Heroes' main page, whatever val; else noble.htm for val 0 or below, or
// noble_<val>.htm, with the Grand Olympiad Manager's own page 0 for a
// noble, noble_main.htm.
func (f *Folk) olympiadChat(pages Pages, val int, noble, hero, inactive bool) string {
	if _, ok := monuments[f.NpcID()]; ok {
		if hero || inactive {
			return f.heroMainPage(pages, inactive)
		}
		return f.page(pages, olympiadPages+"hero_main2.htm")
	}
	name := "noble.htm"
	if val > 0 {
		name = "noble_" + strconv.Itoa(val) + ".htm"
	}
	if f.NpcID() == grandOlympiadManager && noble && val == 0 {
		name = "noble_main.htm"
	}
	return f.page(pages, olympiadPages+name)
}

// heroClaimLink is the line of a Monument of Heroes' main page that lets an
// elected hero claim the status.
const heroClaimLink = `<a action="bypass -h npc_%objectId%_Olympiad 5">"I want to be a Hero."</a><br>`

// heroMainPage is a Monument of Heroes' main page, offering the claim of
// the hero status when inactive is set.
func (f *Folk) heroMainPage(pages Pages, inactive bool) string {
	const path = olympiadPages + "hero_main.htm"
	page, ok := pages.Get(path)
	if !ok {
		return f.page(pages, path)
	}
	link := ""
	if inactive {
		link = heroClaimLink
	}
	return strings.ReplaceAll(strings.ReplaceAll(page, "%hero%", link), "%objectId%", strconv.Itoa(int(f.ObjectID())))
}

// priestPage names the page a priest of own's cabal greets r's player
// with, prefix naming the cabal's pages: the rival cabal's members are
// turned away; otherwise the page follows the period, and during seal
// validation the competition's outcome. A member of a winning cabal that
// does not own the Seal of Gnosis gets its own page.
func priestPage(prefix string, own sevensigns.Cabal, r sevensigns.Record) string {
	member := r.PlayerCabal == own
	page := "1"
	switch {
	case r.PlayerCabal != sevensigns.NoCabal && !member:
		page = "3"
	case r.Period == sevensigns.Results:
		page = "5"
	case r.Period == sevensigns.Recruiting:
		page = "6"
	case r.Period != sevensigns.SealValidation:
	case r.Winner == own && !member:
		page = "4"
	case r.Winner == own && r.Seals[1].Owner != own:
		page = "2c"
	case r.Winner == own:
		page = "2a"
	case r.Winner == sevensigns.NoCabal:
		page = "2d"
	default:
		page = "2b"
	}
	return prefix + "_priest_" + page + ".htm"
}

// mammonChat is a Mammon NPC's first page. The merchant and the blacksmith
// serve only members of the winning cabal that owns the Seal of Avarice,
// respectively of Gnosis; with no winner, the merchant serves nobody and
// the blacksmith everybody.
func (f *Folk) mammonChat(pages Pages, r sevensigns.Record) (string, ChatOutcome) {
	var name string
	switch f.NpcID() {
	case blackMarketeerOfMammon:
		name = "blkmrkt_1.htm"
	case merchantOfMammon:
		if r.Winner == sevensigns.NoCabal {
			return "", ChatCompetitionOnly
		}
		if refusal, refused := mammonRefusal(r, r.Seals[0].Owner); refused {
			return "", refusal
		}
		name = "mammmerch_1.htm"
	case blacksmithOfMammon:
		if refusal, refused := mammonRefusal(r, r.Seals[1].Owner); refused {
			return "", refusal
		}
		name = "mammblack_1.htm"
	default:
		return f.chatPage(pages, folkChat{}, 0), ChatShown
	}
	return f.page(pages, sevenSignsPages+name), ChatShown
}

// mammonRefusal refuses r's player unless they belong to the winning cabal
// and it owns the seal owner stands for.
func mammonRefusal(r sevensigns.Record, owner sevensigns.Cabal) (ChatOutcome, bool) {
	if r.PlayerCabal == r.Winner && r.PlayerCabal == owner {
		return ChatShown, false
	}
	switch r.Winner {
	case sevensigns.Dawn:
		return ChatDawnOnly, true
	case sevensigns.Dusk:
		return ChatDuskOnly, true
	}
	return ChatShown, false
}

// festivalGuidePath is a festival guide's or witch's page: each oracle's
// guides share one, all witches another. An id outside both reads the
// pages folder itself, which no page answers.
func festivalGuidePath(npcID int) string {
	switch {
	case has(dawnFestivalGuides, npcID):
		return sevenSignsPages + "festival/dawn_guide.htm"
	case has(duskFestivalGuides, npcID):
		return sevenSignsPages + "festival/dusk_guide.htm"
	case has(festivalWitches, npcID):
		return sevenSignsPages + "festival/festival_witch.htm"
	}
	return sevenSignsPages
}

func has(set map[int]struct{}, id int) bool {
	_, ok := set[id]
	return ok
}
