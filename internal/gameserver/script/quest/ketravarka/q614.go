package ketravarka

import (
	"strings"

	"github.com/fatal10110/acis_golang/internal/gameserver/script"
)

const q614Name = "Q614_SlayTheEnemyCommander"

// tayr is the Ketra raid boss whose head Q614 asks for.
const tayr = 25302

const headOfTayr = 7241

// q614 is the quest; the head and the trade are the commander's.
type q614 struct{ commander }

// Q614 is the quest Slay the Enemy Commander: Ashas Varka Durai asks a level
// 75 Varka ally of alliance level 4 or more for the head of Tayr, the Ketra
// raid boss, and pays for it with the Feather of Wisdom.
func Q614() script.Script {
	q := q614{commander{tribe: varka(), name: q614Name, level: 4, head: headOfTayr, totem: featherOfWisdom}}
	return script.Script{
		Title:   "Slay the enemy commander!",
		QuestID: 614,
		Items:   []int32{headOfTayr},
		Bind: script.Bindings{
			script.EventQuestStart: {ashas},
			script.EventTalked:     {ashas},
			script.EventMyDying:    {tayr},
		},
		Hooks: script.Hooks{OnEvent: q.onAdvEvent, OnTalk: q.onTalk, OnMyDying: q.onMyDying},
	}
}

// onAdvEvent starts the quest, or trades the head for the totem.
func (q q614) onAdvEvent(s *script.Script, e script.Event) string {
	htmltext := e.Name
	st := s.QuestState(e.Player, q614Name)
	if st == nil {
		return htmltext
	}
	switch {
	case strings.EqualFold(e.Name, "31377-04.htm"):
		q.start(s, e.Player, st)
	case strings.EqualFold(e.Name, "31377-07.htm"):
		if !q.trade(s, e.Player, st) {
			htmltext = "31377-06.htm"
		}
	}
	return htmltext
}

func (q q614) onTalk(s *script.Script, e script.Talk) string {
	return q.talk(s, e, "31377-03.htm", "31377-01.htm", "31377-02.htm", "31377-05.htm", "31377-06.htm")
}
