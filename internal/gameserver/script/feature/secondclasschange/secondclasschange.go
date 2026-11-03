// Package secondclasschange is the masters who move a player from a first
// occupation to its second. Talking to one opens the master's pages for the
// player's race and occupation; a page's change link names the occupation,
// and a player of the right occupation and race at level 40 or above who
// holds the three marks it asks for gives them up for fifteen C-grade
// coupons and the new class. Any other link is answered as the page it
// names.
package secondclasschange

import (
	"strconv"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/player"
	"github.com/fatal10110/acis_golang/internal/gameserver/script"
)

// The marks the second occupations ask for.
const (
	markOfChallenger  = 2627
	markOfDuty        = 2633
	markOfSeeker      = 2673
	markOfScholar     = 2674
	markOfPilgrim     = 2721
	markOfDuelist     = 2762
	markOfSearcher    = 2809
	markOfReformer    = 2821
	markOfMagus       = 2840
	markOfFate        = 3172
	markOfSagittarius = 3293
	markOfWitchcraft  = 3307
	markOfSummoner    = 3336
	markOfWarspirit   = 2879
	markOfGlory       = 3203
	markOfChampion    = 3276
	markOfLord        = 3390
	markOfGuildsman   = 3119
	markOfProsperity  = 3238
	markOfMaestro     = 2867
	markOfTrust       = 2734
	markOfHealer      = 2820
	markOfLife        = 3140
)

// cGradeCoupon is the reward of every transfer.
const cGradeCoupon = 8870

// The first occupations the talk pages tell apart.
const (
	warrior        = 1
	knight         = 4
	rogue          = 7
	humanWizard    = 11
	cleric         = 15
	elvenKnight    = 19
	elvenScout     = 22
	elvenWizard    = 26
	elvenOracle    = 29
	palusKnight    = 32
	assassin       = 35
	darkWizard     = 39
	shillienOracle = 42
	orcRaider      = 45
	monk           = 47
	orcShaman      = 50
	scavenger      = 54
	artisan        = 56
)

// Masters are the masters who talk about second occupations.
func Masters() []int32 {
	return []int32{
		// Dark elf.
		31328, 30195, 30699, 30474, 31324, 30862, 30910, 31285, 31331, 31334, 31974, 32096,
		// Orc.
		30513, 30681, 30704, 30865, 30913, 31288, 31326, 31977,
		// Dwarf.
		30511, 30676, 30685, 30845, 30894, 31269, 31314, 31958,
		30512, 30677, 30687, 30847, 30897, 31272, 31317, 31961,
		// Human and elf fighters.
		30109, 30187, 30689, 30849, 30900, 31965, 32094,
		// Human and elf nuker mystics.
		30115, 30174, 30176, 30694, 30854, 31996,
		// Human and elf buffer mystics.
		30120, 30191, 30857, 30905, 31276, 31321, 31279, 31755, 31968, 32095, 31336,
	}
}

// New returns the feature.
func New() script.Script {
	return script.Script{
		Dir:   "feature",
		Bind:  script.Bindings{script.EventTalked: Masters()},
		Hooks: script.Hooks{OnEvent: onAdvEvent, OnTalk: onTalk},
	}
}

// transfer is one second occupation a change link names. The page suffixes
// are the master's answer: below level 40 without and with the marks, at
// level 40 or above without them, and the transfer made.
type transfer struct {
	newClass, reqClass, reqRace       int32
	lowNoItem, lowItem, noItem, moved int32
	reqItems                          [3]int32
	// denied is the page suffix for a player of another occupation or race.
	denied int32
}

// transferOf returns the occupation the change link event names, and false
// for an event that names none.
func transferOf(event string) (transfer, bool) {
	switch event {
	// Dark elf.
	case "SK":
		return transfer{33, 32, 2, 26, 27, 28, 29, [3]int32{markOfDuty, markOfFate, markOfWitchcraft}, 56}, true
	case "BD":
		return transfer{34, 32, 2, 30, 31, 32, 33, [3]int32{markOfChallenger, markOfFate, markOfDuelist}, 56}, true
	case "SE":
		return transfer{43, 42, 2, 34, 35, 36, 37, [3]int32{markOfPilgrim, markOfFate, markOfReformer}, 56}, true
	case "AW":
		return transfer{36, 35, 2, 38, 39, 40, 41, [3]int32{markOfSeeker, markOfFate, markOfSearcher}, 56}, true
	case "PR":
		return transfer{37, 35, 2, 42, 43, 44, 45, [3]int32{markOfSeeker, markOfFate, markOfSagittarius}, 56}, true
	case "SH":
		return transfer{40, 39, 2, 46, 47, 48, 49, [3]int32{markOfScholar, markOfFate, markOfMagus}, 56}, true
	case "PS":
		return transfer{41, 39, 2, 50, 51, 52, 53, [3]int32{markOfScholar, markOfFate, markOfSummoner}, 56}, true

	// Orc.
	case "TY":
		return transfer{48, 47, 3, 16, 17, 18, 19, [3]int32{markOfChallenger, markOfGlory, markOfDuelist}, 34}, true
	case "DE":
		return transfer{46, 45, 3, 20, 21, 22, 23, [3]int32{markOfChallenger, markOfGlory, markOfChampion}, 34}, true
	case "OL":
		return transfer{51, 50, 3, 24, 25, 26, 27, [3]int32{markOfPilgrim, markOfGlory, markOfLord}, 34}, true
	case "WC":
		return transfer{52, 50, 3, 28, 29, 30, 31, [3]int32{markOfPilgrim, markOfGlory, markOfWarspirit}, 34}, true

	// Dwarf. The Bounty Hunter's page below level 40 without the marks is
	// numbered 109.
	case "BH":
		return transfer{55, 54, 4, 109, 10, 11, 12, [3]int32{markOfGuildsman, markOfProsperity, markOfSearcher}, 15}, true
	case "WS":
		return transfer{57, 56, 4, 16, 17, 18, 19, [3]int32{markOfGuildsman, markOfProsperity, markOfMaestro}, 22}, true

	// Human and elf fighters.
	case "TK":
		return transfer{20, 19, 1, 36, 37, 38, 39, [3]int32{markOfDuty, markOfLife, markOfHealer}, 78}, true
	case "SS":
		return transfer{21, 19, 1, 40, 41, 42, 43, [3]int32{markOfChallenger, markOfLife, markOfDuelist}, 78}, true
	case "PL":
		return transfer{5, 4, 0, 44, 45, 46, 47, [3]int32{markOfDuty, markOfTrust, markOfHealer}, 78}, true
	case "DA":
		return transfer{6, 4, 0, 48, 49, 50, 51, [3]int32{markOfDuty, markOfTrust, markOfWitchcraft}, 78}, true
	case "TH":
		return transfer{8, 7, 0, 52, 53, 54, 55, [3]int32{markOfSeeker, markOfTrust, markOfSearcher}, 78}, true
	case "HE":
		return transfer{9, 7, 0, 56, 57, 58, 59, [3]int32{markOfSeeker, markOfTrust, markOfSagittarius}, 78}, true
	case "PW":
		return transfer{23, 22, 1, 60, 61, 62, 63, [3]int32{markOfSeeker, markOfLife, markOfSearcher}, 78}, true
	case "SR":
		return transfer{24, 22, 1, 64, 65, 66, 67, [3]int32{markOfSeeker, markOfLife, markOfSagittarius}, 78}, true
	case "GL":
		return transfer{2, 1, 0, 68, 69, 70, 71, [3]int32{markOfChallenger, markOfTrust, markOfDuelist}, 78}, true
	case "WL":
		return transfer{3, 1, 0, 72, 73, 74, 75, [3]int32{markOfChallenger, markOfTrust, markOfChampion}, 78}, true

	// Human and elf nuker mystics.
	case "EW":
		return transfer{27, 26, 1, 18, 19, 20, 21, [3]int32{markOfScholar, markOfLife, markOfMagus}, 40}, true
	case "ES":
		return transfer{28, 26, 1, 22, 23, 24, 25, [3]int32{markOfScholar, markOfLife, markOfSummoner}, 40}, true
	case "HS":
		return transfer{12, 11, 0, 26, 27, 28, 29, [3]int32{markOfScholar, markOfTrust, markOfMagus}, 40}, true
	case "HN":
		return transfer{13, 11, 0, 30, 31, 32, 33, [3]int32{markOfScholar, markOfTrust, markOfWitchcraft}, 40}, true
	case "HW":
		return transfer{14, 11, 0, 34, 35, 36, 37, [3]int32{markOfScholar, markOfTrust, markOfSummoner}, 40}, true

	// Human and elf buffer mystics.
	case "BI":
		return transfer{16, 15, 0, 16, 17, 18, 19, [3]int32{markOfPilgrim, markOfTrust, markOfHealer}, 26}, true
	case "PH":
		return transfer{17, 15, 0, 20, 21, 22, 23, [3]int32{markOfPilgrim, markOfTrust, markOfReformer}, 26}, true
	case "EE":
		return transfer{30, 29, 1, 12, 13, 14, 15, [3]int32{markOfPilgrim, markOfLife, markOfHealer}, 26}, true
	}
	return transfer{}, false
}

// numbered returns the suffix of the page numbered n: "-<n>".
func numbered(n int32) string { return "-" + strconv.Itoa(int(n)) }

// onAdvEvent answers a change link with the master's page for the player,
// making the transfer when the player qualifies: the three marks taken, the
// coupons given, the fanfare played, then the new class played and made
// the base class, its henna list refreshed and the player shown anew. Any
// other event is the page it names.
func onAdvEvent(s *script.Script, e script.Event) string {
	htmltext := e.Name
	t, ok := transferOf(e.Name)
	if !ok {
		return htmltext
	}
	p := e.Player
	if p.ClassID() != t.reqClass || int32(p.Race()) != t.reqRace {
		return classHTML(p) + numbered(t.denied) + ".htm"
	}
	var suffix string
	if p.Level() < 40 {
		if p.HasItems(t.reqItems[:]...) {
			suffix = numbered(t.lowItem)
		} else {
			suffix = numbered(t.lowNoItem)
		}
	} else if p.HasItems(t.reqItems[:]...) {
		suffix = numbered(t.moved)
		s.TakeItems(p, t.reqItems[0], -1)
		s.TakeItems(p, t.reqItems[1], -1)
		s.TakeItems(p, t.reqItems[2], -1)
		s.GiveItems(p, cGradeCoupon, 15)
		s.PlaySound(p, script.SoundFanfare)
		p.SetClassID(t.newClass)
		p.SetBaseClass(t.newClass)
		p.RefreshHennaList()
		p.BroadcastUserInfo()
	} else {
		suffix = numbered(t.noItem)
	}
	return classHTML(p) + suffix + ".htm"
}

// onTalk opens the master's pages for the player's race and occupation; a
// player in a subclass gets the no-quest page.
func onTalk(_ *script.Script, e script.Talk) string {
	htmltext := script.NoQuestMsg()
	p := e.Player
	if p.SubClassActive() {
		return htmltext
	}
	race, classID, tier := p.Race(), p.ClassID(), p.ClassLevel()
	humanOrElf := race == player.RaceHuman || race == player.RaceElf
	switch e.NPC.NpcID() {
	case 31328, 30195, 30699, 30474, 31324, 30862, 30910, 31285, 31331, 31334, 31974, 32096: // Dark elf.
		switch {
		case race != player.RaceDarkElf:
			htmltext = "master_de-56.htm"
		case tier == 1:
			switch classID {
			case palusKnight:
				htmltext = "master_de-01.htm"
			case shillienOracle:
				htmltext = "master_de-08.htm"
			case assassin:
				htmltext = "master_de-12.htm"
			case darkWizard:
				htmltext = "master_de-19.htm"
			}
		case tier == 0:
			htmltext = "master_de-55.htm"
		default:
			htmltext = "master_de-54.htm"
		}

	case 30513, 30681, 30704, 30865, 30913, 31288, 31326, 31977: // Orc.
		switch {
		case race != player.RaceOrc:
			htmltext = "master_orc-34.htm"
		case tier == 1:
			switch classID {
			case monk:
				htmltext = "master_orc-01.htm"
			case orcRaider:
				htmltext = "master_orc-05.htm"
			case orcShaman:
				htmltext = "master_orc-09.htm"
			}
		case tier == 0:
			htmltext = "master_orc-33.htm"
		default:
			htmltext = "master_orc-32.htm"
		}

	case 30511, 30676, 30685, 30845, 30894, 31269, 31314, 31958: // Dwarf, for the Bounty Hunter.
		switch {
		case race != player.RaceDwarf:
			htmltext = "master_dwarf-15.htm"
		case tier == 1:
			switch classID {
			case scavenger:
				htmltext = "master_dwarf-01.htm"
			case artisan:
				htmltext = "master_dwarf-15.htm"
			}
		case tier == 0:
			htmltext = "master_dwarf-13.htm"
		default:
			htmltext = "master_dwarf-14.htm"
		}

	case 30512, 30677, 30687, 30847, 30897, 31272, 31317, 31961: // Dwarf, for the Warsmith.
		switch {
		case race != player.RaceDwarf:
			htmltext = "master_dwarf-22.htm"
		case tier == 1:
			switch classID {
			case scavenger:
				htmltext = "master_dwarf-22.htm"
			case artisan:
				htmltext = "master_dwarf-05.htm"
			}
		case tier == 0:
			htmltext = "master_dwarf-20.htm"
		default:
			htmltext = "master_dwarf-21.htm"
		}

	case 30109, 30187, 30689, 30849, 30900, 31965, 32094: // Human and elf fighters.
		switch {
		case !humanOrElf:
			htmltext = "master_human_elf_fighter-78.htm"
		case tier == 1:
			switch classID {
			case elvenKnight:
				htmltext = "master_human_elf_fighter-01.htm"
			case knight:
				htmltext = "master_human_elf_fighter-08.htm"
			case rogue:
				htmltext = "master_human_elf_fighter-15.htm"
			case elvenScout:
				htmltext = "master_human_elf_fighter-22.htm"
			case warrior:
				htmltext = "master_human_elf_fighter-29.htm"
			default:
				htmltext = "master_human_elf_fighter-78.htm"
			}
		case tier == 0:
			htmltext = "master_human_elf_fighter-76.htm"
		default:
			htmltext = "master_human_elf_fighter-77.htm"
		}

	case 30115, 30174, 30176, 30694, 30854, 31996: // Human and elf nuker mystics.
		switch {
		case !humanOrElf:
			htmltext = "master_human_elf_mystic-40.htm"
		case tier == 1:
			switch classID {
			case elvenWizard:
				htmltext = "master_human_elf_mystic-01.htm"
			case humanWizard:
				htmltext = "master_human_elf_mystic-08.htm"
			default:
				htmltext = "master_human_elf_mystic-40.htm"
			}
		case tier == 0:
			htmltext = "master_human_elf_mystic-38.htm"
		default:
			htmltext = "master_human_elf_mystic-39.htm"
		}

	case 30120, 30191, 30857, 30905, 31276, 31321, 31279, 31755, 31968, 32095, 31336: // Human and elf buffer mystics.
		switch {
		case !humanOrElf:
			htmltext = "master_human_elf_buffer-26.htm"
		case tier == 1:
			switch classID {
			case elvenOracle:
				htmltext = "master_human_elf_buffer-01.htm"
			case cleric:
				htmltext = "master_human_elf_buffer-05.htm"
			default:
				htmltext = "master_human_elf_buffer-26.htm"
			}
		case tier == 0:
			htmltext = "master_human_elf_buffer-24.htm"
		default:
			htmltext = "master_human_elf_buffer-25.htm"
		}
	}
	return htmltext
}

// classHTML returns the page family of the player's race and line: the
// human and elf mystics split between the nukers, whose first occupation
// is a wizard's, and the buffers.
func classHTML(p *script.Player) string {
	switch p.Race() {
	case player.RaceDarkElf:
		return "master_de"
	case player.RaceDwarf:
		return "master_dwarf"
	case player.RaceOrc:
		return "master_orc"
	case player.RaceHuman, player.RaceElf:
		if !p.IsMageClass() {
			return "master_human_elf_fighter"
		}
		// The first occupation of the player's line.
		tested := p.ClassID()
		if p.ClassLevel() != 1 {
			tested = script.ClassParent(tested)
		}
		if tested == humanWizard || tested == elvenWizard {
			return "master_human_elf_mystic"
		}
		return "master_human_elf_buffer"
	}
	return ""
}
