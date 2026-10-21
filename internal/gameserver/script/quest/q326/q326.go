// Package q326 is the quest Vanquish Remnants: Leopold pays for the cross
// badges of the remnant soldiers, with the Black Lion mark for a hundred.
package q326

import (
	"strings"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/questlog"
	"github.com/fatal10110/acis_golang/internal/gameserver/script"
)

const questName = "Q326_VanquishRemnants"

// Items.
const (
	redCrossBadge   = 1359
	blueCrossBadge  = 1360
	blackCrossBadge = 1361
)

// Reward.
const blackLionMark = 1369

// New returns the quest.
func New() script.Script {
	return script.Script{
		Title:   "Vanquish Remnants",
		QuestID: 326,
		Items:   []int32{redCrossBadge, blueCrossBadge, blackCrossBadge},
		Bind: script.Bindings{
			script.EventQuestStart: {30435}, // Leopold
			script.EventTalked:     {30435},
			script.EventMyDying:    {20053, 20437, 20058, 20436, 20061, 20439, 20063, 20066, 20438},
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
	case strings.EqualFold(e.Name, "30435-03.htm"):
		st.SetStatus(questlog.StatusStarted)
		st.SetCond(1)
		s.PlaySound(e.Player, script.SoundAccept)
	case strings.EqualFold(e.Name, "30435-07.htm"):
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
		if p.Level() < 21 {
			htmltext = "30435-01.htm"
		} else {
			htmltext = "30435-02.htm"
		}
	case questlog.StatusStarted:
		redBadges := p.ItemCount(redCrossBadge)
		blueBadges := p.ItemCount(blueCrossBadge)
		blackBadges := p.ItemCount(blackCrossBadge)
		badgesSum := redBadges + blueBadges + blackBadges
		if badgesSum > 0 {
			s.TakeItems(p, redCrossBadge, -1)
			s.TakeItems(p, blueCrossBadge, -1)
			s.TakeItems(p, blackCrossBadge, -1)
			bonus := int32(0)
			if badgesSum >= 10 {
				bonus = 4320
			}
			s.RewardItems(p, 57, redBadges*46+blueBadges*52+blackBadges*58+bonus)
			switch {
			case badgesSum < 100:
				htmltext = "30435-05.htm"
			case !p.HasItem(blackLionMark):
				htmltext = "30435-06.htm"
				s.GiveItems(p, blackLionMark, 1)
				s.PlaySound(p, script.SoundItemGet)
			default:
				htmltext = "30435-09.htm"
			}
		} else {
			htmltext = "30435-04.htm"
		}
	}
	return htmltext
}

func onMyDying(s *script.Script, e script.MyDying) {
	p := script.ActingPlayer(e.Killer)
	if s.CheckPlayerState(p, e.NPC, questlog.StatusStarted) == nil {
		return
	}
	switch e.NPC.NpcID() {
	case 20053, 20437, 20058:
		s.DropItems(p, redCrossBadge, 1, 0, 330000)
	case 20436, 20061, 20439, 20063:
		s.DropItems(p, blueCrossBadge, 1, 0, 160000)
	case 20066, 20438:
		s.DropItems(p, blackCrossBadge, 1, 0, 120000)
	}
}
