// Package q655 is the quest A Grand Plan for Taming Wild Beasts: while the
// Wild Beast Reserve waits for its battle, the leader of a level 4 clan with
// no clan hall gathers ten Crystals of Purity, which his clan's tamed beasts
// bring him, for the trainer license that enters the clan in the contest.
package q655

import (
	"strings"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/questlog"
	"github.com/fatal10110/acis_golang/internal/gameserver/script"
)

const questName = "Q655_AGrandPlanForTamingWildBeasts"

// beastFarm is the siegable clan hall the quest waits on.
const beastFarm = 63

// messenger is the quest giver.
const messenger = 35627

// Items.
const (
	crystalOfPurity = 8084
	trainerLicense  = 8293
)

// requiredClanLevel is the clan level the quest asks of the leader.
const requiredClanLevel = 4

// registerPage is the messenger's registration menu.
const registerPage = "data/html/script/siegablehall/WildBeastReserve/farm_kel_mahum_messenger_1.htm"

// siegeDateLayout is how the pages print the next siege's date.
const siegeDateLayout = "2006-01-02 15:04:05"

// New returns the quest.
func New() script.Script {
	return script.Script{
		Title:   "A Grand Plan for Taming Wild Beasts",
		QuestID: 655,
		Items:   []int32{crystalOfPurity, trainerLicense},
		Bind: script.Bindings{
			script.EventQuestStart: {messenger},
			script.EventTalked:     {messenger},
		},
		Hooks: script.Hooks{OnEvent: onAdvEvent, OnTalk: onTalk},
	}
}

// onAdvEvent starts the quest for a qualifying leader and answers the
// registration question: the registration menu while the hall waits for its
// battle, else the event's own page. Its date placeholder is filled in the
// event name, not the page, so the page shows it unfilled.
func onAdvEvent(s *script.Script, e script.Event) string {
	htmltext := e.Name
	st := s.QuestState(e.Player, questName)
	if st == nil {
		return htmltext
	}
	p := e.Player
	hall := s.SiegableHall(beastFarm)
	switch {
	case strings.EqualFold(e.Name, "35627-06.htm"):
		if p.InClan() && p.ClanLevel() >= requiredClanLevel && p.ClanHallID() == 0 && p.IsClanLeader() && hall.WaitingBattle() {
			st.SetStatus(questlog.StatusStarted)
			st.SetCond(1)
			s.PlaySound(p, script.SoundAccept)
		}
	case strings.EqualFold(e.Name, "35627-11.htm"):
		if hall.WaitingBattle() {
			htmltext, _ = s.Page(registerPage)
		} else {
			htmltext = strings.ReplaceAll(htmltext, "%next_siege%", hall.SiegeDate().Format(siegeDateLayout))
		}
	}
	return htmltext
}

// onTalk answers a clan's player with the next siege's date until the hall
// waits for its battle. A started quest aborts once the hall stops waiting;
// the tenth crystal's second step trades the crystals for the license.
func onTalk(s *script.Script, e script.Talk) string {
	st := s.QuestState(e.Player, questName)
	htmltext := script.NoQuestMsg()
	if st == nil {
		return htmltext
	}
	p := e.Player
	hall := s.SiegableHall(beastFarm)
	switch st.Status() {
	case questlog.StatusCreated:
		if !p.InClan() {
			return htmltext
		}
		switch {
		case !hall.WaitingBattle():
			htmltext = strings.ReplaceAll(s.HTMLText("35627-02.htm"), "%next_siege%", hall.SiegeDate().Format(siegeDateLayout))
		case p.IsClanLeader():
			switch {
			case p.ClanHallID() != 0:
				htmltext = "35627-04.htm"
			case p.ClanLevel() >= requiredClanLevel:
				htmltext = "35627-01.htm"
			default:
				htmltext = "35627-03.htm"
			}
		case p.ClanHallID() == beastFarm && hall.WaitingBattle():
			htmltext, _ = s.Page(registerPage)
		default:
			htmltext = "35627-05.htm"
		}
	case questlog.StatusStarted:
		if !hall.WaitingBattle() {
			htmltext = "35627-07.htm"
			st.Exit(true)
			break
		}
		switch st.Cond() {
		case 1:
			htmltext = "35627-08.htm"
		case 2:
			htmltext = "35627-10.htm"
			st.SetCond(3)
			s.PlaySound(p, script.SoundMiddle)
			s.TakeItems(p, crystalOfPurity, -1)
			s.GiveItems(p, trainerLicense, 1)
		case 3:
			htmltext = "35627-09.htm"
		}
	}
	return htmltext
}
