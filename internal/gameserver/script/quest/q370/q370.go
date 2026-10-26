// Package q370 is the quest An Elder Sows Seeds: Casian collects the spellbook
// pages of the Hallate's and Kaiser's monsters and pays for a full set of
// chapters.
package q370

import (
	"strings"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/questlog"
	"github.com/fatal10110/acis_golang/internal/gameserver/script"
)

const questName = "Q370_AnElderSowsSeeds"

// casian is the quest giver.
const casian = 30612

// Quest items.
const (
	spellbookPage  = 5916
	chapterOfFire  = 5917
	chapterOfWater = 5918
	chapterOfWind  = 5919
	chapterOfEarth = 5920
)

// New returns the quest.
func New() script.Script {
	return script.Script{
		Title:   "An Elder Sows Seeds",
		QuestID: 370,
		Items:   []int32{spellbookPage, chapterOfFire, chapterOfWater, chapterOfWind, chapterOfEarth},
		Bind: script.Bindings{
			script.EventQuestStart: {casian},
			script.EventTalked:     {casian},
			script.EventMyDying:    {20082, 20084, 20086, 20089, 20090},
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
	case strings.EqualFold(e.Name, "30612-3.htm"):
		st.SetStatus(questlog.StatusStarted)
		st.SetCond(1)
		s.PlaySound(p, script.SoundAccept)
	case strings.EqualFold(e.Name, "30612-6.htm"):
		if p.HasItems(chapterOfFire, chapterOfWater, chapterOfWind, chapterOfEarth) {
			htmltext = "30612-8.htm"
			s.TakeItems(p, chapterOfFire, 1)
			s.TakeItems(p, chapterOfWater, 1)
			s.TakeItems(p, chapterOfWind, 1)
			s.TakeItems(p, chapterOfEarth, 1)
			s.RewardItems(p, 57, 3600)
		}
	case strings.EqualFold(e.Name, "30612-9.htm"):
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
	switch st.Status() {
	case questlog.StatusCreated:
		if e.Player.Level() < 28 {
			htmltext = "30612-0a.htm"
		} else {
			htmltext = "30612-0.htm"
		}
	case questlog.StatusStarted:
		htmltext = "30612-4.htm"
	}
	return htmltext
}

func onMyDying(s *script.Script, e script.MyDying) {
	st := s.RandomPartyMemberState(script.ActingPlayer(e.Killer), e.NPC, questlog.StatusStarted)
	if st == nil {
		return
	}
	s.DropItems(st.Player(), spellbookPage, 1, 0, chance(e.NPC.NpcID()))
}

// chance is the drop chance of a monster's spellbook page.
func chance(npcID int32) int32 {
	switch npcID {
	case 20082:
		return 86000
	case 20084:
		return 94000
	case 20086:
		return 90000
	case 20089:
		return 100000
	default:
		return 202000
	}
}
