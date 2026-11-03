// Package q319 is the quest Scent of Death: Minaless pays for five zombie
// skins.
package q319

import (
	"strings"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/questlog"
	"github.com/fatal10110/acis_golang/internal/gameserver/script"
)

const questName = "Q319_ScentOfDeath"

// Item.
const zombieSkin = 1045

// New returns the quest.
func New() script.Script {
	return script.Script{
		Title:   "Scent of Death",
		QuestID: 319,
		Items:   []int32{zombieSkin},
		Bind: script.Bindings{
			script.EventQuestStart: {30138}, // Minaless
			script.EventTalked:     {30138},
			script.EventMyDying:    {20015, 20020},
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
	if strings.EqualFold(e.Name, "30138-04.htm") {
		st.SetStatus(questlog.StatusStarted)
		st.SetCond(1)
		s.PlaySound(e.Player, script.SoundAccept)
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
		if p.Level() < 11 {
			htmltext = "30138-02.htm"
		} else {
			htmltext = "30138-03.htm"
		}
	case questlog.StatusStarted:
		if st.Cond() == 1 {
			htmltext = "30138-05.htm"
		} else {
			htmltext = "30138-06.htm"
			s.TakeItems(p, zombieSkin, -1)
			s.RewardItems(p, 57, 3350)
			s.RewardItems(p, 1060, 1)
			s.PlaySound(p, script.SoundFinish)
			st.Exit(true)
		}
	}
	return htmltext
}

func onMyDying(s *script.Script, e script.MyDying) {
	p := script.ActingPlayer(e.Killer)
	st := s.CheckPlayerCondition(p, e.NPC, 1)
	if st == nil {
		return
	}
	if s.DropItems(p, zombieSkin, 1, 5, 200000) {
		st.SetCond(2)
	}
}
