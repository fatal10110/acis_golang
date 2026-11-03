// Package q659 is the quest I'd Rather Be Collecting Fairy Breath: Galatea,
// for a character of level 26 or more, pays adena for the fairy breath the
// wind spirits drop.
package q659

import (
	"strings"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/questlog"
	"github.com/fatal10110/acis_golang/internal/gameserver/script"
)

const questName = "Q659_IdRatherBeCollectingFairyBreath"

// NPC.
const galatea = 30634

// Items.
const fairyBreath = 8286

// Monsters.
const (
	sobbingWind  = 21023
	babblingWind = 21024
	gigglingWind = 21025
)

// New returns the quest.
func New() script.Script {
	return script.Script{
		Title:   "I'd Rather Be Collecting Fairy Breath",
		QuestID: 659,
		Items:   []int32{fairyBreath},
		Bind: script.Bindings{
			script.EventQuestStart: {galatea},
			script.EventTalked:     {galatea},
			script.EventMyDying:    {gigglingWind, babblingWind, sobbingWind},
		},
		Hooks: script.Hooks{OnEvent: onAdvEvent, OnTalk: onTalk, OnMyDying: onMyDying},
	}
}

// onAdvEvent starts the quest, pays adena for the fairy breath held, or ends
// the quest.
func onAdvEvent(s *script.Script, e script.Event) string {
	htmltext := e.Name
	st := s.QuestState(e.Player, questName)
	if st == nil {
		return htmltext
	}
	switch {
	case strings.EqualFold(e.Name, "30634-03.htm"):
		st.SetStatus(questlog.StatusStarted)
		st.SetCond(1)
		s.PlaySound(e.Player, script.SoundAccept)
	case strings.EqualFold(e.Name, "30634-06.htm"):
		count := e.Player.ItemCount(fairyBreath)
		if count > 0 {
			s.TakeItems(e.Player, fairyBreath, count)
			if count < 10 {
				s.RewardItems(e.Player, 57, count*50)
			} else {
				s.RewardItems(e.Player, 57, count*50+5365)
			}
		}
	case strings.EqualFold(e.Name, "30634-08.htm"):
		st.Exit(true)
	}
	return htmltext
}

func onTalk(s *script.Script, e script.Talk) string {
	st := s.QuestState(e.Player, questName)
	htmltext := script.NoQuestMsg()
	if st == nil {
		return htmltext
	}
	switch st.Status() {
	case questlog.StatusCreated:
		if e.Player.Level() < 26 {
			htmltext = "30634-01.htm"
		} else {
			htmltext = "30634-02.htm"
		}
	case questlog.StatusStarted:
		if !e.Player.HasItem(fairyBreath) {
			htmltext = "30634-04.htm"
		} else {
			htmltext = "30634-05.htm"
		}
	}
	return htmltext
}

// onMyDying drops fairy breath to the killer, who must have the quest.
func onMyDying(s *script.Script, e script.MyDying) {
	p := script.ActingPlayer(e.Killer)
	st := s.CheckPlayerState(p, e.NPC, questlog.StatusStarted)
	if st == nil {
		return
	}
	s.DropItems(p, fairyBreath, 1, 0, 900000)
}
