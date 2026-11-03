// Package q101 is the quest Sword of Solidarity: a human carries Roien's
// letter to Altran, gathers the two halves of a broken blade from the ruins
// and brings the reforged handle back, for the Sword of Solidarity.
package q101

import (
	"strings"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/player"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/questlog"
	"github.com/fatal10110/acis_golang/internal/gameserver/script"
)

const questName = "Q101_SwordOfSolidarity"

// NPCs.
const (
	roien  = 30008
	altran = 30283
)

// Items.
const (
	brokenSwordHandle = 739
	brokenBladeBottom = 740
	brokenBladeTop    = 741
	roienLetter       = 796
	directionsToRuins = 937
	altranNote        = 742
)

// Rewards.
const (
	swordOfSolidarity = 738
	lesserHealingPot  = 1060
	echoBattle        = 4412
	echoLove          = 4413
	echoSolitude      = 4414
	echoFeast         = 4415
	echoCelebration   = 4416
)

// New returns the quest.
func New() script.Script {
	return script.Script{
		Title:   "Sword of Solidarity",
		QuestID: 101,
		Items:   []int32{brokenSwordHandle, brokenBladeBottom, brokenBladeTop, roienLetter, directionsToRuins, altranNote},
		Bind: script.Bindings{
			script.EventQuestStart: {roien},
			script.EventTalked:     {roien, altran},
			script.EventMyDying:    {20361, 20362},
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
	case strings.EqualFold(e.Name, "30008-03.htm"):
		st.SetStatus(questlog.StatusStarted)
		st.SetCond(1)
		s.PlaySound(p, script.SoundAccept)
		s.GiveItems(p, roienLetter, 1)
	case strings.EqualFold(e.Name, "30283-02.htm"):
		st.SetCond(2)
		s.PlaySound(p, script.SoundMiddle)
		s.TakeItems(p, roienLetter, 1)
		s.GiveItems(p, directionsToRuins, 1)
	case strings.EqualFold(e.Name, "30283-06.htm"):
		s.TakeItems(p, brokenSwordHandle, 1)
		s.GiveItems(p, swordOfSolidarity, 1)

		s.RewardNewbieShots(p, 7000, 0)
		s.RewardItems(p, lesserHealingPot, 100)
		s.RewardItems(p, echoBattle, 10)
		s.RewardItems(p, echoLove, 10)
		s.RewardItems(p, echoSolitude, 10)
		s.RewardItems(p, echoFeast, 10)
		s.RewardItems(p, echoCelebration, 10)

		p.SocialAction(3)
		s.PlaySound(p, script.SoundFinish)
		st.Exit(false)
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
		switch {
		case p.Race() != player.RaceHuman:
			htmltext = "30008-01a.htm"
		case p.Level() < 9:
			htmltext = "30008-01.htm"
		default:
			htmltext = "30008-02.htm"
		}
	case questlog.StatusStarted:
		cond := st.Cond()
		switch e.NPC.NpcID() {
		case roien:
			switch cond {
			case 1:
				htmltext = "30008-04.htm"
			case 2:
				htmltext = "30008-03a.htm"
			case 3:
				htmltext = "30008-06.htm"
			case 4:
				htmltext = "30008-05.htm"
				st.SetCond(5)
				s.PlaySound(p, script.SoundMiddle)
				s.TakeItems(p, altranNote, 1)
				s.GiveItems(p, brokenSwordHandle, 1)
			case 5:
				htmltext = "30008-05a.htm"
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
				s.TakeItems(p, directionsToRuins, 1)
				s.TakeItems(p, brokenBladeTop, 1)
				s.TakeItems(p, brokenBladeBottom, 1)
				s.GiveItems(p, altranNote, 1)
			case 4:
				htmltext = "30283-04a.htm"
			case 5:
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
	if !p.HasItem(brokenBladeTop) {
		s.DropItems(p, brokenBladeTop, 1, 1, 200000)
	} else if s.DropItems(p, brokenBladeBottom, 1, 1, 200000) {
		st.SetCond(3)
	}
}
