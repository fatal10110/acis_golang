// Package q628 is the quest Hunt of the Golden Ram Mercenary Force: Kahman
// asks a character of level 66 or more for chitins that the monsters of the
// swamp drop, swaps them for the badge of a soldier of the Golden Ram, and the
// soldier trades his coins with Abercrombie for goods and with Selina for
// blessings.
package q628

import (
	"math"
	"strconv"
	"strings"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/questlog"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/script"
)

const questName = "Q628_HuntOfTheGoldenRamMercenaryForce"

// NPCs.
const (
	pierce      = 31553
	kahman      = 31554
	abercrombie = 31555
	selina      = 31556
)

// Items.
const (
	goldenRamBadgeRecruit = 7246
	goldenRamBadgeSoldier = 7247
	splinterStakatoChitin = 7248
	needleStakatoChitin   = 7249
	goldenRamCoin         = 7251
)

// buff is a blessing of Selina: the level she casts it at and the coins it
// costs.
type buff struct {
	level, coins int
}

// buffs are the blessings Selina casts, by skill id.
func buffs() map[int]buff {
	return map[int]buff{
		4404: {2, 2},
		4405: {2, 2},
		4393: {3, 3},
		4400: {2, 3},
		4397: {1, 3},
		4399: {2, 3},
		4401: {1, 6},
		4402: {2, 6},
	}
}

// chances are the drop chances of the monsters of the swamp, by monster.
func chances() map[int32]int32 {
	return map[int32]int32{
		21508: 500000,
		21509: 430000,
		21510: 521000,
		21511: 575000,
		21512: 746000,
		21513: 500000,
		21514: 430000,
		21515: 520000,
		21516: 531000,
		21517: 744000,
	}
}

// New returns the quest.
func New() script.Script {
	return script.Script{
		Title:   "Hunt of the Golden Ram Mercenary Force",
		QuestID: 628,
		Items:   []int32{splinterStakatoChitin, needleStakatoChitin, goldenRamBadgeRecruit, goldenRamBadgeSoldier},
		Bind: script.Bindings{
			script.EventQuestStart: {kahman},
			script.EventFirstTalk:  {pierce, kahman, abercrombie, selina},
			script.EventTalked:     {kahman, abercrombie, selina},
			script.EventMyDying: {
				21508, 21509, 21510, 21511, 21512, 21513, 21514, 21515, 21516, 21517,
			},
		},
		Hooks: script.Hooks{
			OnEvent: onAdvEvent, OnFirstTalk: onFirstTalk, OnTalk: onTalk, OnMyDying: onMyDying,
		},
	}
}

// onAdvEvent starts the quest, trades the chitins for the badge of a recruit,
// gives a blessing for coins or a multisell list at once the badge is of a
// soldier, or ends the quest.
func onAdvEvent(s *script.Script, e script.Event) string {
	htmltext := e.Name
	st := s.QuestState(e.Player, questName)
	if st == nil {
		return ""
	}
	p := e.Player
	switch {
	case script.IsDigit(e.Name):
		if st.Cond() < 3 {
			return ""
		}
		buffID, err := strconv.Atoi(e.Name)
		if err != nil || buffID > math.MaxInt32 {
			return htmltext
		}
		if b, ok := buffs()[buffID]; ok {
			if p.ItemCount(goldenRamCoin) < int32(b.coins) {
				htmltext = "31556-11.htm"
			} else {
				htmltext = "31556-12.htm"
				s.TakeItems(p, goldenRamCoin, int32(b.coins))
				e.NPC.AddCastDesire(p, skill.Ref{ID: skill.ID(buffID), Level: b.level}, 1000000)
			}
		}
	case strings.HasPrefix(e.Name, "gmultisell"):
		if st.Cond() == 3 {
			p.ShowMultisell(e.NPC, strings.TrimSpace(e.Name[10:]))
		}
		return ""
	case strings.EqualFold(e.Name, "31554-02.htm"):
		st.SetStatus(questlog.StatusStarted)
		st.SetCond(1)
		s.PlaySound(p, script.SoundAccept)
	case strings.EqualFold(e.Name, "31554-03a.htm"):
		if p.ItemCount(splinterStakatoChitin) >= 100 && st.Cond() == 1 {
			htmltext = "31554-04.htm"
			st.SetCond(2)
			s.PlaySound(p, script.SoundMiddle)
			s.TakeItems(p, splinterStakatoChitin, -1)
			s.GiveItems(p, goldenRamBadgeRecruit, 1)
		}
	case strings.EqualFold(e.Name, "31554-07.htm"):
		s.PlaySound(p, script.SoundGiveUp)
		st.Exit(true)
	}
	return htmltext
}

// onFirstTalk sends a character past the first step to the page of the
// NPC's own business; the others get the window of the NPC.
func onFirstTalk(s *script.Script, e script.FirstTalk) string {
	if st := s.QuestState(e.Player, questName); st != nil {
		npcID := e.NPC.NpcID()
		cond := st.Cond()
		switch npcID {
		case pierce, kahman:
			if cond > 1 {
				return strconv.Itoa(int(npcID)) + "-09.htm"
			}
		case abercrombie, selina:
			if cond == 2 {
				return strconv.Itoa(int(npcID)) + "-09.htm"
			}
			if cond == 3 {
				return strconv.Itoa(int(npcID)) + "-10.htm"
			}
		}
	}
	e.NPC.ShowChatWindow(e.Player)
	return ""
}

func onTalk(s *script.Script, e script.Talk) string {
	htmltext := script.NoQuestMsg()
	st := s.QuestState(e.Player, questName)
	if st == nil {
		return htmltext
	}
	p := e.Player
	switch st.Status() {
	case questlog.StatusCreated:
		if p.Level() < 66 {
			htmltext = "31554-01a.htm"
		} else {
			htmltext = "31554-01.htm"
		}
	case questlog.StatusStarted:
		switch st.Cond() {
		case 1:
			if p.ItemCount(splinterStakatoChitin) >= 100 {
				htmltext = "31554-03.htm"
			} else {
				htmltext = "31554-03a.htm"
			}
		case 2:
			splinters, needles := p.ItemCount(splinterStakatoChitin), p.ItemCount(needleStakatoChitin)
			switch {
			case splinters >= 100 && needles >= 100:
				htmltext = "31554-05.htm"
				st.SetCond(3)
				s.PlaySound(p, script.SoundFinish)
				s.TakeItems(p, splinterStakatoChitin, -1)
				s.TakeItems(p, needleStakatoChitin, -1)
				s.TakeItems(p, goldenRamBadgeRecruit, 1)
				s.GiveItems(p, goldenRamBadgeSoldier, 1)
			case !p.HasItem(splinterStakatoChitin) && !p.HasItem(needleStakatoChitin):
				htmltext = "31554-04b.htm"
			default:
				htmltext = "31554-04a.htm"
			}
		case 3:
			htmltext = "31554-05a.htm"
		}
	}
	return htmltext
}

// onMyDying drops a chitin to a party member who has the quest: a splinter
// from the first five monsters while the badge is not yet of a soldier, a
// needle from the others once it is of a recruit.
func onMyDying(s *script.Script, e script.MyDying) {
	p := script.ActingPlayer(e.Killer)
	st := s.RandomPartyMemberState(p, e.NPC, questlog.StatusStarted)
	if st == nil {
		return
	}
	npcID := e.NPC.NpcID()
	chance := chances()[npcID]
	cond := st.Cond()
	if npcID <= 21512 {
		if cond == 1 || cond == 2 {
			s.DropItems(st.Player(), splinterStakatoChitin, 1, 100, chance)
		}
	} else if cond == 2 {
		s.DropItems(st.Player(), needleStakatoChitin, 1, 100, chance)
	}
}
