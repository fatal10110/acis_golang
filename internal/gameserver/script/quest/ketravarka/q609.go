package ketravarka

import (
	"strings"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/questlog"
	"github.com/fatal10110/acis_golang/internal/gameserver/script"
)

const q609Name = "Q609_MagicalPowerOfWater_Part1"

// NPCs of Q609.
const (
	udanBox = 31561
	udanEye = 31684
)

// Items of Q609.
const (
	stolenGreenTotem = 7237
	greenTotem       = 7238
)

// q609 is the quest; the theft and the eye are the theft's.
type q609 struct{ theft }

// Q609 is the quest Magical Power of Water, part 1: Wahkan sends a Ketra ally
// to steal the Green Totem from the Udan Box. A Varka warrior that sees the
// thief at the box calls Udan's Eye, and the theft then fails until Asefa
// lets the thief try again.
func Q609() script.Script {
	q := q609{theft{
		tribe: ketra(), name: q609Name, eye: udanEye, called: 60903, seen: 60904,
		stolen: stolenGreenTotem, totem: greenTotem,
	}}
	return script.Script{
		Title:   "Magical Power of Water - Part 1",
		QuestID: 609,
		Items:   []int32{stolenGreenTotem},
		Bind: script.Bindings{
			script.EventQuestStart: {wahkan},
			script.EventTalked:     {wahkan, asefa, udanBox},
			script.EventMyDying:    {udanEye},
			script.EventSeeCreature: {
				21350, 21351, 21353, 21354, 21355, 21357, 21358, 21360, 21361, 21362, 21369, 21370,
				21364, 21365, 21366, 21368, 21371, 21372, 21373, 21374, 21375,
			},
		},
		Hooks: script.Hooks{OnEvent: q.onAdvEvent, OnTalk: q.onTalk, OnMyDying: q.onMyDying, OnSeeCreature: q.onSeeCreature},
	}
}

// onAdvEvent starts the quest, or steals the totem.
func (q q609) onAdvEvent(s *script.Script, e script.Event) string {
	htmltext := e.Name
	st := s.QuestState(e.Player, q609Name)
	if st == nil {
		return htmltext
	}
	switch {
	case strings.EqualFold(e.Name, "31371-03.htm"):
		q.start(s, e.Player, st)
	case strings.EqualFold(e.Name, "31561-03.htm"):
		htmltext = q.steal(s, e.Player, st, "31561-04.htm", "31561-02.htm", htmltext)
	}
	return htmltext
}

// onTalk offers the quest to a level 74 Ketra ally of level 2 or more.
func (q q609) onTalk(s *script.Script, e script.Talk) string {
	st := s.QuestState(e.Player, q609Name)
	htmltext := script.NoQuestMsg()
	if st == nil {
		return htmltext
	}
	switch st.Status() {
	case questlog.StatusCreated:
		htmltext = pick(q.offered(e.Player), "31371-01.htm", "31371-02.htm")
	case questlog.StatusStarted:
		switch e.NPC.NpcID() {
		case wahkan:
			htmltext = "31371-04.htm"
		case asefa:
			htmltext = q.shaman(s, e.Player, st, "31372-01.htm", "31372-02.htm", "31372-03.htm", "31372-04.htm")
		case udanBox:
			switch st.Cond() {
			case 2:
				htmltext = "31561-01.htm"
			case 3:
				htmltext = "31561-05.htm"
			}
		}
	}
	return htmltext
}
