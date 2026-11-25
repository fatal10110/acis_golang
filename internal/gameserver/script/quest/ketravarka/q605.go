package ketravarka

import (
	"strconv"
	"strings"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/questlog"
	"github.com/fatal10110/acis_golang/internal/gameserver/script"
)

const q605Name = "Q605_AllianceWithKetraOrcs"

// q605 is the quest; the raises and the badge drops are the alliance's.
type q605 struct{ alliance }

// Q605 is the quest Alliance with Ketra Orcs: Wahkan raises a character's
// standing with the Ketra Orcs one level at a time, for Varka badges and the
// totems of the other Ketra quests. Its kill hook also drops the manes of
// War with Varka Silenos.
func Q605() script.Script {
	q := q605{alliance{
		tribe:        ketra(),
		badges:       [3]int32{varkaBadgeSoldier, varkaBadgeOfficer, varkaBadgeCaptain},
		valor:        totemOfValor,
		wisdom:       totemOfWisdom,
		soldiers:     []int32{21350, 21351, 21353, 21354, 21355},
		officers:     []int32{21357, 21358, 21360, 21361, 21362, 21364, 21369, 21370},
		captains:     []int32{21365, 21366, 21368, 21371, 21372, 21373, 21374, 21375},
		chance:       varkaBadgeChance,
		trophy:       varkaMane,
		trophyQuest:  q606Name,
		trophyChance: maneChance,
	}}
	return script.Script{
		Title:   "Alliance with Ketra Orcs",
		QuestID: 605,
		Items:   []int32{varkaBadgeSoldier, varkaBadgeOfficer, varkaBadgeCaptain},
		Bind: script.Bindings{
			script.EventQuestStart: {wahkan},
			script.EventTalked:     {wahkan},
			script.EventMyDying: {
				21350, 21351, 21353, 21354, 21355, 21357, 21358, 21360, 21361, 21362, 21364,
				21365, 21366, 21368, 21369, 21370, 21371, 21372, 21373, 21374, 21375,
			},
		},
		Hooks: script.Hooks{OnEvent: q.onAdvEvent, OnTalk: q.onTalk, OnMyDying: q.onMyDying},
	}
}

// varkaBadgeChance is the badge drop chance of a Varka warrior, in
// millionths.
func varkaBadgeChance(npcID int32) int32 {
	switch npcID {
	case 21350, 21351, 21357, 21358, 21365, 21366:
		return 500000
	case 21353, 21360:
		return 509000
	case 21354:
		return 521000
	case 21355:
		return 519000
	case 21361, 21362:
		return 518000
	case 21364:
		return 527000
	case 21368:
		return 508000
	case 21369:
		return 628000
	case 21370, 21372:
		return 604000
	case 21371:
		return 627000
	case 21373:
		return 649000
	case 21374, 21375:
		return 626000
	}
	return 0
}

// maneChance is the mane drop chance of a Varka warrior in millionths, and
// whether it drops manes at all.
func maneChance(npcID int32) (int32, bool) {
	switch npcID {
	case 21350:
		return 500000, true
	case 21353:
		return 510000, true
	case 21354:
		return 522000, true
	case 21355:
		return 519000, true
	case 21357, 21358:
		return 529000, true
	case 21360:
		return 539000, true
	case 21362:
		return 548000, true
	case 21364:
		return 558000, true
	case 21365, 21366, 21368:
		return 568000, true
	case 21369:
		return 664000, true
	case 21371:
		return 713000, true
	case 21373:
		return 738000, true
	}
	return 0, false
}

// onAdvEvent starts the quest at the stage of the alliance mark the player
// holds, trades badges for the next mark, or ends the quest and takes every
// mark.
func (q q605) onAdvEvent(s *script.Script, e script.Event) string {
	htmltext := e.Name
	st := s.QuestState(e.Player, q605Name)
	if st == nil {
		return htmltext
	}
	p := e.Player
	switch {
	case strings.EqualFold(e.Name, "31371-03a.htm"):
		if p.AlliedWithVarka() {
			htmltext = "31371-02a.htm"
			break
		}
		st.SetStatus(questlog.StatusStarted)
		s.PlaySound(p, script.SoundAccept)
		for i := int32(ketraAlliance1); i <= ketraAlliance5; i++ {
			if p.HasItem(i) {
				// The mark i is the alliance level i-7210, and the stage i-7209.
				st.SetCond(i - 7209)
				p.SetVarkaKetraAlliance(i - 7210)
				return "31371-0" + strconv.Itoa(int(i-7207)) + ".htm"
			}
		}
		st.SetCond(1)
	case strings.EqualFold(e.Name, "31371-10-1.htm"):
		if !q.raise(s, p, st, 1) {
			htmltext = "31371-03b.htm"
		}
	case strings.EqualFold(e.Name, "31371-10-2.htm"):
		if !q.raise(s, p, st, 2) {
			htmltext = "31371-12.htm"
		}
	case strings.EqualFold(e.Name, "31371-10-3.htm"):
		if !q.raise(s, p, st, 3) {
			htmltext = "31371-15.htm"
		}
	case strings.EqualFold(e.Name, "31371-10-4.htm"):
		if !q.raise(s, p, st, 4) {
			htmltext = "31371-21.htm"
		}
	case strings.EqualFold(e.Name, "31371-20.htm"):
		q.quit(s, p, st)
	}
	return htmltext
}

// onTalk offers the quest to a character of level 74 or more, and answers a
// started one by the badges it holds for its stage. At stage 5 holding
// everything, it raises the alliance to its top level.
func (q q605) onTalk(s *script.Script, e script.Talk) string {
	htmltext := script.NoQuestMsg()
	st := s.QuestState(e.Player, q605Name)
	if st == nil {
		return htmltext
	}
	p := e.Player
	switch st.Status() {
	case questlog.StatusCreated:
		if p.Level() >= 74 {
			htmltext = "31371-01.htm"
		} else {
			htmltext = "31371-02b.htm"
			st.Exit(true)
			p.SetVarkaKetraAlliance(0)
		}
	case questlog.StatusStarted:
		switch st.Cond() {
		case 1:
			htmltext = pick(q.ready(p, 1), "31371-09.htm", "31371-03b.htm")
		case 2:
			htmltext = pick(q.ready(p, 2), "31371-13.htm", "31371-12.htm")
		case 3:
			htmltext = pick(q.ready(p, 3), "31371-16.htm", "31371-15.htm")
		case 4:
			htmltext = pick(q.ready(p, 4), "31371-22.htm", "31371-21.htm")
		case 5:
			htmltext = pick(q.raise(s, p, st, 5), "31371-10-5.htm", "31371-17.htm")
		case 6:
			htmltext = "31371-08.htm"
		}
	}
	return htmltext
}
