// Package q003 is the quest Will the Seal be Broken?: a dark elf brings
// Talloth an onyx beast's eye, a taint stone and a succubus's blood, for a
// scroll of enchant armor (D grade).
package q003

import (
	"strings"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/player"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/questlog"
	"github.com/fatal10110/acis_golang/internal/gameserver/script"
)

const questName = "Q003_WillTheSealBeBroken"

// talloth is the quest giver.
const talloth = 30141

// Items.
const (
	onyxBeastEye  = 1081
	taintStone    = 1082
	succubusBlood = 1083
)

// Reward.
const scrollEnchantArmorD = 956

// Monsters.
const (
	omenBeast           = 20031
	taintedZombie       = 20041
	stinkZombie         = 20046
	lesserSuccubus      = 20048
	lesserSuccubusTuren = 20052
	lesserSuccubusTilfo = 20057
)

// New returns the quest.
func New() script.Script {
	return script.Script{
		Title:   "Will the Seal be Broken?",
		QuestID: 3,
		Items:   []int32{onyxBeastEye, taintStone, succubusBlood},
		Bind: script.Bindings{
			script.EventQuestStart: {talloth},
			script.EventTalked:     {talloth},
			script.EventMyDying: {
				omenBeast, taintedZombie, stinkZombie,
				lesserSuccubus, lesserSuccubusTuren, lesserSuccubusTilfo,
			},
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
	if strings.EqualFold(e.Name, "30141-03.htm") {
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
	switch st.Status() {
	case questlog.StatusCreated:
		switch {
		case e.Player.Race() != player.RaceDarkElf:
			htmltext = "30141-00.htm"
		case e.Player.Level() < 16:
			htmltext = "30141-01.htm"
		default:
			htmltext = "30141-02.htm"
		}
	case questlog.StatusStarted:
		switch st.Cond() {
		case 1:
			htmltext = "30141-04.htm"
		case 2:
			htmltext = "30141-06.htm"
			s.TakeItems(e.Player, onyxBeastEye, 1)
			s.TakeItems(e.Player, succubusBlood, 1)
			s.TakeItems(e.Player, taintStone, 1)
			s.GiveItems(e.Player, scrollEnchantArmorD, 1)
			s.PlaySound(e.Player, script.SoundFinish)
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
	switch e.NPC.NpcID() {
	case omenBeast:
		if s.DropItemsAlways(p, onyxBeastEye, 1, 1) && p.HasItems(taintStone, succubusBlood) {
			st.SetCond(2)
		}
	case taintedZombie, stinkZombie:
		if s.DropItemsAlways(p, taintStone, 1, 1) && p.HasItems(onyxBeastEye, succubusBlood) {
			st.SetCond(2)
		}
	case lesserSuccubus, lesserSuccubusTuren, lesserSuccubusTilfo:
		if s.DropItemsAlways(p, succubusBlood, 1, 1) && p.HasItems(onyxBeastEye, taintStone) {
			st.SetCond(2)
		}
	}
}
