// Package q320 is the quest Bones Tell the Future: Kaitar pays a dark elf
// for ten bone fragments.
package q320

import (
	"strings"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/player"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/questlog"
	"github.com/fatal10110/acis_golang/internal/gameserver/script"
)

const questName = "Q320_BonesTellTheFuture"

// Quest item.
const boneFragment = 809

// New returns the quest.
func New() script.Script {
	return script.Script{
		Title:   "Bones Tell the Future",
		QuestID: 320,
		Items:   []int32{boneFragment},
		Bind: script.Bindings{
			script.EventQuestStart: {30359}, // Kaitar
			script.EventTalked:     {30359},
			script.EventMyDying:    {20517, 20518},
		},
		Hooks: script.Hooks{OnEvent: onAdvEvent, OnTalk: onTalk, OnMyDying: onMyDying},
	}
}

func onAdvEvent(s *script.Script, e script.Event) string {
	st := s.QuestState(e.Player, questName)
	if st == nil {
		return e.Name
	}
	if strings.EqualFold(e.Name, "30359-04.htm") {
		st.SetStatus(questlog.StatusStarted)
		st.SetCond(1)
		s.PlaySound(e.Player, script.SoundAccept)
	}
	return e.Name
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
		switch {
		case p.Race() != player.RaceDarkElf:
			htmltext = "30359-00.htm"
		case p.Level() < 10:
			htmltext = "30359-02.htm"
		default:
			htmltext = "30359-03.htm"
		}
	case questlog.StatusStarted:
		if st.Cond() == 1 {
			htmltext = "30359-05.htm"
		} else {
			htmltext = "30359-06.htm"
			s.TakeItems(p, boneFragment, -1)
			s.RewardItems(p, 57, 8470)
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
	chance := int32(200000)
	if e.NPC.NpcID() == 20517 {
		chance = 180000
	}
	if s.DropItems(p, boneFragment, 1, 10, chance) {
		st.SetCond(2)
	}
}
