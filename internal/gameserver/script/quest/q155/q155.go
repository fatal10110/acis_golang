// Package q155 is the quest Find Sir Windawood: Abellos's official letter
// goes to Sir Windawood, who rewards it with a haste potion.
package q155

import (
	"strings"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/questlog"
	"github.com/fatal10110/acis_golang/internal/gameserver/script"
)

const questName = "Q155_FindSirWindawood"

// Items.
const (
	officialLetter = 1019
	hastePotion    = 734
)

// NPCs.
const (
	abellos   = 30042
	windawood = 30311
)

// New returns the quest.
func New() script.Script {
	return script.Script{
		Title:   "Find Sir Windawood",
		QuestID: 155,
		Items:   []int32{officialLetter},
		Bind: script.Bindings{
			script.EventQuestStart: {abellos},
			script.EventTalked:     {windawood, abellos},
		},
		Hooks: script.Hooks{OnEvent: onAdvEvent, OnTalk: onTalk},
	}
}

func onAdvEvent(s *script.Script, e script.Event) string {
	htmltext := e.Name
	st := s.QuestState(e.Player, questName)
	if st == nil {
		return htmltext
	}
	if strings.EqualFold(e.Name, "30042-02.htm") {
		st.SetStatus(questlog.StatusStarted)
		st.SetCond(1)
		s.PlaySound(e.Player, script.SoundAccept)
		s.GiveItems(e.Player, officialLetter, 1)
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
		if p.Level() < 3 {
			htmltext = "30042-01a.htm"
		} else {
			htmltext = "30042-01.htm"
		}
	case questlog.StatusStarted:
		switch e.NPC.NpcID() {
		case abellos:
			htmltext = "30042-03.htm"
		case windawood:
			if p.HasItem(officialLetter) {
				htmltext = "30311-01.htm"
				s.TakeItems(p, officialLetter, 1)
				s.RewardItems(p, hastePotion, 1)
				s.PlaySound(p, script.SoundFinish)
				st.Exit(false)
			}
		}
	case questlog.StatusCompleted:
		htmltext = script.AlreadyCompletedMsg()
	}
	return htmltext
}
