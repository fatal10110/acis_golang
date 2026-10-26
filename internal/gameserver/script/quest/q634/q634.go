// Package q634 is the quest In Search of Fragments of Dimension: a
// Dimensional Gate Keeper, for a character of level 20 or more, lets the
// character collect dimension fragments from the monsters of the dimensional
// rift, as long as it keeps the quest.
package q634

import (
	"strings"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/questlog"
	"github.com/fatal10110/acis_golang/internal/gameserver/script"
)

const questName = "Q634_InSearchOfFragmentsOfDimension"

// Items.
const dimensionFragment = 7079

// gatekeepers returns the Dimensional Gate Keepers.
func gatekeepers() []int32 {
	ids := make([]int32, 0)
	for id := int32(31494); id < 31508; id++ {
		ids = append(ids, id)
	}
	return ids
}

// monsters returns every monster of the rift.
func monsters() []int32 {
	ids := make([]int32, 0)
	for id := int32(21208); id < 21256; id++ {
		ids = append(ids, id)
	}
	return ids
}

// New returns the quest.
func New() script.Script {
	return script.Script{
		Title:   "In Search of Fragments of Dimension",
		QuestID: 634,
		Bind: script.Bindings{
			script.EventQuestStart: gatekeepers(),
			script.EventTalked:     gatekeepers(),
			script.EventMyDying:    monsters(),
		},
		Hooks: script.Hooks{OnEvent: onAdvEvent, OnTalk: onTalk, OnMyDying: onMyDying},
	}
}

// onAdvEvent starts the quest, or ends it.
func onAdvEvent(s *script.Script, e script.Event) string {
	htmltext := e.Name
	st := s.QuestState(e.Player, questName)
	if st == nil {
		return htmltext
	}
	switch {
	case strings.EqualFold(e.Name, "02.htm"):
		st.SetStatus(questlog.StatusStarted)
		st.SetCond(1)
		s.PlaySound(e.Player, script.SoundAccept)
	case strings.EqualFold(e.Name, "05.htm"):
		s.PlaySound(e.Player, script.SoundFinish)
		st.Exit(true)
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
		if e.Player.Level() < 20 {
			htmltext = "01a.htm"
		} else {
			htmltext = "01.htm"
		}
	case questlog.StatusStarted:
		htmltext = "03.htm"
	}
	return htmltext
}

// onMyDying drops dimension fragments to a party member who has the quest,
// more of them the higher the monster's level.
func onMyDying(s *script.Script, e script.MyDying) {
	p := script.ActingPlayer(e.Killer)
	st := s.RandomPartyMemberState(p, e.NPC, questlog.StatusStarted)
	if st == nil {
		return
	}
	// The conversion keeps the product from fusing with the sum on a CPU that fuses.
	s.DropItems(st.Player(), dimensionFragment, int32(float64(float64(e.NPC.Level())*0.15)+1.6), -1, 900000)
}
