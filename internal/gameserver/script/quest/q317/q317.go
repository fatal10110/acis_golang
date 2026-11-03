// Package q317 is the quest Catch the Wind: Rizraell buys the wind shards
// of the lirein, with a bonus for ten or more.
package q317

import (
	"strings"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/questlog"
	"github.com/fatal10110/acis_golang/internal/gameserver/script"
)

const questName = "Q317_CatchTheWind"

// Item.
const windShard = 1078

// New returns the quest.
func New() script.Script {
	return script.Script{
		Title:   "Catch the Wind",
		QuestID: 317,
		Items:   []int32{windShard},
		Bind: script.Bindings{
			script.EventQuestStart: {30361}, // Rizraell
			script.EventTalked:     {30361},
			script.EventMyDying:    {20036, 20044},
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
	case strings.EqualFold(e.Name, "30361-04.htm"):
		st.SetStatus(questlog.StatusStarted)
		st.SetCond(1)
		s.PlaySound(e.Player, script.SoundAccept)
	case strings.EqualFold(e.Name, "30361-08.htm"):
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
		if p.Level() < 18 {
			htmltext = "30361-02.htm"
		} else {
			htmltext = "30361-03.htm"
		}
	case questlog.StatusStarted:
		shards := p.ItemCount(windShard)
		if shards == 0 {
			htmltext = "30361-05.htm"
		} else {
			htmltext = "30361-07.htm"
			s.TakeItems(p, windShard, -1)
			bonus := int32(0)
			if shards >= 10 {
				bonus = 2988
			}
			s.RewardItems(p, 57, 40*shards+bonus)
		}
	}
	return htmltext
}

func onMyDying(s *script.Script, e script.MyDying) {
	p := script.ActingPlayer(e.Killer)
	if s.CheckPlayerState(p, e.NPC, questlog.StatusStarted) == nil {
		return
	}
	s.DropItems(p, windShard, 1, 0, 500000)
}
