// Package q162 is the quest Curse of the Underground Fortress: Unoren pays
// a bone shield and adena for ten bone fragments and three elf skulls.
package q162

import (
	"strings"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/player"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/questlog"
	"github.com/fatal10110/acis_golang/internal/gameserver/script"
)

const questName = "Q162_CurseOfTheUndergroundFortress"

// Monsters.
const (
	shadeHorror           = 20033
	darkTerror            = 20345
	mistTerror            = 20371
	dungeonSkeletonArcher = 20463
	dungeonSkeleton       = 20464
	dreadSoldier          = 20504
)

// Items.
const (
	boneFragment = 1158
	elfSkull     = 1159
)

// Reward.
const boneShield = 625

// chances returns the drop chance of each monster, by NPC id.
func chances() map[int32]int32 {
	return map[int32]int32{
		shadeHorror:           250000,
		darkTerror:            260000,
		mistTerror:            230000,
		dungeonSkeletonArcher: 250000,
		dungeonSkeleton:       230000,
		dreadSoldier:          260000,
	}
}

// New returns the quest.
func New() script.Script {
	return script.Script{
		Title:   "Curse of the Underground Fortress",
		QuestID: 162,
		Items:   []int32{boneFragment, elfSkull},
		Bind: script.Bindings{
			script.EventQuestStart: {30147}, // Unoren
			script.EventTalked:     {30147},
			script.EventMyDying:    {shadeHorror, darkTerror, mistTerror, dungeonSkeletonArcher, dungeonSkeleton, dreadSoldier},
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
	if strings.EqualFold(e.Name, "30147-04.htm") {
		st.SetStatus(questlog.StatusStarted)
		st.SetCond(1)
		s.PlaySound(e.Player, script.SoundAccept)
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
		switch {
		case p.Race() == player.RaceDarkElf:
			htmltext = "30147-00.htm"
		case p.Level() < 12:
			htmltext = "30147-01.htm"
		default:
			htmltext = "30147-02.htm"
		}
	case questlog.StatusStarted:
		switch st.Cond() {
		case 1:
			htmltext = "30147-05.htm"
		case 2:
			htmltext = "30147-06.htm"
			s.TakeItems(p, elfSkull, -1)
			s.TakeItems(p, boneFragment, -1)
			s.GiveItems(p, boneShield, 1)
			s.RewardItems(p, 57, 24000)
			s.PlaySound(p, script.SoundFinish)
			st.Exit(false)
		}
	case questlog.StatusCompleted:
		htmltext = script.AlreadyCompletedMsg()
	}
	return htmltext
}

func onMyDying(s *script.Script, e script.MyDying) {
	p := script.ActingPlayer(e.Killer)
	st := s.CheckPlayerCondition(p, e.NPC, 1)
	if st == nil {
		return
	}
	npcID := e.NPC.NpcID()
	switch npcID {
	case dungeonSkeleton, dungeonSkeletonArcher, dreadSoldier:
		if s.DropItems(p, boneFragment, 1, 10, chances()[npcID]) && p.ItemCount(elfSkull) >= 3 {
			st.SetCond(2)
		}
	case shadeHorror, darkTerror, mistTerror:
		if s.DropItems(p, elfSkull, 1, 3, chances()[npcID]) && p.ItemCount(boneFragment) >= 10 {
			st.SetCond(2)
		}
	}
}
