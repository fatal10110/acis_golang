// Package q035 is the quest Find Glittering Jewelry: for a player whose
// formal wear needs a jewel box, Ellie sends the player to Felton, wants
// rough jewels from alligators, then oriharukon, silver nuggets and thons for
// the box.
package q035

import (
	"strings"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/questlog"
	"github.com/fatal10110/acis_golang/internal/gameserver/script"
)

const questName = "Q035_FindGlitteringJewelry"

// NPCs.
const (
	ellie  = 30091
	felton = 30879
)

// Items.
const (
	roughJewel   = 7162
	oriharukon   = 1893
	silverNugget = 1873
	thons        = 4044
)

// Reward.
const jewelBox = 7077

// alligator drops the rough jewels.
const alligator = 20135

// New returns the quest.
func New() script.Script {
	return script.Script{
		Title:   "Find Glittering Jewelry",
		QuestID: 35,
		Items:   []int32{roughJewel},
		Bind: script.Bindings{
			script.EventQuestStart: {ellie},
			script.EventTalked:     {ellie, felton},
			script.EventMyDying:    {alligator},
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
	case strings.EqualFold(e.Name, "30091-1.htm"):
		st.SetStatus(questlog.StatusStarted)
		st.SetCond(1)
		s.PlaySound(p, script.SoundAccept)
	case strings.EqualFold(e.Name, "30879-1.htm"):
		st.SetCond(2)
		s.PlaySound(p, script.SoundMiddle)
	case strings.EqualFold(e.Name, "30091-3.htm"):
		st.SetCond(4)
		s.PlaySound(p, script.SoundMiddle)
		s.TakeItems(p, roughJewel, 10)
	case strings.EqualFold(e.Name, "30091-5.htm"):
		if hasMaterials(p) {
			s.TakeItems(p, oriharukon, 5)
			s.TakeItems(p, silverNugget, 500)
			s.TakeItems(p, thons, 150)
			s.GiveItems(p, jewelBox, 1)
			s.PlaySound(p, script.SoundFinish)
			st.Exit(false)
		} else {
			htmltext = "30091-4a.htm"
		}
	}
	return htmltext
}

// hasMaterials reports whether the player holds the oriharukon, silver
// nuggets and thons the jewel box takes.
func hasMaterials(p *script.Player) bool {
	return p.ItemCount(oriharukon) >= 5 && p.ItemCount(silverNugget) >= 500 && p.ItemCount(thons) >= 150
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
		if p.Level() >= 60 {
			if fwear := s.QuestState(p, "Q037_MakeFormalWear"); fwear != nil && fwear.Cond() == 6 {
				htmltext = "30091-0.htm"
			} else {
				htmltext = "30091-0a.htm"
			}
		} else {
			htmltext = "30091-0b.htm"
		}
	case questlog.StatusStarted:
		cond := st.Cond()
		switch e.NPC.NpcID() {
		case ellie:
			switch cond {
			case 1, 2:
				htmltext = "30091-1a.htm"
			case 3:
				htmltext = "30091-2.htm"
			case 4:
				if hasMaterials(p) {
					htmltext = "30091-4.htm"
				} else {
					htmltext = "30091-4a.htm"
				}
			}
		case felton:
			if cond == 1 {
				htmltext = "30879-0.htm"
			} else if cond > 1 {
				htmltext = "30879-1a.htm"
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
	if s.DropItems(p, roughJewel, 1, 10, 500000) {
		st.SetCond(3)
	}
}
