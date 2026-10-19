// Package q153 is the quest Deliver Goods: Arnold's three parcels go to
// Jackson, Silvia and Rant, and their three receipts back to Arnold for two
// rings of knowledge.
package q153

import (
	"strings"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/questlog"
	"github.com/fatal10110/acis_golang/internal/gameserver/script"
)

const questName = "Q153_DeliverGoods"

// NPCs.
const (
	jackson = 30002
	silvia  = 30003
	arnold  = 30041
	rant    = 30054
)

// Items.
const (
	deliveryList   = 1012
	heavyWoodBox   = 1013
	clothBundle    = 1014
	clayPot        = 1015
	jacksonReceipt = 1016
	silviaReceipt  = 1017
	rantReceipt    = 1018
)

// Rewards.
const (
	soulshotNoGrade = 1835
	ringOfKnowledge = 875
)

// New returns the quest.
func New() script.Script {
	return script.Script{
		Title:   "Deliver Goods",
		QuestID: 153,
		Items:   []int32{deliveryList, heavyWoodBox, clothBundle, clayPot, jacksonReceipt, silviaReceipt, rantReceipt},
		Bind: script.Bindings{
			script.EventQuestStart: {arnold},
			script.EventTalked:     {jackson, silvia, arnold, rant},
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
	p := e.Player
	if strings.EqualFold(e.Name, "30041-02.htm") {
		st.SetStatus(questlog.StatusStarted)
		st.SetCond(1)
		s.PlaySound(p, script.SoundAccept)
		s.GiveItems(p, deliveryList, 1)
		s.GiveItems(p, clayPot, 1)
		s.GiveItems(p, clothBundle, 1)
		s.GiveItems(p, heavyWoodBox, 1)
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
			htmltext = "30041-00.htm"
		} else {
			htmltext = "30041-01.htm"
		}
	case questlog.StatusStarted:
		switch e.NPC.NpcID() {
		case arnold:
			switch st.Cond() {
			case 1:
				htmltext = "30041-03.htm"
			case 2:
				htmltext = "30041-04.htm"
				s.TakeItems(p, deliveryList, 1)
				s.TakeItems(p, jacksonReceipt, 1)
				s.TakeItems(p, silviaReceipt, 1)
				s.TakeItems(p, rantReceipt, 1)
				s.GiveItems(p, ringOfKnowledge, 1)
				s.GiveItems(p, ringOfKnowledge, 1)
				s.RewardExpAndSp(p, 600, 0)
				s.PlaySound(p, script.SoundFinish)
				st.Exit(false)
			}
		case jackson:
			if p.HasItem(heavyWoodBox) {
				htmltext = "30002-01.htm"
				s.TakeItems(p, heavyWoodBox, 1)
				s.GiveItems(p, jacksonReceipt, 1)

				if p.HasItems(silviaReceipt, rantReceipt) {
					st.SetCond(2)
					s.PlaySound(p, script.SoundMiddle)
				} else {
					s.PlaySound(p, script.SoundItemGet)
				}
			} else {
				htmltext = "30002-02.htm"
			}
		case silvia:
			if p.HasItem(clothBundle) {
				htmltext = "30003-01.htm"
				s.TakeItems(p, clothBundle, 1)
				s.GiveItems(p, silviaReceipt, 1)
				s.GiveItems(p, soulshotNoGrade, 3)

				if p.HasItems(jacksonReceipt, rantReceipt) {
					st.SetCond(2)
					s.PlaySound(p, script.SoundMiddle)
				} else {
					s.PlaySound(p, script.SoundItemGet)
				}
			} else {
				htmltext = "30003-02.htm"
			}
		case rant:
			if p.HasItem(clayPot) {
				htmltext = "30054-01.htm"
				s.TakeItems(p, clayPot, 1)
				s.GiveItems(p, rantReceipt, 1)

				if p.HasItems(jacksonReceipt, silviaReceipt) {
					st.SetCond(2)
					s.PlaySound(p, script.SoundMiddle)
				} else {
					s.PlaySound(p, script.SoundItemGet)
				}
			} else {
				htmltext = "30054-02.htm"
			}
		}
	case questlog.StatusCompleted:
		htmltext = script.AlreadyCompletedMsg()
	}
	return htmltext
}
