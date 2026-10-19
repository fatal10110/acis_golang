// Package q273 is the quest Invaders of the Holy Land: Varkees pays an orc
// for the black and red soulstones of the Rakeclaw imps, with beginner
// soulshots on the first payment.
package q273

import (
	"strings"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/player"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/questlog"
	"github.com/fatal10110/acis_golang/internal/gameserver/script"
)

const questName = "Q273_InvadersOfTheHolyLand"

// Items.
const (
	blackSoulstone = 1475
	redSoulstone   = 1476
)

// New returns the quest.
func New() script.Script {
	return script.Script{
		Title:   "Invaders of the Holy Land",
		QuestID: 273,
		Items:   []int32{blackSoulstone, redSoulstone},
		Bind: script.Bindings{
			script.EventQuestStart: {30566}, // Varkees
			script.EventTalked:     {30566},
			script.EventMyDying:    {20311, 20312, 20313},
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
	case strings.EqualFold(e.Name, "30566-03.htm"):
		st.SetStatus(questlog.StatusStarted)
		st.SetCond(1)
		s.PlaySound(e.Player, script.SoundAccept)
	case strings.EqualFold(e.Name, "30566-07.htm"):
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
		switch {
		case p.Race() != player.RaceOrc:
			htmltext = "30566-00.htm"
		case p.Level() < 6:
			htmltext = "30566-01.htm"
		default:
			htmltext = "30566-02.htm"
		}
	case questlog.StatusStarted:
		red := p.ItemCount(redSoulstone)
		black := p.ItemCount(blackSoulstone)
		if red+black == 0 {
			htmltext = "30566-04.htm"
		} else {
			if red == 0 {
				htmltext = "30566-05.htm"
			} else {
				htmltext = "30566-06.htm"
			}
			s.TakeItems(p, blackSoulstone, -1)
			s.TakeItems(p, redSoulstone, -1)
			reward := black*3 + red*10
			if black >= 10 {
				if red >= 1 {
					reward += 1800
				} else {
					reward += 1500
				}
			}
			s.RewardItems(p, 57, reward)
			s.RewardNewbieShots(p, 6000, 0)
		}
	}
	return htmltext
}

func onMyDying(s *script.Script, e script.MyDying) {
	p := script.ActingPlayer(e.Killer)
	if s.CheckPlayerState(p, e.NPC, questlog.StatusStarted) == nil {
		return
	}
	probability := 77
	switch e.NPC.NpcID() {
	case 20311:
		probability = 90
	case 20312:
		probability = 87
	}
	if s.Rnd(100) <= probability {
		s.DropItemsAlways(p, blackSoulstone, 1, 0)
	} else {
		s.DropItemsAlways(p, redSoulstone, 1, 0)
	}
}
