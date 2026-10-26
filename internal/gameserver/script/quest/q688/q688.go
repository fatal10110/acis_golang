// Package q688 is the quest Defeat the Elrokian Raiders: Dinn pays adena for
// the dinosaur fang necklaces the elrokis drop to a party member at random.
package q688

import (
	"strings"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/questlog"
	"github.com/fatal10110/acis_golang/internal/gameserver/script"
)

const questName = "Q688_DefeatTheElrokianRaiders"

// NPC.
const dinn = 32105

// Item.
const dinosaurFangNecklace = 8785

// Monster.
const elroki = 22214

// New returns the quest.
func New() script.Script {
	return script.Script{
		Title:   "Defeat the Elrokian Raiders!",
		QuestID: 688,
		Items:   []int32{dinosaurFangNecklace},
		Bind: script.Bindings{
			script.EventQuestStart: {dinn},
			script.EventTalked:     {dinn},
			script.EventMyDying:    {elroki},
		},
		Hooks: script.Hooks{OnEvent: onAdvEvent, OnTalk: onTalk, OnMyDying: onMyDying},
	}
}

// onAdvEvent starts the quest, pays adena for the necklaces, or ends the
// quest.
func onAdvEvent(s *script.Script, e script.Event) string {
	htmltext := e.Name
	st := s.QuestState(e.Player, questName)
	if st == nil {
		return htmltext
	}
	switch {
	case strings.EqualFold(e.Name, "32105-03.htm"):
		st.SetStatus(questlog.StatusStarted)
		st.SetCond(1)
		s.PlaySound(e.Player, script.SoundAccept)
	case strings.EqualFold(e.Name, "32105-08.htm"):
		count := e.Player.ItemCount(dinosaurFangNecklace)
		if count > 0 {
			s.TakeItems(e.Player, dinosaurFangNecklace, -1)
			s.RewardItems(e.Player, 57, count*3000)
		}
		s.PlaySound(e.Player, script.SoundFinish)
		st.Exit(true)
	case strings.EqualFold(e.Name, "32105-06.htm"):
		count := e.Player.ItemCount(dinosaurFangNecklace)
		s.TakeItems(e.Player, dinosaurFangNecklace, -1)
		s.RewardItems(e.Player, 57, count*3000)
	case strings.EqualFold(e.Name, "32105-07.htm"):
		count := e.Player.ItemCount(dinosaurFangNecklace)
		if count >= 100 {
			s.TakeItems(e.Player, dinosaurFangNecklace, 100)
			s.RewardItems(e.Player, 57, 450000)
		} else {
			htmltext = "32105-04.htm"
		}
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
		if e.Player.Level() < 75 {
			htmltext = "32105-00.htm"
		} else {
			htmltext = "32105-01.htm"
		}
	case questlog.StatusStarted:
		if !e.Player.HasItem(dinosaurFangNecklace) {
			htmltext = "32105-04.htm"
		} else {
			htmltext = "32105-05.htm"
		}
	}
	return htmltext
}

// onMyDying drops a necklace to one party member at random among those with
// the quest.
func onMyDying(s *script.Script, e script.MyDying) {
	p := script.ActingPlayer(e.Killer)
	st := s.RandomPartyMemberState(p, e.NPC, questlog.StatusStarted)
	if st == nil {
		return
	}
	s.DropItems(st.Player(), dinosaurFangNecklace, 1, 0, 500000)
}
