// Package q654 is the quest Journey to a Settlement: the Nameless Spirit, for
// a character of level 74 or more who has done Last Imperial Prince, asks for
// an antelope skin and pays a scroll of force field removal with it.
package q654

import (
	"strings"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/questlog"
	"github.com/fatal10110/acis_golang/internal/gameserver/script"
)

const questName = "Q654_JourneyToASettlement"

// completedQuest is the quest the spirit wants done before it talks.
const completedQuest = "Q119_LastImperialPrince"

// NPC.
const namelessSpirit = 31453

// Items.
const (
	antelopeSkin            = 8072
	forceFieldRemovalScroll = 8073
)

// New returns the quest.
func New() script.Script {
	return script.Script{
		Title:   "Journey to a Settlement",
		QuestID: 654,
		Items:   []int32{antelopeSkin},
		Bind: script.Bindings{
			script.EventQuestStart: {namelessSpirit},
			script.EventTalked:     {namelessSpirit},
			script.EventMyDying:    {21294, 21295},
		},
		Hooks: script.Hooks{OnEvent: onAdvEvent, OnTalk: onTalk, OnMyDying: onMyDying},
	}
}

// onAdvEvent starts the quest, sends the character for the skin, or pays the
// scroll for it and ends the quest.
func onAdvEvent(s *script.Script, e script.Event) string {
	htmltext := e.Name
	st := s.QuestState(e.Player, questName)
	if st == nil {
		return htmltext
	}
	switch {
	case strings.EqualFold(e.Name, "31453-02.htm"):
		st.SetStatus(questlog.StatusStarted)
		st.SetCond(1)
		s.PlaySound(e.Player, script.SoundAccept)
	case strings.EqualFold(e.Name, "31453-03.htm"):
		st.SetCond(2)
		s.PlaySound(e.Player, script.SoundMiddle)
	case strings.EqualFold(e.Name, "31453-06.htm"):
		s.TakeItems(e.Player, antelopeSkin, -1)
		s.GiveItems(e.Player, forceFieldRemovalScroll, 1)
		s.PlaySound(e.Player, script.SoundFinish)
		st.Exit(true)
	}
	return htmltext
}

func onTalk(s *script.Script, e script.Talk) string {
	htmltext := script.NoQuestMsg()
	st := s.QuestState(e.Player, questName)
	if st == nil {
		return htmltext
	}
	switch st.Status() {
	case questlog.StatusCreated:
		prevSt := s.QuestState(e.Player, completedQuest)
		if prevSt == nil || !prevSt.IsCompleted() || e.Player.Level() < 74 {
			htmltext = "31453-00.htm"
		} else {
			htmltext = "31453-01.htm"
		}
	case questlog.StatusStarted:
		switch st.Cond() {
		case 1:
			htmltext = "31453-02.htm"
		case 2:
			htmltext = "31453-04.htm"
		case 3:
			htmltext = "31453-05.htm"
		}
	}
	return htmltext
}

// onMyDying drops an antelope skin to the killer at cond 2, and moves the
// killer to cond 3 on the first.
func onMyDying(s *script.Script, e script.MyDying) {
	p := script.ActingPlayer(e.Killer)
	st := s.CheckPlayerCondition(p, e.NPC, 2)
	if st == nil {
		return
	}
	if s.DropItems(p, antelopeSkin, 1, 1, 50000) {
		st.SetCond(3)
	}
}
