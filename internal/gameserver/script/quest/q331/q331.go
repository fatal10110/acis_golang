// Package q331 is the quest Arrow of Vengeance: Belton buys harpy feathers,
// medusa venom and wyrm teeth.
package q331

import (
	"strings"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/questlog"
	"github.com/fatal10110/acis_golang/internal/gameserver/script"
)

const questName = "Q331_ArrowOfVengeance"

// Items.
const (
	harpyFeather = 1452
	medusaVenom  = 1453
	wyrmTooth    = 1454
)

// New returns the quest.
func New() script.Script {
	return script.Script{
		Title:   "Arrow of Vengeance",
		QuestID: 331,
		Items:   []int32{harpyFeather, medusaVenom, wyrmTooth},
		Bind: script.Bindings{
			script.EventQuestStart: {30125}, // Belton
			script.EventTalked:     {30125},
			script.EventMyDying:    {20145, 20158, 20176},
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
	case strings.EqualFold(e.Name, "30125-03.htm"):
		st.SetStatus(questlog.StatusStarted)
		st.SetCond(1)
		s.PlaySound(e.Player, script.SoundAccept)
	case strings.EqualFold(e.Name, "30125-06.htm"):
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
		if p.Level() < 32 {
			htmltext = "30125-01.htm"
		} else {
			htmltext = "30125-02.htm"
		}
	case questlog.StatusStarted:
		feathers := p.ItemCount(harpyFeather)
		venom := p.ItemCount(medusaVenom)
		teeth := p.ItemCount(wyrmTooth)
		if feathers+venom+teeth > 0 {
			htmltext = "30125-05.htm"
			s.TakeItems(p, harpyFeather, -1)
			s.TakeItems(p, medusaVenom, -1)
			s.TakeItems(p, wyrmTooth, -1)
			reward := feathers*78 + venom*88 + teeth*92
			if feathers+venom+teeth > 10 {
				reward += 3100
			}
			s.RewardItems(p, 57, reward)
		} else {
			htmltext = "30125-04.htm"
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
	case 20145:
		s.DropItems(p, harpyFeather, 1, 0, 500000)
	case 20158:
		s.DropItems(p, medusaVenom, 1, 0, 500000)
	case 20176:
		s.DropItems(p, wyrmTooth, 1, 0, 500000)
	}
}
