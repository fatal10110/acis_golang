// Package q606 is the quest War with Varka Silenos: Kadun Zu Ketra asks a
// Ketra ally of level 74 or more for Varka manes, and pays 20 Horns of
// Buffalo for every 100. The manes drop through the Alliance with Ketra
// Orcs quest, which has no kill hook here to share.
package q606

import (
	"strings"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/questlog"
	"github.com/fatal10110/acis_golang/internal/gameserver/script"
)

const questName = "Q606_WarWithVarkaSilenos"

// kadun is the quest giver, Kadun Zu Ketra.
const kadun = 31370

// Items.
const (
	hornOfBuffalo = 7186
	varkaMane     = 7233
)

// New returns the quest.
func New() script.Script {
	return script.Script{
		Title:   "War with Varka Silenos",
		QuestID: 606,
		Items:   []int32{varkaMane},
		Bind: script.Bindings{
			script.EventQuestStart: {kadun},
			script.EventTalked:     {kadun},
		},
		Hooks: script.Hooks{OnEvent: onAdvEvent, OnTalk: onTalk},
	}
}

// onAdvEvent starts the quest, trades 100 manes for 20 horns, or ends the
// quest and takes every mane.
func onAdvEvent(s *script.Script, e script.Event) string {
	htmltext := e.Name
	st := s.QuestState(e.Player, questName)
	if st == nil {
		return htmltext
	}
	switch {
	case strings.EqualFold(e.Name, "31370-03.htm"):
		st.SetStatus(questlog.StatusStarted)
		st.SetCond(1)
		s.PlaySound(e.Player, script.SoundAccept)
	case strings.EqualFold(e.Name, "31370-07.htm"):
		if e.Player.ItemCount(varkaMane) >= 100 {
			s.PlaySound(e.Player, script.SoundItemGet)
			s.TakeItems(e.Player, varkaMane, 100)
			s.GiveItems(e.Player, hornOfBuffalo, 20)
		} else {
			htmltext = "31370-08.htm"
		}
	case strings.EqualFold(e.Name, "31370-09.htm"):
		s.TakeItems(e.Player, varkaMane, -1)
		st.Exit(true)
	}
	return htmltext
}

// onTalk offers the quest to a Ketra ally of level 74 or more.
func onTalk(s *script.Script, e script.Talk) string {
	htmltext := script.NoQuestMsg()
	st := s.QuestState(e.Player, questName)
	if st == nil {
		return htmltext
	}
	switch st.Status() {
	case questlog.StatusCreated:
		if e.Player.Level() >= 74 && e.Player.AlliedWithKetra() {
			htmltext = "31370-01.htm"
		} else {
			htmltext = "31370-02.htm"
		}
	case questlog.StatusStarted:
		if e.Player.HasItem(varkaMane) {
			htmltext = "31370-04.htm"
		} else {
			htmltext = "31370-05.htm"
		}
	}
	return htmltext
}
