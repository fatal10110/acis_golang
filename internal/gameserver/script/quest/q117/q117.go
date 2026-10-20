// Package q117 is the quest The Ocean of Distant Stars: Abey, the ancient
// ghost, Obi and a box send the character after an engraved hammer and a
// grey star of the bandits, for experience from the ghost.
package q117

import (
	"strings"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/questlog"
	"github.com/fatal10110/acis_golang/internal/gameserver/script"
)

const questName = "Q117_TheOceanOfDistantStars"

// NPCs.
const (
	abey         = 32053
	ghost        = 32054
	ancientGhost = 32055
	obi          = 32052
	box          = 32076
)

// Items.
const (
	greyStar       = 8495
	engravedHammer = 8488
)

// Monsters.
const (
	banditWarrior   = 22023
	banditInspector = 22024
)

// New returns the quest.
func New() script.Script {
	return script.Script{
		Title:   "The Ocean of Distant Stars",
		QuestID: 117,
		Items:   []int32{greyStar, engravedHammer},
		Bind: script.Bindings{
			script.EventQuestStart: {abey},
			script.EventTalked:     {abey, ancientGhost, ghost, obi, box},
			script.EventMyDying:    {banditWarrior, banditInspector},
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
	case strings.EqualFold(e.Name, "32053-02.htm"):
		st.SetStatus(questlog.StatusStarted)
		st.SetCond(1)
		s.PlaySound(p, script.SoundAccept)
	case strings.EqualFold(e.Name, "32055-02.htm"):
		st.SetCond(2)
		s.PlaySound(p, script.SoundMiddle)
	case strings.EqualFold(e.Name, "32052-02.htm"):
		st.SetCond(3)
		s.PlaySound(p, script.SoundMiddle)
	case strings.EqualFold(e.Name, "32053-04.htm"):
		st.SetCond(4)
		s.PlaySound(p, script.SoundMiddle)
	case strings.EqualFold(e.Name, "32076-02.htm"):
		st.SetCond(5)
		s.PlaySound(p, script.SoundMiddle)
		s.GiveItems(p, engravedHammer, 1)
	case strings.EqualFold(e.Name, "32053-06.htm"):
		st.SetCond(6)
		s.PlaySound(p, script.SoundMiddle)
	case strings.EqualFold(e.Name, "32052-04.htm"):
		st.SetCond(7)
		s.PlaySound(p, script.SoundMiddle)
	case strings.EqualFold(e.Name, "32052-06.htm"):
		st.SetCond(9)
		s.PlaySound(p, script.SoundMiddle)
		s.TakeItems(p, greyStar, 1)
	case strings.EqualFold(e.Name, "32055-04.htm"):
		st.SetCond(10)
		s.PlaySound(p, script.SoundMiddle)
		s.TakeItems(p, engravedHammer, 1)
	case strings.EqualFold(e.Name, "32054-03.htm"):
		s.RewardExpAndSp(p, 63591, 0)
		s.PlaySound(p, script.SoundFinish)
		st.Exit(false)
	}
	return htmltext
}

func onTalk(s *script.Script, e script.Talk) string {
	st := s.QuestState(e.Player, questName)
	htmltext := script.NoQuestMsg()
	if st == nil {
		return htmltext
	}
	switch st.Status() {
	case questlog.StatusCreated:
		if e.Player.Level() < 39 {
			htmltext = "32053-00.htm"
		} else {
			htmltext = "32053-01.htm"
		}
	case questlog.StatusStarted:
		cond := st.Cond()
		switch e.NPC.NpcID() {
		case ancientGhost:
			switch {
			case cond == 1:
				htmltext = "32055-01.htm"
			case cond > 1 && cond < 9:
				htmltext = "32055-02.htm"
			case cond == 9:
				htmltext = "32055-03.htm"
			case cond > 9:
				htmltext = "32055-05.htm"
			}
		case obi:
			switch {
			case cond == 2:
				htmltext = "32052-01.htm"
			case cond > 2 && cond < 6:
				htmltext = "32052-02.htm"
			case cond == 6:
				htmltext = "32052-03.htm"
			case cond == 7:
				htmltext = "32052-04.htm"
			case cond == 8:
				htmltext = "32052-05.htm"
			case cond > 8:
				htmltext = "32052-06.htm"
			}
		case abey:
			switch {
			case cond == 1 || cond == 2:
				htmltext = "32053-02.htm"
			case cond == 3:
				htmltext = "32053-03.htm"
			case cond == 4:
				htmltext = "32053-04.htm"
			case cond == 5:
				htmltext = "32053-05.htm"
			case cond > 5:
				htmltext = "32053-06.htm"
			}
		case box:
			switch {
			case cond == 4:
				htmltext = "32076-01.htm"
			case cond > 4:
				htmltext = "32076-03.htm"
			}
		case ghost:
			if cond == 10 {
				htmltext = "32054-01.htm"
			}
		}
	case questlog.StatusCompleted:
		htmltext = script.AlreadyCompletedMsg()
	}
	return htmltext
}

func onMyDying(s *script.Script, e script.MyDying) {
	p := script.ActingPlayer(e.Killer)
	st := s.CheckPlayerCondition(p, e.NPC, 7)
	if st == nil {
		return
	}
	if s.DropItems(p, greyStar, 1, 1, 200000) {
		st.SetCond(8)
	}
}
