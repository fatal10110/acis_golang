// Package q329 is the quest Curiosity of a Dwarf: Rolento buys the golem
// heartstones, whole and broken.
package q329

import (
	"strings"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/questlog"
	"github.com/fatal10110/acis_golang/internal/gameserver/script"
)

const questName = "Q329_CuriosityOfADwarf"

// Items.
const (
	golemHeartstone  = 1346
	brokenHeartstone = 1365
)

// New returns the quest.
func New() script.Script {
	return script.Script{
		Title:   "Curiosity of a Dwarf",
		QuestID: 329,
		Bind: script.Bindings{
			script.EventQuestStart: {30437}, // Rolento
			script.EventTalked:     {30437},
			script.EventMyDying:    {20083, 20085}, // Granite golem, Puncher
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
	switch {
	case strings.EqualFold(e.Name, "30437-03.htm"):
		st.SetStatus(questlog.StatusStarted)
		st.SetCond(1)
		s.PlaySound(e.Player, script.SoundAccept)
	case strings.EqualFold(e.Name, "30437-06.htm"):
		s.PlaySound(e.Player, script.SoundFinish)
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
	p := e.Player
	switch st.Status() {
	case questlog.StatusCreated:
		if p.Level() < 33 {
			htmltext = "30437-01.htm"
		} else {
			htmltext = "30437-02.htm"
		}
	case questlog.StatusStarted:
		golem := p.ItemCount(golemHeartstone)
		broken := p.ItemCount(brokenHeartstone)
		if golem+broken == 0 {
			htmltext = "30437-04.htm"
		} else {
			htmltext = "30437-05.htm"
			s.TakeItems(p, golemHeartstone, -1)
			s.TakeItems(p, brokenHeartstone, -1)
			bonus := int32(0)
			if golem+broken > 10 {
				bonus = 1183
			}
			s.RewardItems(p, 57, broken*50+golem*1000+bonus)
		}
	}
	return htmltext
}

func onMyDying(s *script.Script, e script.MyDying) {
	p := script.ActingPlayer(e.Killer)
	if s.CheckPlayerState(p, e.NPC, questlog.StatusStarted) == nil {
		return
	}
	chance := s.Rnd(100)
	brokenChance := 50
	if e.NPC.NpcID() == 20083 {
		brokenChance = 44
	}
	if chance < 2 {
		s.DropItemsAlways(p, golemHeartstone, 1, 0)
	} else if chance < brokenChance {
		s.DropItemsAlways(p, brokenHeartstone, 1, 0)
	}
}
