// Package q638 is the quest Seekers of the Holy Grail: Innocentin, for a
// character of level 73 or more, asks for two thousand pagan totems, as often
// as the character brings them, and pays a scroll or adena for each batch.
package q638

import (
	"strings"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/questlog"
	"github.com/fatal10110/acis_golang/internal/gameserver/script"
)

const questName = "Q638_SeekersOfTheHolyGrail"

// NPC.
const innocentin = 31328

// Items.
const paganTotem = 8068

// monsters returns the monsters that drop pagan totems.
func monsters() []int32 {
	var ids []int32
	for id := int32(22138); id < 22175; id++ {
		ids = append(ids, id)
	}
	return ids
}

// New returns the quest.
func New() script.Script {
	return script.Script{
		Title:   "Seekers of the Holy Grail",
		QuestID: 638,
		Items:   []int32{paganTotem},
		Bind: script.Bindings{
			script.EventQuestStart: {innocentin},
			script.EventTalked:     {innocentin},
			script.EventMyDying:    monsters(),
		},
		Hooks: script.Hooks{OnEvent: onAdvEvent, OnTalk: onTalk, OnMyDying: onMyDying},
	}
}

// onAdvEvent starts the quest, or ends it.
func onAdvEvent(s *script.Script, e script.Event) string {
	htmltext := e.Name
	st := s.QuestState(e.Player, questName)
	if st == nil {
		return htmltext
	}
	switch {
	case strings.EqualFold(e.Name, "31328-02.htm"):
		st.SetStatus(questlog.StatusStarted)
		st.SetCond(1)
		s.PlaySound(e.Player, script.SoundAccept)
	case strings.EqualFold(e.Name, "31328-06.htm"):
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
		if e.Player.Level() < 73 {
			htmltext = "31328-00.htm"
		} else {
			htmltext = "31328-01.htm"
		}
	case questlog.StatusStarted:
		if e.Player.ItemCount(paganTotem) >= 2000 {
			htmltext = "31328-03.htm"
			s.PlaySound(e.Player, script.SoundMiddle)
			s.TakeItems(e.Player, paganTotem, 2000)

			switch s.Rnd(3) {
			case 0:
				s.RewardItems(e.Player, 959, 1)
			case 1:
				s.RewardItems(e.Player, 960, 1)
			default:
				s.RewardItems(e.Player, 57, 3576000)
			}
		} else {
			htmltext = "31328-04.htm"
		}
	}
	return htmltext
}

// onMyDying drops a pagan totem to a party member who has the quest.
func onMyDying(s *script.Script, e script.MyDying) {
	p := script.ActingPlayer(e.Killer)
	st := s.RandomPartyMemberState(p, e.NPC, questlog.StatusStarted)
	if st == nil {
		return
	}
	s.DropItemsAlways(st.Player(), paganTotem, 1, 0)
}
