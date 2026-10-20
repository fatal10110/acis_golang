// Package q296 is the quest Tarantula's Spider Silk: Mion, for a holder of a
// ring of raccoon or firefly, pays for the tarantulas' spider silk, which
// Defender Nathan also spins from their spinnerettes.
package q296

import (
	"strings"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/questlog"
	"github.com/fatal10110/acis_golang/internal/gameserver/script"
)

const questName = "Q296_TarantulasSpiderSilk"

// NPCs.
const (
	mion           = 30519
	defenderNathan = 30548
)

// Quest items.
const (
	tarantulaSpiderSilk  = 1493
	tarantulaSpinnerette = 1494
)

// Items.
const (
	ringOfRaccoon = 1508
	ringOfFirefly = 1509
)

// New returns the quest.
func New() script.Script {
	return script.Script{
		Title:   "Tarantula's Spider Silk",
		QuestID: 296,
		Items:   []int32{tarantulaSpiderSilk, tarantulaSpinnerette},
		Bind: script.Bindings{
			script.EventQuestStart: {mion},
			script.EventTalked:     {mion, defenderNathan},
			script.EventMyDying:    {20394, 20403, 20508}, // Crimson Tarantula, Hunter Tarantula, Plunder Tarantula
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
	case strings.EqualFold(e.Name, "30519-03.htm"):
		if p.HasAtLeastOneItem(ringOfRaccoon, ringOfFirefly) {
			st.SetStatus(questlog.StatusStarted)
			st.SetCond(1)
			s.PlaySound(p, script.SoundAccept)
		} else {
			htmltext = "30519-03a.htm"
		}
	case strings.EqualFold(e.Name, "30519-06.htm"):
		s.TakeItems(p, tarantulaSpiderSilk, -1)
		s.TakeItems(p, tarantulaSpinnerette, -1)
		s.PlaySound(p, script.SoundFinish)
		st.Exit(true)
	case strings.EqualFold(e.Name, "30548-02.htm"):
		count := p.ItemCount(tarantulaSpinnerette)
		if count > 0 {
			htmltext = "30548-03.htm"
			s.TakeItems(p, tarantulaSpinnerette, -1)
			s.GiveItems(p, tarantulaSpiderSilk, count*(15+int32(s.Rnd(10))))
		}
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
		if p.Level() < 15 {
			htmltext = "30519-01.htm"
		} else {
			htmltext = "30519-02.htm"
		}
	case questlog.StatusStarted:
		switch e.NPC.NpcID() {
		case mion:
			count := p.ItemCount(tarantulaSpiderSilk)
			if count == 0 {
				htmltext = "30519-04.htm"
			} else {
				htmltext = "30519-05.htm"
				s.TakeItems(p, tarantulaSpiderSilk, -1)
				var bonus int32
				if count >= 10 {
					bonus = 2000
				}
				s.RewardItems(p, 57, bonus+count*30)
			}
		case defenderNathan:
			htmltext = "30548-01.htm"
		}
	}
	return htmltext
}

func onMyDying(s *script.Script, e script.MyDying) {
	p := script.ActingPlayer(e.Killer)
	if s.CheckPlayerState(p, e.NPC, questlog.StatusStarted) == nil {
		return
	}
	chance := s.Rnd(100)
	if chance > 95 {
		s.DropItemsAlways(p, tarantulaSpinnerette, 1, 0)
	} else if chance > 45 {
		s.DropItemsAlways(p, tarantulaSpiderSilk, 1, 0)
	}
}
