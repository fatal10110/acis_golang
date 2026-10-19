// Package q265 is the quest Chains of Slavery: Kristin pays a dark elf for
// the shackles of the imps, with beginner shots on the first payment.
package q265

import (
	"strings"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/player"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/questlog"
	"github.com/fatal10110/acis_golang/internal/gameserver/script"
)

const questName = "Q265_ChainsOfSlavery"

// Item.
const shackle = 1368

// New returns the quest.
func New() script.Script {
	return script.Script{
		Title:   "Chains of Slavery",
		QuestID: 265,
		Items:   []int32{shackle},
		Bind: script.Bindings{
			script.EventQuestStart: {30357}, // Kristin
			script.EventTalked:     {30357},
			script.EventMyDying:    {20004, 20005},
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
	case strings.EqualFold(e.Name, "30357-03.htm"):
		st.SetStatus(questlog.StatusStarted)
		st.SetCond(1)
		s.PlaySound(e.Player, script.SoundAccept)
	case strings.EqualFold(e.Name, "30357-06.htm"):
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
		switch {
		case p.Race() != player.RaceDarkElf:
			htmltext = "30357-00.htm"
		case p.Level() < 6:
			htmltext = "30357-01.htm"
		default:
			htmltext = "30357-02.htm"
		}
	case questlog.StatusStarted:
		shackles := p.ItemCount(shackle)
		if shackles == 0 {
			htmltext = "30357-04.htm"
		} else {
			htmltext = "30357-05.htm"
			s.TakeItems(p, shackle, -1)
			reward := 12 * shackles
			if shackles >= 10 {
				reward += 500
			}
			s.RewardItems(p, 57, reward)
			s.RewardNewbieShots(p, 6000, 3000)
		}
	}
	return htmltext
}

func onMyDying(s *script.Script, e script.MyDying) {
	p := script.ActingPlayer(e.Killer)
	if s.CheckPlayerState(p, e.NPC, questlog.StatusStarted) == nil {
		return
	}
	chance := int32(600000)
	if e.NPC.NpcID() == 20004 {
		chance = 500000
	}
	s.DropItems(p, shackle, 1, 0, chance)
}
