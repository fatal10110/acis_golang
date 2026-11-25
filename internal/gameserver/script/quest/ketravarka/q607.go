package ketravarka

import (
	"strings"

	"github.com/fatal10110/acis_golang/internal/gameserver/script"
)

const q607Name = "Q607_ProveYourCourage"

// shadith is the Varka raid boss whose head Q607 asks for.
const shadith = 25309

const headOfShadith = 7235

// q607 is the quest; the head and the trade are the commander's.
type q607 struct{ commander }

// Q607 is the quest Prove Your Courage: Kadun Zu Ketra asks a level 75 Ketra
// ally of alliance level 3 or more for the head of Shadith, the Varka raid
// boss, and pays for it with the Totem of Valor.
func Q607() script.Script {
	q := q607{commander{tribe: ketra(), name: q607Name, level: 3, head: headOfShadith, totem: totemOfValor}}
	return script.Script{
		Title:   "Prove your courage!",
		QuestID: 607,
		Items:   []int32{headOfShadith},
		Bind: script.Bindings{
			script.EventQuestStart: {kadun},
			script.EventTalked:     {kadun},
			script.EventMyDying:    {shadith},
		},
		Hooks: script.Hooks{OnEvent: q.onAdvEvent, OnTalk: q.onTalk, OnMyDying: q.onMyDying},
	}
}

// onAdvEvent starts the quest, or trades the head for the totem.
func (q q607) onAdvEvent(s *script.Script, e script.Event) string {
	htmltext := e.Name
	st := s.QuestState(e.Player, q607Name)
	if st == nil {
		return htmltext
	}
	switch {
	case strings.EqualFold(e.Name, "31370-04.htm"):
		q.start(s, e.Player, st)
	case strings.EqualFold(e.Name, "31370-07.htm"):
		if !q.trade(s, e.Player, st) {
			htmltext = "31370-06.htm"
		}
	}
	return htmltext
}

func (q q607) onTalk(s *script.Script, e script.Talk) string {
	return q.talk(s, e, "31370-03.htm", "31370-01.htm", "31370-02.htm", "31370-05.htm", "31370-06.htm")
}
