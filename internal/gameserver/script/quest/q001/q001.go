// Package q001 is the quest Letters of Love: Darin's letter carried to
// Roxxy, her kerchief back to Darin, his receipt to Baulro and Baulro's
// potion back to Darin, for a necklace.
package q001

import (
	"strings"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/questlog"
	"github.com/fatal10110/acis_golang/internal/gameserver/script"
)

const questName = "Q001_LettersOfLove"

// NPCs.
const (
	darin  = 30048
	roxxy  = 30006
	baulro = 30033
)

// Items.
const (
	darinLetter   = 687
	roxxyKerchief = 688
	darinReceipt  = 1079
	baulroPotion  = 1080
)

// Reward.
const necklace = 906

// New returns the quest.
func New() script.Script {
	return script.Script{
		Title:   "Letters of Love",
		QuestID: 1,
		Items:   []int32{darinLetter, roxxyKerchief, darinReceipt, baulroPotion},
		Bind: script.Bindings{
			script.EventQuestStart: {darin},
			script.EventTalked:     {darin, roxxy, baulro},
		},
		Hooks: script.Hooks{OnEvent: onAdvEvent, OnTalk: onTalk},
	}
}

func onAdvEvent(s *script.Script, e script.Event) string {
	htmltext := e.Name
	st := s.QuestState(e.Player, questName)
	if st == nil {
		return htmltext
	}
	if strings.EqualFold(e.Name, "30048-06.htm") {
		st.SetStatus(questlog.StatusStarted)
		st.SetCond(1)
		s.PlaySound(e.Player, script.SoundAccept)
		s.GiveItems(e.Player, darinLetter, 1)
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
		if e.Player.Level() < 2 {
			htmltext = "30048-01.htm"
		} else {
			htmltext = "30048-02.htm"
		}
	case questlog.StatusStarted:
		cond := st.Cond()
		switch e.NPC.NpcID() {
		case darin:
			switch cond {
			case 1:
				htmltext = "30048-07.htm"
			case 2:
				htmltext = "30048-08.htm"
				st.SetCond(3)
				s.PlaySound(e.Player, script.SoundMiddle)
				s.TakeItems(e.Player, roxxyKerchief, 1)
				s.GiveItems(e.Player, darinReceipt, 1)
			case 3:
				htmltext = "30048-09.htm"
			case 4:
				htmltext = "30048-10.htm"
				s.TakeItems(e.Player, baulroPotion, 1)
				s.GiveItems(e.Player, necklace, 1)
				s.PlaySound(e.Player, script.SoundFinish)
				st.Exit(false)
			}
		case roxxy:
			switch {
			case cond == 1:
				htmltext = "30006-01.htm"
				st.SetCond(2)
				s.PlaySound(e.Player, script.SoundMiddle)
				s.TakeItems(e.Player, darinLetter, 1)
				s.GiveItems(e.Player, roxxyKerchief, 1)
			case cond == 2:
				htmltext = "30006-02.htm"
			case cond > 2:
				htmltext = "30006-03.htm"
			}
		case baulro:
			switch cond {
			case 3:
				htmltext = "30033-01.htm"
				st.SetCond(4)
				s.PlaySound(e.Player, script.SoundMiddle)
				s.TakeItems(e.Player, darinReceipt, 1)
				s.GiveItems(e.Player, baulroPotion, 1)
			case 4:
				htmltext = "30033-02.htm"
			}
		}
	case questlog.StatusCompleted:
		htmltext = script.AlreadyCompletedMsg()
	}
	return htmltext
}
