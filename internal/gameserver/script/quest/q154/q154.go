// Package q154 is the quest Sacrifice to the Sea: ten fox furs become
// Cristel's yarn, the yarn Rolfe's maiden doll, and the doll Rockswell's
// earring.
package q154

import (
	"strings"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/questlog"
	"github.com/fatal10110/acis_golang/internal/gameserver/script"
)

const questName = "Q154_SacrificeToTheSea"

// NPCs.
const (
	rockswell = 30312
	cristel   = 30051
	rolfe     = 30055
)

// Items.
const (
	foxFur     = 1032
	foxFurYarn = 1033
	maidenDoll = 1034
)

// Reward.
const earing = 113

// New returns the quest.
func New() script.Script {
	return script.Script{
		Title:   "Sacrifice to the Sea",
		QuestID: 154,
		Items:   []int32{foxFur, foxFurYarn, maidenDoll},
		Bind: script.Bindings{
			script.EventQuestStart: {rockswell},
			script.EventTalked:     {rockswell, cristel, rolfe},
			// The keltirs near Talking Island.
			script.EventMyDying: {20481, 20544, 20545},
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
	if strings.EqualFold(e.Name, "30312-04.htm") {
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
		if p.Level() < 2 {
			htmltext = "30312-02.htm"
		} else {
			htmltext = "30312-03.htm"
		}
	case questlog.StatusStarted:
		cond := st.Cond()
		switch e.NPC.NpcID() {
		case rockswell:
			switch cond {
			case 1:
				htmltext = "30312-05.htm"
			case 2:
				htmltext = "30312-08.htm"
			case 3:
				htmltext = "30312-06.htm"
			case 4:
				htmltext = "30312-07.htm"
				s.TakeItems(p, maidenDoll, -1)
				s.GiveItems(p, earing, 1)
				s.RewardExpAndSp(p, 100, 0)
				s.PlaySound(p, script.SoundFinish)
				st.Exit(false)
			}
		case cristel:
			switch cond {
			case 1:
				if p.HasItem(foxFur) {
					htmltext = "30051-01.htm"
				} else {
					htmltext = "30051-01a.htm"
				}
			case 2:
				htmltext = "30051-02.htm"
				st.SetCond(3)
				s.PlaySound(p, script.SoundMiddle)
				s.TakeItems(p, foxFur, -1)
				s.GiveItems(p, foxFurYarn, 1)
			case 3:
				htmltext = "30051-03.htm"
			case 4:
				htmltext = "30051-04.htm"
			}
		case rolfe:
			switch {
			case cond < 3:
				htmltext = "30055-03.htm"
			case cond == 3:
				htmltext = "30055-01.htm"
				st.SetCond(4)
				s.PlaySound(p, script.SoundMiddle)
				s.TakeItems(p, foxFurYarn, 1)
				s.GiveItems(p, maidenDoll, 1)
			case cond == 4:
				htmltext = "30055-02.htm"
			}
		}
	case questlog.StatusCompleted:
		htmltext = script.AlreadyCompletedMsg()
	}
	return htmltext
}

func onMyDying(s *script.Script, e script.MyDying) {
	p := script.ActingPlayer(e.Killer)
	st := s.CheckPlayerCondition(p, e.NPC, 1)
	if st == nil {
		return
	}
	if s.DropItems(p, foxFur, 1, 10, 400000) {
		st.SetCond(2)
	}
}
