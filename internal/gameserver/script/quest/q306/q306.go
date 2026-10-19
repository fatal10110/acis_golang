// Package q306 is the quest Crystals of Fire and Ice: Katerina buys the
// flame and ice shards of the salamanders and undines, with a bonus past ten.
package q306

import (
	"strings"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/questlog"
	"github.com/fatal10110/acis_golang/internal/gameserver/script"
)

const questName = "Q306_CrystalsOfFireAndIce"

// Items.
const (
	flameShard = 1020
	iceShard   = 1021
)

// droplist returns, per monster, its NPC id, the shard it drops and the
// drop chance.
func droplist() [][]int32 {
	return [][]int32{
		{20109, flameShard, 300000},
		{20110, iceShard, 300000},
		{20112, flameShard, 400000},
		{20113, iceShard, 400000},
		{20114, flameShard, 500000},
		{20115, iceShard, 500000},
	}
}

// New returns the quest.
func New() script.Script {
	return script.Script{
		Title:   "Crystals of Fire and Ice",
		QuestID: 306,
		Items:   []int32{flameShard, iceShard},
		Bind: script.Bindings{
			script.EventQuestStart: {30004}, // Katerina
			script.EventTalked:     {30004},
			script.EventMyDying:    {20109, 20110, 20112, 20113, 20114, 20115},
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
	case strings.EqualFold(e.Name, "30004-03.htm"):
		st.SetStatus(questlog.StatusStarted)
		st.SetCond(1)
		s.PlaySound(e.Player, script.SoundAccept)
	case strings.EqualFold(e.Name, "30004-06.htm"):
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
		if p.Level() < 17 {
			htmltext = "30004-01.htm"
		} else {
			htmltext = "30004-02.htm"
		}
	case questlog.StatusStarted:
		totalItems := p.ItemCount(flameShard) + p.ItemCount(iceShard)
		if totalItems == 0 {
			htmltext = "30004-04.htm"
		} else {
			htmltext = "30004-05.htm"
			s.TakeItems(p, flameShard, -1)
			s.TakeItems(p, iceShard, -1)
			bonus := int32(0)
			if totalItems > 10 {
				bonus = 5000
			}
			s.RewardItems(p, 57, 30*totalItems+bonus)
		}
	}
	return htmltext
}

func onMyDying(s *script.Script, e script.MyDying) {
	p := script.ActingPlayer(e.Killer)
	if s.CheckPlayerState(p, e.NPC, questlog.StatusStarted) == nil {
		return
	}
	for _, drop := range droplist() {
		if e.NPC.NpcID() == drop[0] {
			s.DropItems(p, drop[1], 1, 0, drop[2])
			break
		}
	}
}
