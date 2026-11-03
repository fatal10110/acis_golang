// Package q247 is the quest Possessor of a Precious Soul - 4: on a
// subclass of level 75 or more, with Caradine's second letter, Caradine
// sends the player to the Lady of the Lake, who makes it a noblesse.
package q247

import (
	"strings"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/questlog"
	"github.com/fatal10110/acis_golang/internal/gameserver/script"
)

const questName = "Q247_PossessorOfAPreciousSoul"

// NPCs.
const (
	caradine      = 31740
	ladyOfTheLake = 31745
)

// Items.
const (
	caradineLetter = 7679
	noblessTiara   = 7694
)

// New returns the quest.
func New() script.Script {
	return script.Script{
		Title:   "Possessor of a Precious Soul - 4",
		QuestID: 247,
		Bind: script.Bindings{
			script.EventQuestStart: {caradine},
			script.EventTalked:     {caradine, ladyOfTheLake},
		},
		Hooks: script.Hooks{OnEvent: onAdvEvent, OnTalk: onTalk},
	}
}

func onAdvEvent(s *script.Script, e script.Event) string {
	htmltext := e.Name
	p := e.Player
	st := s.QuestState(p, questName)
	if st == nil {
		return htmltext
	}
	switch {
	// Caradine.
	case strings.EqualFold(e.Name, "31740-03.htm"):
		st.SetStatus(questlog.StatusStarted)
		st.SetCond(1)
		s.PlaySound(p, script.SoundAccept)
		s.TakeItems(p, caradineLetter, 1)
	case strings.EqualFold(e.Name, "31740-05.htm"):
		st.SetCond(2)
		p.TeleportTo(script.Loc{X: 143209, Y: 43968, Z: -3038}, 0)
	// Lady of the lake.
	case strings.EqualFold(e.Name, "31745-05.htm"):
		p.MakeNoble()
		s.GiveItems(p, noblessTiara, 1)
		s.RewardExpAndSp(p, 93836, 0)
		p.SocialAction(3)
		s.PlaySound(p, script.SoundFinish)
		st.Exit(false)
	}
	return htmltext
}

func onTalk(s *script.Script, e script.Talk) string {
	htmltext := script.NoQuestMsg()
	p := e.Player
	st := s.QuestState(p, questName)
	if st == nil {
		return htmltext
	}
	switch st.Status() {
	case questlog.StatusCreated:
		if p.HasItem(caradineLetter) {
			if !p.SubClassActive() || p.Level() < 75 {
				htmltext = "31740-02.htm"
			} else {
				htmltext = "31740-01.htm"
			}
		}
	case questlog.StatusStarted:
		if !p.SubClassActive() {
			break
		}
		cond := st.Cond()
		switch e.NPC.NpcID() {
		case caradine:
			switch cond {
			case 1:
				htmltext = "31740-04.htm"
			case 2:
				htmltext = "31740-06.htm"
			}
		case ladyOfTheLake:
			if cond == 2 {
				if p.Level() < 75 {
					htmltext = "31745-06.htm"
				} else {
					htmltext = "31745-01.htm"
				}
			}
		}
	case questlog.StatusCompleted:
		htmltext = script.AlreadyCompletedMsg()
	}
	return htmltext
}
