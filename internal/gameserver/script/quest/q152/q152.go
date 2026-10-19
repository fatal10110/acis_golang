// Package q152 is the quest Shards of Golem: Harris's receipt goes to
// Altran, who wants five stone golem shards for the tool box Harris trades
// for a wooden breastplate.
package q152

import (
	"strings"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/questlog"
	"github.com/fatal10110/acis_golang/internal/gameserver/script"
)

const questName = "Q152_ShardsOfGolem"

// Items.
const (
	harrisReceipt1 = 1008
	harrisReceipt2 = 1009
	golemShard     = 1010
	toolBox        = 1011
)

// Reward.
const woodenBreastplate = 23

// NPCs.
const (
	harris = 30035
	altran = 30283
)

// Monster.
const stoneGolem = 20016

// New returns the quest.
func New() script.Script {
	return script.Script{
		Title:   "Shards of Golem",
		QuestID: 152,
		Items:   []int32{harrisReceipt1, harrisReceipt2, golemShard, toolBox},
		Bind: script.Bindings{
			script.EventQuestStart: {harris},
			script.EventTalked:     {harris, altran},
			script.EventMyDying:    {stoneGolem},
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
	p := e.Player
	switch {
	case strings.EqualFold(e.Name, "30035-02.htm"):
		st.SetStatus(questlog.StatusStarted)
		st.SetCond(1)
		s.PlaySound(p, script.SoundAccept)
		s.GiveItems(p, harrisReceipt1, 1)
	case strings.EqualFold(e.Name, "30283-02.htm"):
		st.SetCond(2)
		s.PlaySound(p, script.SoundMiddle)
		s.TakeItems(p, harrisReceipt1, 1)
		s.GiveItems(p, harrisReceipt2, 1)
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
		if p.Level() < 10 {
			htmltext = "30035-01a.htm"
		} else {
			htmltext = "30035-01.htm"
		}
	case questlog.StatusStarted:
		cond := st.Cond()
		switch e.NPC.NpcID() {
		case harris:
			switch {
			case cond < 4:
				htmltext = "30035-03.htm"
			case cond == 4:
				htmltext = "30035-04.htm"
				s.TakeItems(p, harrisReceipt2, 1)
				s.TakeItems(p, toolBox, 1)
				s.GiveItems(p, woodenBreastplate, 1)
				s.RewardExpAndSp(p, 5000, 0)
				s.PlaySound(p, script.SoundFinish)
				st.Exit(false)
			}
		case altran:
			switch cond {
			case 1:
				htmltext = "30283-01.htm"
			case 2:
				htmltext = "30283-03.htm"
			case 3:
				htmltext = "30283-04.htm"
				st.SetCond(4)
				s.PlaySound(p, script.SoundMiddle)
				s.TakeItems(p, golemShard, -1)
				s.GiveItems(p, toolBox, 1)
			case 4:
				htmltext = "30283-05.htm"
			}
		}
	case questlog.StatusCompleted:
		htmltext = script.AlreadyCompletedMsg()
	}
	return htmltext
}

func onMyDying(s *script.Script, e script.MyDying) {
	p := script.ActingPlayer(e.Killer)
	st := s.CheckPlayerCondition(p, e.NPC, 2)
	if st == nil {
		return
	}
	if s.DropItems(p, golemShard, 1, 5, 300000) {
		st.SetCond(3)
	}
}
