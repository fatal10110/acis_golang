// Package q316 is the quest Destroy Plague Carriers: Ellenia pays an elf
// for the fangs of the Sukar wererats and of Varool Foulclaw, who cries out
// the first time he is struck.
package q316

import (
	"strings"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/player"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/questlog"
	"github.com/fatal10110/acis_golang/internal/gameserver/script"
)

const questName = "Q316_DestroyPlagueCarriers"

// Items.
const (
	wereratFang        = 1042
	varoolFoulclawFang = 1043
)

// Monsters.
const (
	sukarWererat       = 20040
	sukarWereratLeader = 20047
	varoolFoulclaw     = 27020
)

// ellenia is the quest giver.
const ellenia = 30155

// varoolCry is the NpcString Varool Foulclaw cries out when first struck.
const varoolCry = 31603

// New returns the quest.
func New() script.Script {
	return script.Script{
		Title:   "Destroy Plague Carriers",
		QuestID: 316,
		Items:   []int32{wereratFang, varoolFoulclawFang},
		Bind: script.Bindings{
			script.EventQuestStart: {ellenia},
			script.EventTalked:     {ellenia},
			script.EventAttacked:   {varoolFoulclaw},
			script.EventMyDying:    {sukarWererat, sukarWereratLeader, varoolFoulclaw},
		},
		Hooks: script.Hooks{OnEvent: onAdvEvent, OnTalk: onTalk, OnAttacked: onAttacked, OnMyDying: onMyDying},
	}
}

func onAdvEvent(s *script.Script, e script.Event) string {
	htmltext := e.Name
	st := s.QuestState(e.Player, questName)
	if st == nil {
		return htmltext
	}
	switch {
	case strings.EqualFold(e.Name, "30155-04.htm"):
		st.SetStatus(questlog.StatusStarted)
		st.SetCond(1)
		s.PlaySound(e.Player, script.SoundAccept)
	case strings.EqualFold(e.Name, "30155-08.htm"):
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
	switch st.Status() {
	case questlog.StatusCreated:
		switch {
		case e.Player.Race() != player.RaceElf:
			htmltext = "30155-00.htm"
		case e.Player.Level() < 18:
			htmltext = "30155-02.htm"
		default:
			htmltext = "30155-03.htm"
		}
	case questlog.StatusStarted:
		ratFangs := e.Player.ItemCount(wereratFang)
		varoolFangs := e.Player.ItemCount(varoolFoulclawFang)
		if ratFangs+varoolFangs == 0 {
			htmltext = "30155-05.htm"
		} else {
			htmltext = "30155-07.htm"
			s.TakeItems(e.Player, wereratFang, -1)
			s.TakeItems(e.Player, varoolFoulclawFang, -1)
			bonus := int32(0)
			if ratFangs > 10 {
				bonus = 5000
			}
			s.RewardItems(e.Player, 57, ratFangs*30+varoolFangs*10000+bonus)
		}
	}
	return htmltext
}

// onAttacked makes Varool Foulclaw cry out once per life.
func onAttacked(_ *script.Script, e script.Attacked) {
	if e.NPC.ScriptValue() == 0 {
		e.NPC.Say(script.NpcString(varoolCry))
		e.NPC.SetScriptValue(1)
	}
}

func onMyDying(s *script.Script, e script.MyDying) {
	p := script.ActingPlayer(e.Killer)
	if s.CheckPlayerState(p, e.NPC, questlog.StatusStarted) == nil {
		return
	}
	switch e.NPC.NpcID() {
	case sukarWererat, sukarWereratLeader:
		s.DropItems(p, wereratFang, 1, 0, 400000)
	case varoolFoulclaw:
		s.DropItems(p, varoolFoulclawFang, 1, 1, 200000)
	}
}
