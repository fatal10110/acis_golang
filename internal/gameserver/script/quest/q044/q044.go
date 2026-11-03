// Package q044 is the quest Help the Son!: Lundy trades a work hammer for
// the task of gathering thirty gemstone fragments from the Maille lizardmen;
// Drikus takes the gemstone, and Lundy pays with a pet ticket.
package q044

import (
	"strings"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/questlog"
	"github.com/fatal10110/acis_golang/internal/gameserver/script"
)

const questName = "Q044_HelpTheSon"

// NPCs.
const (
	lundy  = 30827
	drikus = 30505
)

// Items.
const (
	workHammer       = 168
	gemstoneFragment = 7552
	gemstone         = 7553
	petTicket        = 7585
)

// Monsters.
const (
	maille      = 20919
	mailleScout = 20920
	mailleGuard = 20921
)

// New returns the quest.
func New() script.Script {
	return script.Script{
		Title:   "Help the Son!",
		QuestID: 44,
		Items:   []int32{gemstoneFragment, gemstone},
		Bind: script.Bindings{
			script.EventQuestStart: {lundy},
			script.EventTalked:     {lundy, drikus},
			script.EventMyDying:    {maille, mailleScout, mailleGuard},
		},
		Hooks: script.Hooks{OnEvent: onAdvEvent, OnTalk: onTalk, OnMyDying: onMyDying},
	}
}

func onAdvEvent(s *script.Script, e script.Event) string {
	htmltext := e.Name
	st := s.QuestState(e.Player, questName)
	if st == nil {
		return htmltext
	}
	p := e.Player
	switch {
	case strings.EqualFold(e.Name, "30827-01.htm"):
		st.SetStatus(questlog.StatusStarted)
		st.SetCond(1)
		s.PlaySound(p, script.SoundAccept)
	case strings.EqualFold(e.Name, "30827-03.htm") && p.HasItem(workHammer):
		st.SetCond(2)
		s.PlaySound(p, script.SoundMiddle)
		s.TakeItems(p, workHammer, 1)
	case strings.EqualFold(e.Name, "30827-05.htm"):
		st.SetCond(4)
		s.PlaySound(p, script.SoundMiddle)
		s.TakeItems(p, gemstoneFragment, 30)
		s.GiveItems(p, gemstone, 1)
	case strings.EqualFold(e.Name, "30505-06.htm"):
		st.SetCond(5)
		s.PlaySound(p, script.SoundMiddle)
		s.TakeItems(p, gemstone, 1)
	case strings.EqualFold(e.Name, "30827-07.htm"):
		s.GiveItems(p, petTicket, 1)
		s.PlaySound(p, script.SoundFinish)
		st.Exit(false)
	}
	return htmltext
}

func onTalk(s *script.Script, e script.Talk) string {
	st := s.QuestState(e.Player, questName)
	htmltext := script.NoQuestMsg()
	if st == nil {
		return htmltext
	}
	p := e.Player
	switch st.Status() {
	case questlog.StatusCreated:
		if p.Level() < 24 {
			htmltext = "30827-00a.htm"
		} else {
			htmltext = "30827-00.htm"
		}
	case questlog.StatusStarted:
		cond := st.Cond()
		switch e.NPC.NpcID() {
		case lundy:
			switch cond {
			case 1:
				if !p.HasItem(workHammer) {
					htmltext = "30827-01a.htm"
				} else {
					htmltext = "30827-02.htm"
				}
			case 2:
				htmltext = "30827-03a.htm"
			case 3:
				htmltext = "30827-04.htm"
			case 4:
				htmltext = "30827-05a.htm"
			case 5:
				htmltext = "30827-06.htm"
			}
		case drikus:
			if cond == 4 {
				htmltext = "30505-05.htm"
			} else if cond == 5 {
				htmltext = "30505-06a.htm"
			}
		}
	case questlog.StatusCompleted:
		htmltext = script.AlreadyCompletedMsg()
	}
	return htmltext
}

func onMyDying(s *script.Script, e script.MyDying) {
	p := script.ActingPlayer(e.Killer)
	st := s.CheckPlayerCondition(p, e.NPC, 2)
	if st == nil {
		return
	}
	if s.DropItemsAlways(p, gemstoneFragment, 1, 30) {
		st.SetCond(3)
	}
}
