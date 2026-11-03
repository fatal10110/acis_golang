// Package q647 is the quest Influx of Machines: Gutenhagen, for a character
// of level 46 or more, collects the shards of destroyed golems and pays a
// random recipe with them.
package q647

import (
	"strings"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/questlog"
	"github.com/fatal10110/acis_golang/internal/gameserver/script"
)

const questName = "Q647_InfluxOfMachines"

// Items.
const destroyedGolemShard = 8100

// NPC.
const gutenhagen = 32069

// New returns the quest.
func New() script.Script {
	var golems []int32
	for id := int32(22052); id < 22079; id++ {
		golems = append(golems, id)
	}
	return script.Script{
		Title:   "Influx of Machines",
		QuestID: 647,
		Items:   []int32{destroyedGolemShard},
		Bind: script.Bindings{
			script.EventQuestStart: {gutenhagen},
			script.EventTalked:     {gutenhagen},
			script.EventMyDying:    golems,
		},
		Hooks: script.Hooks{OnEvent: onAdvEvent, OnTalk: onTalk, OnMyDying: onMyDying},
	}
}

// onAdvEvent starts the quest, or takes the shards in for a random recipe and
// ends the quest.
func onAdvEvent(s *script.Script, e script.Event) string {
	htmltext := e.Name
	st := s.QuestState(e.Player, questName)
	if st == nil {
		return htmltext
	}
	switch {
	case strings.EqualFold(e.Name, "32069-02.htm"):
		st.SetStatus(questlog.StatusStarted)
		st.SetCond(1)
		s.PlaySound(e.Player, script.SoundAccept)
	case strings.EqualFold(e.Name, "32069-06.htm"):
		s.TakeItems(e.Player, destroyedGolemShard, -1)
		s.GiveItems(e.Player, int32(4963+s.Rnd(4972-4963+1)), 1)
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
		if e.Player.Level() < 46 {
			htmltext = "32069-03.htm"
		} else {
			htmltext = "32069-01.htm"
		}
	case questlog.StatusStarted:
		switch st.Cond() {
		case 1:
			htmltext = "32069-04.htm"
		case 2:
			htmltext = "32069-05.htm"
		}
	}
	return htmltext
}

// onMyDying drops a shard to a party member at cond 1, and moves the one who
// then holds five hundred to cond 2.
func onMyDying(s *script.Script, e script.MyDying) {
	p := script.ActingPlayer(e.Killer)
	st := s.RandomPartyMember(p, e.NPC, script.ByCond(1))
	if st == nil {
		return
	}
	if s.DropItems(st.Player(), destroyedGolemShard, 1, 500, 300000) {
		st.SetCond(2)
	}
}
