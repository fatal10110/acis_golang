// Package q034 is the quest In Search of Cloth: for a player whose formal
// wear needs cloth, Radia sends the player to Varan and Ralford, who spins
// trisalim spinnerets into spider silk, and weaves it with suede and thread
// into a mysterious cloth.
package q034

import (
	"strings"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/questlog"
	"github.com/fatal10110/acis_golang/internal/gameserver/script"
)

const questName = "Q034_InSearchOfCloth"

// NPCs.
const (
	radia   = 30088
	ralford = 30165
	varan   = 30294
)

// Monsters.
const (
	trisalimSpider    = 20560
	trisalimTarantula = 20561
)

// Items.
const (
	spinneret  = 7528
	suede      = 1866
	thread     = 1868
	spiderSilk = 7161
)

// Reward.
const mysteriousCloth = 7076

// New returns the quest.
func New() script.Script {
	return script.Script{
		Title:   "In Search of Cloth",
		QuestID: 34,
		Items:   []int32{spinneret, spiderSilk},
		Bind: script.Bindings{
			script.EventQuestStart: {radia},
			script.EventTalked:     {radia, ralford, varan},
			script.EventMyDying:    {trisalimSpider, trisalimTarantula},
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
	case strings.EqualFold(e.Name, "30088-1.htm"):
		st.SetStatus(questlog.StatusStarted)
		st.SetCond(1)
		s.PlaySound(p, script.SoundAccept)
	case strings.EqualFold(e.Name, "30294-1.htm"):
		st.SetCond(2)
		s.PlaySound(p, script.SoundMiddle)
	case strings.EqualFold(e.Name, "30088-3.htm"):
		st.SetCond(3)
		s.PlaySound(p, script.SoundMiddle)
	case strings.EqualFold(e.Name, "30165-1.htm"):
		st.SetCond(4)
		s.PlaySound(p, script.SoundMiddle)
	case strings.EqualFold(e.Name, "30165-3.htm"):
		st.SetCond(6)
		s.PlaySound(p, script.SoundMiddle)
		s.TakeItems(p, spinneret, 10)
		s.GiveItems(p, spiderSilk, 1)
	case strings.EqualFold(e.Name, "30088-5.htm"):
		if hasMaterials(p) {
			s.TakeItems(p, spiderSilk, 1)
			s.TakeItems(p, suede, 3000)
			s.TakeItems(p, thread, 5000)
			s.GiveItems(p, mysteriousCloth, 1)
			s.PlaySound(p, script.SoundFinish)
			st.Exit(false)
		} else {
			htmltext = "30088-4a.htm"
		}
	}
	return htmltext
}

// hasMaterials reports whether the player holds the suede, thread and spider
// silk Radia weaves.
func hasMaterials(p *script.Player) bool {
	return p.ItemCount(suede) >= 3000 && p.ItemCount(thread) >= 5000 && p.HasItem(spiderSilk)
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
				htmltext = "30088-0.htm"
			} else {
				htmltext = "30088-0a.htm"
			}
		} else {
			htmltext = "30088-0b.htm"
		}
	case questlog.StatusStarted:
		cond := st.Cond()
		switch e.NPC.NpcID() {
		case radia:
			switch cond {
			case 1:
				htmltext = "30088-1a.htm"
			case 2:
				htmltext = "30088-2.htm"
			case 3:
				htmltext = "30088-3a.htm"
			case 6:
				if hasMaterials(p) {
					htmltext = "30088-4.htm"
				} else {
					htmltext = "30088-4a.htm"
				}
			}
		case varan:
			if cond == 1 {
				htmltext = "30294-0.htm"
			} else if cond > 1 {
				htmltext = "30294-1a.htm"
			}
		case ralford:
			switch {
			case cond == 3:
				htmltext = "30165-0.htm"
			case cond == 4 && p.ItemCount(spinneret) < 10:
				htmltext = "30165-1a.htm"
			case cond == 5:
				htmltext = "30165-2.htm"
			case cond > 5:
				htmltext = "30165-3a.htm"
			}
		}
	case questlog.StatusCompleted:
		htmltext = script.AlreadyCompletedMsg()
	}
	return htmltext
}

func onMyDying(s *script.Script, e script.MyDying) {
	p := script.ActingPlayer(e.Killer)
	st := s.CheckPlayerCondition(p, e.NPC, 4)
	if st == nil {
		return
	}
	if s.DropItems(p, spinneret, 1, 10, 500000) {
		st.SetCond(5)
	}
}
