package ketravarka

import (
	"strings"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/questlog"
	"github.com/fatal10110/acis_golang/internal/gameserver/script"
)

const q615Name = "Q615_MagicalPowerOfFire_Part1"

// NPCs of Q615.
const (
	asefaBox = 31559
	asefaEye = 31685
)

// Items of Q615.
const (
	stolenRedTotem = 7242
	redTotem       = 7243
)

// q615 is the quest; the theft and the eye are the theft's.
type q615 struct{ theft }

// Q615 is the quest Magical Power of Fire, part 1: Naran Ashanuk sends a
// Varka ally to steal the Red Totem from the Asefa Box. A Ketra warrior that
// sees the thief at the box calls Asefa's Eye, and the theft then fails until
// Udan lets the thief try again.
func Q615() script.Script {
	q := q615{theft{
		tribe: varka(), name: q615Name, eye: asefaEye, called: 61503, seen: 61504,
		stolen: stolenRedTotem, totem: redTotem,
	}}
	return script.Script{
		Title:   "Magical Power of Fire - Part 1",
		QuestID: 615,
		Items:   []int32{stolenRedTotem},
		Bind: script.Bindings{
			script.EventQuestStart: {naran},
			script.EventTalked:     {naran, udan, asefaBox},
			script.EventMyDying:    {asefaEye},
			script.EventSeeCreature: {
				21350, 21351, 21353, 21354, 21355, 21357, 21358, 21360, 21361, 21362, 21369, 21370,
				21364, 21365, 21366, 21368, 21371, 21372, 21373, 21374, 21375,
			},
		},
		Hooks: script.Hooks{OnEvent: q.onAdvEvent, OnTalk: q.onTalk, OnMyDying: q.onMyDying, OnSeeCreature: q.onSeeCreature},
	}
}

// onAdvEvent starts the quest, or steals the totem.
func (q q615) onAdvEvent(s *script.Script, e script.Event) string {
	htmltext := e.Name
	st := s.QuestState(e.Player, q615Name)
	if st == nil {
		return htmltext
	}
	switch {
	case strings.EqualFold(e.Name, "31378-03.htm"):
		q.start(s, e.Player, st)
	case strings.EqualFold(e.Name, "31559-03.htm"):
		htmltext = q.steal(s, e.Player, st, "31559-04.htm", "31559-02.htm", htmltext)
	}
	return htmltext
}

// onTalk offers the quest to a level 74 Varka ally of level 2 or more.
func (q q615) onTalk(s *script.Script, e script.Talk) string {
	st := s.QuestState(e.Player, q615Name)
	htmltext := script.NoQuestMsg()
	if st == nil {
		return htmltext
	}
	switch st.Status() {
	case questlog.StatusCreated:
		htmltext = pick(q.offered(e.Player), "31378-01.htm", "31378-02.htm")
	case questlog.StatusStarted:
		switch e.NPC.NpcID() {
		case naran:
			htmltext = "31378-04.htm"
		case udan:
			htmltext = q.shaman(s, e.Player, st, "31379-01.htm", "31379-02.htm", "31379-03.htm", "31379-04.htm")
		case asefaBox:
			switch st.Cond() {
			case 2:
				htmltext = "31559-01.htm"
			case 3:
				htmltext = "31559-05.htm"
			}
		}
	}
	return htmltext
}
