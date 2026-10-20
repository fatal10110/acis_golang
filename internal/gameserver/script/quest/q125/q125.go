// Package q125 is the quest The Name of Evil - 1: Mushika sends the
// character to Karakawei, who asks for ornithomimus claws and deinonychus
// bones, then to the three Kaimu stones, which turn Gazkh's fragment into
// the epitaph of wisdom.
package q125

import (
	"slices"
	"strings"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/questlog"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/script"
)

const questName = "Q125_TheNameOfEvil_1"

// NPCs.
const (
	mushika    = 32114
	karakawei  = 32117
	uluKaimu   = 32119
	baluKaimu  = 32120
	chutaKaimu = 32121
)

// Items.
const (
	ornithomimusClaw = 8779
	deinonychusBone  = 8780
	epitaphOfWisdom  = 8781
	gazkhFragment    = 8782
)

// ornithomimus lists the monsters that drop the claws.
func ornithomimus() []int32 {
	return []int32{22200, 22201, 22202, 22219, 22224, 22742, 22744}
}

// deinonychus lists the monsters that drop the bones.
func deinonychus() []int32 {
	return []int32{16067, 22203, 22204, 22205, 22220, 22225, 22743, 22745}
}

// New returns the quest.
func New() script.Script {
	return script.Script{
		Title:   "The Name of Evil - 1",
		QuestID: 125,
		Items:   []int32{ornithomimusClaw, deinonychusBone, epitaphOfWisdom, gazkhFragment},
		Bind: script.Bindings{
			script.EventQuestStart: {mushika},
			script.EventTalked:     {mushika, karakawei, uluKaimu, baluKaimu, chutaKaimu},
			script.EventMyDying:    slices.Concat(ornithomimus(), deinonychus()),
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
	case strings.EqualFold(e.Name, "32114-05.htm"):
		st.SetStatus(questlog.StatusStarted)
		st.SetCond(1)
		s.PlaySound(p, script.SoundAccept)
	case strings.EqualFold(e.Name, "32114-09.htm"):
		st.SetCond(2)
		s.PlaySound(p, script.SoundMiddle)
		s.GiveItems(p, gazkhFragment, 1)
	case strings.EqualFold(e.Name, "32117-08.htm"):
		st.SetCond(3)
		s.PlaySound(p, script.SoundMiddle)
	case strings.EqualFold(e.Name, "32117-14.htm"):
		st.SetCond(5)
		s.PlaySound(p, script.SoundMiddle)
	case strings.EqualFold(e.Name, "32119-14.htm"):
		st.SetCond(6)
		s.PlaySound(p, script.SoundMiddle)
	case strings.EqualFold(e.Name, "32120-15.htm"):
		st.SetCond(7)
		s.PlaySound(p, script.SoundMiddle)
	case strings.EqualFold(e.Name, "32121-16.htm"):
		st.SetCond(8)
		s.PlaySound(p, script.SoundMiddle)
		s.TakeItems(p, gazkhFragment, -1)
		s.GiveItems(p, epitaphOfWisdom, 1)
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
	npc := e.NPC
	switch st.Status() {
	case questlog.StatusCreated:
		first := s.QuestState(p, "Q124_MeetingTheElroki")
		if first != nil && first.IsCompleted() && p.Level() >= 76 {
			htmltext = "32114-01.htm"
		} else {
			htmltext = "32114-00.htm"
		}
	case questlog.StatusStarted:
		cond := st.Cond()
		switch npc.NpcID() {
		case mushika:
			switch {
			case cond == 1:
				htmltext = "32114-07.htm"
			case cond == 2:
				htmltext = "32114-10.htm"
			case cond > 2 && cond < 8:
				htmltext = "32114-11.htm"
			case cond == 8:
				htmltext = "32114-12.htm"
				s.TakeItems(p, epitaphOfWisdom, -1)
				s.PlaySound(p, script.SoundFinish)
				st.Exit(false)
			}
		case karakawei:
			switch cond {
			case 2:
				htmltext = "32117-01.htm"
			case 3:
				htmltext = "32117-09.htm"
			case 4:
				if p.ItemCount(ornithomimusClaw) >= 2 && p.ItemCount(deinonychusBone) >= 2 {
					htmltext = "32117-10.htm"
					s.TakeItems(p, ornithomimusClaw, -1)
					s.TakeItems(p, deinonychusBone, -1)
					s.PlaySound(p, script.SoundMiddle)
				} else {
					htmltext = "32117-09.htm"
					st.SetCond(3)
				}
			case 5:
				htmltext = "32117-15.htm"
			case 6, 7:
				htmltext = "32117-16.htm"
			case 8:
				htmltext = "32117-17.htm"
			}
		case uluKaimu:
			switch cond {
			case 5:
				npc.AddCastDesire(npc, skill.Ref{ID: 5089, Level: 1}, 1000000)
				htmltext = "32119-01.htm"
			case 6:
				htmltext = "32119-14.htm"
			}
		case baluKaimu:
			switch cond {
			case 6:
				npc.AddCastDesire(npc, skill.Ref{ID: 5089, Level: 1}, 1000000)
				htmltext = "32120-01.htm"
			case 7:
				htmltext = "32120-16.htm"
			}
		case chutaKaimu:
			switch cond {
			case 7:
				npc.AddCastDesire(npc, skill.Ref{ID: 5089, Level: 1}, 1000000)
				htmltext = "32121-01.htm"
			case 8:
				htmltext = "32121-17.htm"
			}
		}
	case questlog.StatusCompleted:
		htmltext = script.AlreadyCompletedMsg()
	}
	return htmltext
}

func onMyDying(s *script.Script, e script.MyDying) {
	p := script.ActingPlayer(e.Killer)
	st := s.CheckPlayerCondition(p, e.NPC, 3)
	if st == nil {
		return
	}
	npcID := e.NPC.NpcID()
	switch {
	case slices.Contains(ornithomimus(), npcID):
		if s.DropItems(p, ornithomimusClaw, 1, 2, 50000) && p.ItemCount(deinonychusBone) == 2 {
			st.SetCond(4)
		}
	case slices.Contains(deinonychus(), npcID):
		if s.DropItems(p, deinonychusBone, 1, 2, 50000) && p.ItemCount(ornithomimusClaw) == 2 {
			st.SetCond(4)
		}
	}
}
