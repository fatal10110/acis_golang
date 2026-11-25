package ketravarka

import (
	"strings"

	"github.com/fatal10110/acis_golang/internal/gameserver/script"
)

const q613Name = "Q613_ProveYourCourage"

// hekaton is the Ketra raid boss whose head Q613 asks for.
const hekaton = 25299

const headOfHekaton = 7240

// q613 is the quest; the head and the trade are the commander's.
type q613 struct{ commander }

// Q613 is the quest Prove Your Courage: Ashas Varka Durai asks a level 75
// Varka ally of alliance level 3 or more for the head of Hekaton, the Ketra
// raid boss, and pays for it with the Feather of Valor.
func Q613() script.Script {
	q := q613{commander{tribe: varka(), name: q613Name, level: 3, head: headOfHekaton, totem: featherOfValor}}
	return script.Script{
		Title:   "Prove your courage!",
		QuestID: 613,
		Items:   []int32{headOfHekaton},
		Bind: script.Bindings{
			script.EventQuestStart: {ashas},
			script.EventTalked:     {ashas},
			script.EventMyDying:    {hekaton},
		},
		Hooks: script.Hooks{OnEvent: q.onAdvEvent, OnTalk: q.onTalk, OnMyDying: q.onMyDying},
	}
}

// onAdvEvent starts the quest, or trades the head for the totem.
func (q q613) onAdvEvent(s *script.Script, e script.Event) string {
	htmltext := e.Name
	st := s.QuestState(e.Player, q613Name)
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

func (q q613) onTalk(s *script.Script, e script.Talk) string {
	return q.talk(s, e, "31377-03.htm", "31377-01.htm", "31377-02.htm", "31377-05.htm", "31377-06.htm")
}
