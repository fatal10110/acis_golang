// Package q375 is the quest Whisper of Dreams, Part 2: Manakia asks for the
// horns of the Karik and the skulls of the Cave Howlers in exchange for a
// recipe.
package q375

import (
	"strings"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/questlog"
	"github.com/fatal10110/acis_golang/internal/gameserver/script"
)

const questName = "Q375_WhisperOfDreams_Part2"

// Npcs.
const manakia = 30515

// Monsters.
const (
	karik      = 20629
	caveHowler = 20624
)

// Quest items.
const (
	mysteriousStone = 5887
	karikHorn       = 5888
	caveHowlerSkull = 5889
)

// rewards lists the recipes Manakia may give.
func rewards() []int32 { return []int32{5348, 5350, 5352} }

// New returns the quest.
func New() script.Script {
	return script.Script{
		Title:   "Whisper of Dreams, Part 2",
		QuestID: 375,
		Items:   []int32{karikHorn, caveHowlerSkull},
		Bind: script.Bindings{
			script.EventQuestStart: {manakia},
			script.EventTalked:     {manakia},
			script.EventMyDying:    {karik, caveHowler},
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
	case strings.EqualFold(e.Name, "30515-03.htm"):
		st.SetStatus(questlog.StatusStarted)
		st.SetCond(1)
		s.PlaySound(p, script.SoundAccept)
		s.TakeItems(p, mysteriousStone, 1)
	case strings.EqualFold(e.Name, "30515-07.htm"):
		s.PlaySound(p, script.SoundFinish)
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
		if !p.HasItem(mysteriousStone) || p.Level() < 60 {
			htmltext = "30515-01.htm"
		} else {
			htmltext = "30515-02.htm"
		}
	case questlog.StatusStarted:
		if p.ItemCount(karikHorn) >= 100 && p.ItemCount(caveHowlerSkull) >= 100 {
			htmltext = "30515-05.htm"
			s.PlaySound(p, script.SoundMiddle)
			s.TakeItems(p, karikHorn, 100)
			s.TakeItems(p, caveHowlerSkull, 100)
			recipes := rewards()
			s.GiveItems(p, recipes[s.Rnd(len(recipes))], 1)
		} else {
			htmltext = "30515-04.htm"
		}
	}
	return htmltext
}

func onMyDying(s *script.Script, e script.MyDying) {
	st := s.RandomPartyMemberState(script.ActingPlayer(e.Killer), e.NPC, questlog.StatusStarted)
	if st == nil {
		return
	}
	switch e.NPC.NpcID() {
	case karik:
		s.DropItemsAlways(st.Player(), karikHorn, 1, 100)
	case caveHowler:
		s.DropItems(st.Player(), caveHowlerSkull, 1, 100, 900000)
	}
}
