package ketravarka

import (
	"strings"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/questlog"
	"github.com/fatal10110/acis_golang/internal/gameserver/script"
)

const nepenthesSeed = 7187

// Q612 is the quest War with Ketra Orcs: Ashas Varka Durai asks a Varka ally
// of level 74 or more for Ketra molars, and pays 20 Nepenthes Seeds for every
// 100. The molars drop through the kill hook of Alliance with Varka Silenos.
func Q612() script.Script {
	return script.Script{
		Title:   "War with Ketra Orcs",
		QuestID: 612,
		Items:   []int32{ketraMolar},
		Bind: script.Bindings{
			script.EventQuestStart: {ashas},
			script.EventTalked:     {ashas},
		},
		Hooks: script.Hooks{OnEvent: q612AdvEvent, OnTalk: q612Talk},
	}
}

// q612AdvEvent starts the quest, trades 100 molars for 20 seeds, or ends the
// quest and takes every molar.
func q612AdvEvent(s *script.Script, e script.Event) string {
	htmltext := e.Name
	st := s.QuestState(e.Player, q612Name)
	if st == nil {
		return htmltext
	}
	switch {
	case strings.EqualFold(e.Name, "31377-03.htm"):
		st.SetStatus(questlog.StatusStarted)
		st.SetCond(1)
		s.PlaySound(e.Player, script.SoundAccept)
	case strings.EqualFold(e.Name, "31377-07.htm"):
		if e.Player.ItemCount(ketraMolar) >= 100 {
			s.PlaySound(e.Player, script.SoundItemGet)
			s.TakeItems(e.Player, ketraMolar, 100)
			s.GiveItems(e.Player, nepenthesSeed, 20)
		} else {
			htmltext = "31377-08.htm"
		}
	case strings.EqualFold(e.Name, "31377-09.htm"):
		s.TakeItems(e.Player, ketraMolar, -1)
		st.Exit(true)
	}
	return htmltext
}

// q612Talk offers the quest to a Varka ally of level 74 or more.
func q612Talk(s *script.Script, e script.Talk) string {
	htmltext := script.NoQuestMsg()
	st := s.QuestState(e.Player, q612Name)
	if st == nil {
		return htmltext
	}
	switch st.Status() {
	case questlog.StatusCreated:
		if e.Player.Level() >= 74 && e.Player.AlliedWithVarka() {
			htmltext = "31377-01.htm"
		} else {
			htmltext = "31377-02.htm"
		}
	case questlog.StatusStarted:
		if e.Player.HasItem(ketraMolar) {
			htmltext = "31377-04.htm"
		} else {
			htmltext = "31377-05.htm"
		}
	}
	return htmltext
}
