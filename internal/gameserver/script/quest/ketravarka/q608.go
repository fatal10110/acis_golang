package ketravarka

import (
	"strings"

	"github.com/fatal10110/acis_golang/internal/gameserver/script"
)

const q608Name = "Q608_SlayTheEnemyCommander"

// mos is the Varka raid boss whose head Q608 asks for.
const mos = 25312

const headOfMos = 7236

// q608 is the quest; the head and the trade are the commander's.
type q608 struct{ commander }

// Q608 is the quest Slay the Enemy Commander: Kadun Zu Ketra asks a level 75
// Ketra ally of alliance level 4 or more for the head of Mos, the Varka raid
// boss, and pays for it with the Totem of Wisdom.
func Q608() script.Script {
	q := q608{commander{tribe: ketra(), name: q608Name, level: 4, head: headOfMos, totem: totemOfWisdom}}
	return script.Script{
		Title:   "Slay the enemy commander!",
		QuestID: 608,
		Items:   []int32{headOfMos},
		Bind: script.Bindings{
			script.EventQuestStart: {kadun},
			script.EventTalked:     {kadun},
			script.EventMyDying:    {mos},
		},
		Hooks: script.Hooks{OnEvent: q.onAdvEvent, OnTalk: q.onTalk, OnMyDying: q.onMyDying},
	}
}

// onAdvEvent starts the quest, or trades the head for the totem.
func (q q608) onAdvEvent(s *script.Script, e script.Event) string {
	htmltext := e.Name
	st := s.QuestState(e.Player, q608Name)
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

func (q q608) onTalk(s *script.Script, e script.Talk) string {
	return q.talk(s, e, "31370-03.htm", "31370-01.htm", "31370-02.htm", "31370-05.htm", "31370-06.htm")
}
