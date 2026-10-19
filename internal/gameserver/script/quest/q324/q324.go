// Package q324 is the quest Sweetest Venom: Astaron pays for ten venom sacs
// of the dryads.
package q324

import (
	"strings"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/questlog"
	"github.com/fatal10110/acis_golang/internal/gameserver/script"
)

const questName = "Q324_SweetestVenom"

// Item.
const venomSac = 1077

// chances returns the venom sac drop chance of each monster, by NPC id.
func chances() map[int32]int32 {
	return map[int32]int32{
		20034: 220000,
		20038: 230000,
		20043: 250000,
	}
}

// New returns the quest.
func New() script.Script {
	return script.Script{
		Title:   "Sweetest Venom",
		QuestID: 324,
		Items:   []int32{venomSac},
		Bind: script.Bindings{
			script.EventQuestStart: {30351}, // Astaron
			script.EventTalked:     {30351},
			script.EventMyDying:    {20034, 20038, 20043},
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
	if strings.EqualFold(e.Name, "30351-04.htm") {
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
		if p.Level() < 18 {
			htmltext = "30351-02.htm"
		} else {
			htmltext = "30351-03.htm"
		}
	case questlog.StatusStarted:
		if st.Cond() == 1 {
			htmltext = "30351-05.htm"
		} else {
			htmltext = "30351-06.htm"
			s.TakeItems(p, venomSac, -1)
			s.RewardItems(p, 57, 5810)
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
	if s.DropItems(p, venomSac, 1, 10, chances()[e.NPC.NpcID()]) {
		st.SetCond(2)
	}
}
