// Package q107 is the quest Merciless Punishment: an orc follows three of
// Hatos's orders, each sending it to slay Baranka's messenger for a letter,
// for the Butcher's Sword.
package q107

import (
	"strings"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/player"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/questlog"
	"github.com/fatal10110/acis_golang/internal/gameserver/script"
)

const questName = "Q107_MercilessPunishment"

// NPCs.
const (
	hatos   = 30568
	parugon = 30580
)

// Items.
const (
	hatosOrder1     = 1553
	hatosOrder2     = 1554
	hatosOrder3     = 1555
	letterToHuman   = 1557
	letterToDarkElf = 1556
	letterToElf     = 1558
)

// Rewards.
const (
	lesserHealingPot = 1060
	butcherSword     = 1510
	echoBattle       = 4412
	echoLove         = 4413
	echoSolitude     = 4414
	echoFeast        = 4415
	echoCelebration  = 4416
)

// New returns the quest.
func New() script.Script {
	return script.Script{
		Title:   "Merciless Punishment",
		QuestID: 107,
		Items:   []int32{hatosOrder1, hatosOrder2, hatosOrder3, letterToHuman, letterToDarkElf, letterToElf},
		Bind: script.Bindings{
			script.EventQuestStart: {hatos},
			script.EventTalked:     {hatos, parugon},
			script.EventMyDying:    {27041}, // Baranka's Messenger
		},
		Hooks: script.Hooks{OnEvent: onAdvEvent, OnTalk: onTalk, OnMyDying: onMyDying},
	}
}

func onAdvEvent(s *script.Script, e script.Event) string {
	st := s.QuestState(e.Player, questName)
	htmltext := e.Name
	if st == nil {
		return htmltext
	}
	p := e.Player
	switch {
	case strings.EqualFold(e.Name, "30568-03.htm"):
		st.SetStatus(questlog.StatusStarted)
		st.SetCond(1)
		s.PlaySound(p, script.SoundAccept)
		s.GiveItems(p, hatosOrder1, 1)
	case strings.EqualFold(e.Name, "30568-06.htm"):
		s.PlaySound(p, script.SoundGiveUp)
		st.Exit(true)
	case strings.EqualFold(e.Name, "30568-07.htm"):
		st.SetCond(4)
		s.PlaySound(p, script.SoundMiddle)
		s.TakeItems(p, hatosOrder1, 1)
		s.GiveItems(p, hatosOrder2, 1)
	case strings.EqualFold(e.Name, "30568-09.htm"):
		st.SetCond(6)
		s.PlaySound(p, script.SoundMiddle)
		s.TakeItems(p, hatosOrder2, 1)
		s.GiveItems(p, hatosOrder3, 1)
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
		case p.Race() != player.RaceOrc:
			htmltext = "30568-00.htm"
		case p.Level() < 12:
			htmltext = "30568-01.htm"
		default:
			htmltext = "30568-02.htm"
		}
	case questlog.StatusStarted:
		cond := st.Cond()
		switch e.NPC.NpcID() {
		case hatos:
			switch cond {
			case 1, 2:
				htmltext = "30568-04.htm"
			case 3:
				htmltext = "30568-05.htm"
			case 4, 6:
				htmltext = "30568-09.htm"
			case 5:
				htmltext = "30568-08.htm"
			case 7:
				htmltext = "30568-10.htm"
				s.TakeItems(p, hatosOrder3, -1)
				s.TakeItems(p, letterToDarkElf, -1)
				s.TakeItems(p, letterToHuman, -1)
				s.TakeItems(p, letterToElf, -1)
				s.GiveItems(p, butcherSword, 1)

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
		case parugon:
			htmltext = "30580-01.htm"
			if cond == 1 {
				st.SetCond(2)
				s.PlaySound(p, script.SoundMiddle)
			}
		}
	case questlog.StatusCompleted:
		htmltext = script.AlreadyCompletedMsg()
	}
	return htmltext
}

func onMyDying(s *script.Script, e script.MyDying) {
	p := script.ActingPlayer(e.Killer)
	st := s.CheckPlayerState(p, e.NPC, questlog.StatusStarted)
	if st == nil {
		return
	}
	switch st.Cond() {
	case 2:
		st.SetCond(3)
		s.PlaySound(p, script.SoundMiddle)
		s.GiveItems(p, letterToHuman, 1)
	case 4:
		st.SetCond(5)
		s.PlaySound(p, script.SoundMiddle)
		s.GiveItems(p, letterToDarkElf, 1)
	case 6:
		st.SetCond(7)
		s.PlaySound(p, script.SoundMiddle)
		s.GiveItems(p, letterToElf, 1)
	}
}
