// Package q266 is the quest Pleas of Pixies: Murika trades a hundred
// predator fangs from an elf for one gem, picked by a hundred-sided roll.
package q266

import (
	"strings"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/player"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/questlog"
	"github.com/fatal10110/acis_golang/internal/gameserver/script"
)

const questName = "Q266_PleasOfPixies"

// Item.
const predatorFang = 1334

// Rewards.
const (
	glassShard = 1336
	emerald    = 1337
	blueOnyx   = 1338
	onyx       = 1339
)

// New returns the quest.
func New() script.Script {
	return script.Script{
		Title:   "Pleas of Pixies",
		QuestID: 266,
		Items:   []int32{predatorFang},
		Bind: script.Bindings{
			script.EventQuestStart: {31852}, // Murika
			script.EventTalked:     {31852},
			script.EventMyDying:    {20525, 20530, 20534, 20537},
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
	if strings.EqualFold(e.Name, "31852-03.htm") {
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
	p := e.Player
	switch st.Status() {
	case questlog.StatusCreated:
		switch {
		case p.Race() != player.RaceElf:
			htmltext = "31852-00.htm"
		case p.Level() < 3:
			htmltext = "31852-01.htm"
		default:
			htmltext = "31852-02.htm"
		}
	case questlog.StatusStarted:
		if p.ItemCount(predatorFang) < 100 {
			htmltext = "31852-04.htm"
		} else {
			htmltext = "31852-05.htm"
			s.TakeItems(p, predatorFang, -1)
			n := s.Rnd(100)
			switch {
			case n < 10:
				s.PlaySound(p, script.SoundJackpot)
				s.RewardItems(p, emerald, 1)
			case n < 30:
				s.RewardItems(p, blueOnyx, 1)
			case n < 60:
				s.RewardItems(p, onyx, 1)
			default:
				s.RewardItems(p, glassShard, 1)
			}
			s.PlaySound(p, script.SoundFinish)
			st.Exit(true)
		}
	}
	return htmltext
}

func onMyDying(s *script.Script, e script.MyDying) {
	p := script.ActingPlayer(e.Killer)
	st := s.CheckPlayerCondition(p, e.NPC, 1)
	if st == nil {
		return
	}
	switch e.NPC.NpcID() {
	case 20525:
		if s.DropItemsAlways(p, predatorFang, 2+int32(s.Rnd(2)), 100) {
			st.SetCond(2)
		}
	case 20530:
		if s.DropItems(p, predatorFang, 1, 100, 800000) {
			st.SetCond(2)
		}
	case 20534:
		count := int32(2)
		if s.Rnd(3) == 0 {
			count = 1
		}
		if s.DropItems(p, predatorFang, count, 100, 600000) {
			st.SetCond(2)
		}
	case 20537:
		if s.DropItemsAlways(p, predatorFang, 2, 100) {
			st.SetCond(2)
		}
	}
}
