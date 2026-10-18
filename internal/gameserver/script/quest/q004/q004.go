// Package q004 is the quest Long live the Pa'agrio Lord!: an orc collects a
// gift from each of six orc chiefs for Nakusin, for a club.
package q004

import (
	"strconv"
	"strings"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/player"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/questlog"
	"github.com/fatal10110/acis_golang/internal/gameserver/script"
)

const questName = "Q004_LongliveThePaagrioLord"

// nakusin is the quest giver.
const nakusin = 30578

// The chiefs.
const (
	hestui  = 30585
	varkees = 30566
	grookin = 30562
	uska    = 30560
	kunai   = 30559
	urutu   = 30587
)

// The gifts, one per chief.
const (
	varkeesGift = 1541
	hestuiGift  = 1542
	grookinGift = 1543
	uskaGift    = 1544
	kunaiGift   = 1545
	urutuGift   = 1546
)

// New returns the quest.
func New() script.Script {
	return script.Script{
		Title:   "Long live the Pa'agrio Lord!",
		QuestID: 4,
		Items:   []int32{1541, 1542, 1543, 1544, 1545, 1546},
		Bind: script.Bindings{
			script.EventQuestStart: {nakusin},
			script.EventTalked:     {nakusin, hestui, varkees, grookin, uska, kunai, urutu},
		},
		Hooks: script.Hooks{OnEvent: onAdvEvent, OnTalk: onTalk},
	}
}

// gifts are the six gifts in the order the quest walks them: the hash
// order of its chief-to-gift table, buckets of the chief id modulo 8.
func gifts() []int32 {
	return []int32{uskaGift, hestuiGift, grookinGift, urutuGift, varkeesGift, kunaiGift}
}

// giftOf is the gift of chief npcID. Any other NPC has none, and the
// lookup aborts the hook.
func giftOf(npcID int32) int32 {
	switch npcID {
	case hestui:
		return hestuiGift
	case varkees:
		return varkeesGift
	case grookin:
		return grookinGift
	case uska:
		return uskaGift
	case kunai:
		return kunaiGift
	case urutu:
		return urutuGift
	}
	panic("q004: no gift of npc " + strconv.Itoa(int(npcID)))
}

func onAdvEvent(s *script.Script, e script.Event) string {
	htmltext := e.Name
	st := s.QuestState(e.Player, questName)
	if st == nil {
		return htmltext
	}
	if strings.EqualFold(e.Name, "30578-03.htm") {
		st.SetStatus(questlog.StatusStarted)
		st.SetCond(1)
		s.PlaySound(e.Player, script.SoundAccept)
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
		switch {
		case e.Player.Race() != player.RaceOrc:
			htmltext = "30578-00.htm"
		case e.Player.Level() < 2:
			htmltext = "30578-01.htm"
		default:
			htmltext = "30578-02.htm"
		}
	case questlog.StatusStarted:
		cond := st.Cond()
		npcID := e.NPC.NpcID()
		if npcID == nakusin {
			switch cond {
			case 1:
				htmltext = "30578-04.htm"
			case 2:
				htmltext = "30578-06.htm"
				s.GiveItems(e.Player, 4, 1)
				for _, item := range gifts() {
					s.TakeItems(e.Player, item, -1)
				}
				s.PlaySound(e.Player, script.SoundFinish)
				st.Exit(false)
			}
			break
		}
		i := giftOf(npcID)
		if e.Player.HasItem(i) {
			htmltext = strconv.Itoa(int(npcID)) + "-02.htm"
			break
		}
		s.GiveItems(e.Player, i, 1)
		htmltext = strconv.Itoa(int(npcID)) + "-01.htm"
		var count int32
		for _, item := range gifts() {
			count += e.Player.ItemCount(item)
		}
		if count == 6 {
			st.SetCond(2)
			s.PlaySound(e.Player, script.SoundMiddle)
		} else {
			s.PlaySound(e.Player, script.SoundItemGet)
		}
	case questlog.StatusCompleted:
		htmltext = script.AlreadyCompletedMsg()
	}
	return htmltext
}
