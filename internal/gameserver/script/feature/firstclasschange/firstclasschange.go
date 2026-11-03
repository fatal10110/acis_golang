// Package firstclasschange is the grand masters and high priests who move a
// player from a starting class to its first occupation. Talking to one opens
// the master's pages for the player's race and class; a page's change link
// names the occupation, and a player of the right starting class and race
// at level 20 or above who holds the occupation's test token gives it up
// for fifteen D-grade coupons and the new class. Any other link is answered
// as the page it names.
package firstclasschange

import (
	"strconv"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/player"
	"github.com/fatal10110/acis_golang/internal/gameserver/script"
)

// The test tokens, by the race of their occupations.
const (
	// Dark elf.
	gazeOfAbyss     = 1244
	ironHeart       = 1252
	jewelOfDarkness = 1261
	orbOfAbyss      = 1270

	// Orc.
	markOfRaider   = 1592
	khavatariTotem = 1615
	maskOfMedium   = 1631

	// Dwarf.
	artiMarks = 1635
	scavMarks = 1642

	// Elf.
	elvenKnightBrooch    = 1204
	reoriaRecommendation = 1217
	eternityDiamond      = 1230
	leafOfOracle         = 1235

	// Human.
	medallionOfWarrior     = 1145
	swordOfRitual          = 1161
	beziquesRecommendation = 1190
	beadOfSeason           = 1292
	markOfFaith            = 1201
)

// dGradeCoupon is the reward of every transfer.
const dGradeCoupon = 8869

// Masters are the grand masters and high priests who talk about first
// occupations.
func Masters() []int32 {
	return []int32{
		30026, 30031, 30037, 30066, 30070, 30154, 30288, 30289, 30290, 30297,
		30358, 30373, 30462, 30498, 30499, 30500, 30503, 30504, 30505, 30508,
		30520, 30525, 30565, 30594, 30595, 32092, 32093, 32097, 32098,
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

// transfer is one first occupation a change link names. The page suffixes
// are the master's answer: below level 20 without and with the token, at
// level 20 or above without it, and the transfer made.
type transfer struct {
	newClass, reqClass, reqRace       int32
	lowNoItem, lowItem, noItem, moved int32
	reqItem                           int32
	// denied is the page suffix for a player of another class or race.
	denied int32
}

// transferOf returns the occupation the change link event names, and false
// for an event that names none.
func transferOf(event string) (transfer, bool) {
	switch event {
	// Dark elf.
	case "PK":
		return transfer{32, 31, 2, 15, 16, 17, 18, gazeOfAbyss, 33}, true
	case "AS":
		return transfer{35, 31, 2, 19, 20, 21, 22, ironHeart, 33}, true
	case "DW":
		return transfer{39, 38, 2, 23, 24, 25, 26, jewelOfDarkness, 33}, true
	case "SO":
		return transfer{42, 38, 2, 27, 28, 29, 30, orbOfAbyss, 33}, true

	// Orc.
	case "OR":
		return transfer{45, 44, 3, 9, 10, 11, 12, markOfRaider, 23}, true
	case "OM":
		return transfer{47, 44, 3, 13, 14, 15, 16, khavatariTotem, 23}, true
	case "OS":
		return transfer{50, 49, 3, 17, 18, 19, 20, maskOfMedium, 23}, true

	// Dwarf.
	case "SC":
		return transfer{54, 53, 4, 5, 6, 7, 8, scavMarks, 11}, true
	case "AR":
		return transfer{56, 53, 4, 5, 6, 7, 8, artiMarks, 11}, true

	// Elf.
	case "EK":
		return transfer{19, 18, 1, 18, 19, 20, 21, elvenKnightBrooch, 40}, true
	case "ES":
		return transfer{22, 18, 1, 22, 23, 24, 25, reoriaRecommendation, 40}, true
	case "EW":
		return transfer{26, 25, 1, 15, 16, 17, 18, eternityDiamond, 33}, true
	case "EO":
		return transfer{29, 25, 1, 19, 20, 21, 22, leafOfOracle, 33}, true

	// Human.
	case "HW":
		return transfer{1, 0, 0, 26, 27, 28, 29, medallionOfWarrior, 40}, true
	case "HK":
		return transfer{4, 0, 0, 30, 31, 32, 33, swordOfRitual, 40}, true
	case "HR":
		return transfer{7, 0, 0, 34, 35, 36, 37, beziquesRecommendation, 40}, true
	case "HWI":
		return transfer{11, 10, 0, 23, 24, 25, 26, beadOfSeason, 33}, true
	case "HC":
		return transfer{15, 10, 0, 27, 28, 29, 30, markOfFaith, 33}, true
	}
	return transfer{}, false
}

// page returns the master's page npcID with suffix: "<npcID><suffix>.htm".
func page(npcID int32, suffix string) string {
	return strconv.Itoa(int(npcID)) + suffix + ".htm"
}

// numbered returns the suffix of the page numbered n: "-<n>".
func numbered(n int32) string { return "-" + strconv.Itoa(int(n)) }

// onAdvEvent answers a change link with the master's page for the player,
// making the transfer when the player qualifies: the token taken, the
// coupons given, the fanfare played, then the new class played and made
// the base class, its henna list refreshed and the player shown anew.
// Any other event is the page it names.
func onAdvEvent(s *script.Script, e script.Event) string {
	htmltext := e.Name
	t, ok := transferOf(e.Name)
	if !ok {
		return htmltext
	}
	p := e.Player
	npcID := e.NPC.NpcID()
	if p.ClassID() != t.reqClass || int32(p.Race()) != t.reqRace {
		return page(npcID, numbered(t.denied))
	}
	gotItem := p.HasItem(t.reqItem)
	var suffix string
	switch {
	case p.Level() < 20:
		if gotItem {
			suffix = numbered(t.lowItem)
		} else {
			suffix = numbered(t.lowNoItem)
		}
	case gotItem:
		suffix = numbered(t.moved)
		s.TakeItems(p, t.reqItem, 1)
		s.GiveItems(p, dGradeCoupon, 15)
		s.PlaySound(p, script.SoundFanfare)
		p.SetClassID(t.newClass)
		p.SetBaseClass(t.newClass)
		p.RefreshHennaList()
		p.BroadcastUserInfo()
	default:
		suffix = numbered(t.noItem)
	}
	return page(npcID, suffix)
}

// onTalk opens the master's pages for the player's race, class and
// occupation tier; a player in a subclass gets the no-quest page.
func onTalk(_ *script.Script, e script.Talk) string {
	htmltext := script.NoQuestMsg()
	p := e.Player
	if p.SubClassActive() {
		return htmltext
	}
	npcID := e.NPC.NpcID()
	id := strconv.Itoa(int(npcID))
	race, classID, tier := p.Race(), p.ClassID(), p.ClassLevel()
	switch npcID {
	case 30290, 30297, 30462: // Dark elf.
		switch {
		case race != player.RaceDarkElf:
			htmltext = id + "-33.htm"
		case tier == 0:
			if classID == 31 {
				htmltext = id + "-01.htm"
			} else if classID == 38 {
				htmltext = id + "-08.htm"
			}
		case tier == 1:
			htmltext = id + "-32.htm"
		default:
			htmltext = id + "-31.htm"
		}

	case 30358: // Thifiell, dark elf.
		switch {
		case race != player.RaceDarkElf:
			htmltext = id + "-11.htm"
		case tier == 0:
			if classID == 31 {
				htmltext = id + "-01.htm"
			} else if classID == 38 {
				htmltext = id + "-02.htm"
			}
		case tier == 1:
			htmltext = id + "-12.htm"
		default:
			htmltext = id + "-13.htm"
		}

	case 30500, 30505, 30508, 32097: // Orc.
		switch {
		case race != player.RaceOrc:
			htmltext = id + "-23.htm"
		case tier == 0:
			if classID == 44 {
				htmltext = id + "-01.htm"
			} else if classID == 49 {
				htmltext = id + "-06.htm"
			}
		case tier == 1:
			htmltext = id + "-21.htm"
		default:
			htmltext = id + "-22.htm"
		}

	case 30565: // Kakai, orc.
		switch {
		case race != player.RaceOrc:
			htmltext = id + "-11.htm"
		case tier == 0:
			if classID == 44 {
				htmltext = id + "-01.htm"
			} else if classID == 49 {
				htmltext = id + "-06.htm"
			}
		case tier == 1:
			htmltext = id + "-09.htm"
		default:
			htmltext = id + "-10.htm"
		}

	case 30498, 30499, 30503, 30504, 30594, 30595, 32092, 32093: // Dwarf.
		switch {
		case race != player.RaceDwarf:
			htmltext = id + "-11.htm"
		case tier == 0:
			if classID == 53 {
				htmltext = id + "-01.htm"
			}
		case tier == 1:
			htmltext = id + "-09.htm"
		default:
			htmltext = id + "-10.htm"
		}

	case 30520, 30525: // Bronk and Reed, dwarf.
		switch {
		case race != player.RaceDwarf:
			htmltext = id + "-07.htm"
		case tier == 0:
			if classID == 53 {
				htmltext = id + "-01.htm"
			}
		case tier == 1:
			htmltext = id + "-05.htm"
		default:
			htmltext = id + "-06.htm"
		}

	case 30037, 30070, 30289, 32098: // Elf and human mystics.
		switch {
		case race == player.RaceElf && p.IsMageClass():
			switch {
			case tier == 0:
				if classID == 25 {
					htmltext = id + "-01.htm"
				}
			case tier == 1:
				htmltext = id + "-31.htm"
			default:
				htmltext = id + "-32.htm"
			}
		case race == player.RaceHuman && p.IsMageClass():
			switch {
			case tier == 0:
				if classID == 10 {
					htmltext = id + "-08.htm"
				}
			case tier == 1:
				htmltext = id + "-31.htm"
			default:
				htmltext = id + "-32.htm"
			}
		default:
			htmltext = id + "-33.htm"
		}

	case 30154: // Asterios, elf fighters and mystics.
		switch {
		case race != player.RaceElf:
			htmltext = id + "-11.htm"
		case tier == 0:
			if classID == 18 {
				htmltext = id + "-01.htm"
			} else if classID == 25 {
				htmltext = id + "-02.htm"
			}
		case tier == 1:
			htmltext = id + "-12.htm"
		default:
			htmltext = id + "-13.htm"
		}

	case 30031: // Biotin, human mystics.
		switch {
		case race != player.RaceHuman || !p.IsMageClass():
			htmltext = id + "-08.htm"
		case tier == 0:
			if classID == 10 {
				htmltext = id + "-01.htm"
			}
		case tier == 1:
			htmltext = id + "-06.htm"
		default:
			htmltext = id + "-07.htm"
		}

	case 30066, 30288, 30373: // Human and elf fighters.
		switch race {
		case player.RaceHuman:
			switch {
			case tier == 0:
				if classID == 0 {
					htmltext = id + "-08.htm"
				} else {
					htmltext = id + "-40.htm"
				}
			case tier == 1:
				htmltext = id + "-38.htm"
			default:
				htmltext = id + "-39.htm"
			}
		case player.RaceElf:
			switch {
			case tier == 0:
				if classID == 18 {
					htmltext = id + "-01.htm"
				} else {
					htmltext = id + "-40.htm"
				}
			case tier == 1:
				htmltext = id + "-38.htm"
			default:
				htmltext = id + "-39.htm"
			}
		default:
			htmltext = id + "-40.htm"
		}

	case 30026: // Bitz, human fighters.
		switch {
		case race != player.RaceHuman:
			htmltext = id + "-10.htm"
		case tier == 0:
			if classID == 0 {
				htmltext = id + "-01.htm"
			} else {
				htmltext = id + "-10.htm"
			}
		case tier == 1:
			htmltext = id + "-08.htm"
		default:
			htmltext = id + "-09.htm"
		}
	}
	return htmltext
}
