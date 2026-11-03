// Package q366 is the quest Silver Haired Shaman: Dieter buys the hair of the
// silver-haired shamans.
package q366

import (
	"strings"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/questlog"
	"github.com/fatal10110/acis_golang/internal/gameserver/script"
)

const questName = "Q366_SilverHairedShaman"

// dieter is the quest giver.
const dieter = 30111

// hair is the quest item.
const hair = 5874

// New returns the quest.
func New() script.Script {
	return script.Script{
		Title:   "Silver Haired Shaman",
		QuestID: 366,
		Items:   []int32{hair},
		Bind: script.Bindings{
			script.EventQuestStart: {dieter},
			script.EventTalked:     {dieter},
			script.EventMyDying:    {20986, 20987, 20988},
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
	case strings.EqualFold(e.Name, "30111-2.htm"):
		st.SetStatus(questlog.StatusStarted)
		st.SetCond(1)
		s.PlaySound(e.Player, script.SoundAccept)
	case strings.EqualFold(e.Name, "30111-6.htm"):
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
	p := e.Player
	switch st.Status() {
	case questlog.StatusCreated:
		if p.Level() < 48 {
			htmltext = "30111-0.htm"
		} else {
			htmltext = "30111-1.htm"
		}
	case questlog.StatusStarted:
		count := p.ItemCount(hair)
		if count == 0 {
			htmltext = "30111-3.htm"
		} else {
			htmltext = "30111-4.htm"
			s.TakeItems(p, hair, -1)
			s.RewardItems(p, 57, 12070+500*count)
		}
	}
	return htmltext
}

func onMyDying(s *script.Script, e script.MyDying) {
	p := script.ActingPlayer(e.Killer)
	st := s.RandomPartyMemberState(p, e.NPC, questlog.StatusStarted)
	if st == nil {
		return
	}
	s.DropItems(st.Player(), hair, 1, 0, chance(e.NPC.NpcID()))
}

// chance is the per-monster drop chance of the hair.
func chance(npcID int32) int32 {
	switch npcID {
	case 20986:
		return 560000
	case 20987:
		return 660000
	default:
		return 620000
	}
}
