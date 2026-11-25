package ketravarka

import (
	"strconv"
	"strings"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/questlog"
	"github.com/fatal10110/acis_golang/internal/gameserver/script"
)

const q611Name = "Q611_AllianceWithVarkaSilenos"

// q611 is the quest; the raises and the badge drops are the alliance's.
type q611 struct{ alliance }

// Q611 is the quest Alliance with Varka Silenos: Naran Ashanuk raises a
// character's standing with the Varka Silenos one level at a time, for Ketra
// badges and the feathers of the other Varka quests. Its kill hook also drops
// the molars of War with Ketra Orcs.
func Q611() script.Script {
	q := q611{alliance{
		tribe:        varka(),
		badges:       [3]int32{ketraBadgeSoldier, ketraBadgeOfficer, ketraBadgeCaptain},
		valor:        featherOfValor,
		wisdom:       featherOfWisdom,
		soldiers:     []int32{21324, 21325, 21327, 21328, 21329},
		officers:     []int32{21331, 21332, 21334, 21335, 21336, 21338, 21343, 21344},
		captains:     []int32{21339, 21340, 21342, 21345, 21346, 21347, 21348, 21349},
		chance:       ketraBadgeChance,
		trophy:       ketraMolar,
		trophyQuest:  q612Name,
		trophyChance: molarChance,
	}}
	return script.Script{
		Title:   "Alliance with Varka Silenos",
		QuestID: 611,
		Items:   []int32{ketraBadgeSoldier, ketraBadgeOfficer, ketraBadgeCaptain},
		Bind: script.Bindings{
			script.EventQuestStart: {naran},
			script.EventTalked:     {naran},
			script.EventMyDying: {
				21324, 21325, 21327, 21328, 21329, 21331, 21332, 21334, 21335, 21336, 21338,
				21339, 21340, 21342, 21343, 21344, 21345, 21346, 21347, 21348, 21349,
			},
		},
		Hooks: script.Hooks{OnEvent: q.onAdvEvent, OnTalk: q.onTalk, OnMyDying: q.onMyDying},
	}
}

// ketraBadgeChance is the badge drop chance of a Ketra warrior, in
// millionths.
func ketraBadgeChance(npcID int32) int32 {
	switch npcID {
	case 21324, 21325, 21331, 21332, 21339, 21340:
		return 500000
	case 21327, 21334:
		return 509000
	case 21328:
		return 521000
	case 21329:
		return 519000
	case 21335, 21336:
		return 518000
	case 21338:
		return 527000
	case 21342:
		return 508000
	case 21343:
		return 628000
	case 21344, 21346:
		return 604000
	case 21345:
		return 627000
	case 21347:
		return 649000
	case 21348, 21349:
		return 626000
	}
	return 0
}

// molarChance is the molar drop chance of a Ketra warrior in millionths, and
// whether it drops molars at all.
func molarChance(npcID int32) (int32, bool) {
	switch npcID {
	case 21324:
		return 500000, true
	case 21327:
		return 510000, true
	case 21328:
		return 522000, true
	case 21329:
		return 519000, true
	case 21331, 21332:
		return 529000, true
	case 21334:
		return 539000, true
	case 21336:
		return 548000, true
	case 21338:
		return 558000, true
	case 21339, 21340:
		return 568000, true
	case 21342:
		return 578000, true
	case 21343:
		return 664000, true
	case 21345:
		return 713000, true
	case 21347:
		return 738000, true
	}
	return 0, false
}

// onAdvEvent starts the quest at the stage of the alliance mark the player
// holds, trades badges for the next mark, or ends the quest and takes every
// mark.
func (q q611) onAdvEvent(s *script.Script, e script.Event) string {
	htmltext := e.Name
	st := s.QuestState(e.Player, q611Name)
	if st == nil {
		return htmltext
	}
	p := e.Player
	switch {
	case strings.EqualFold(e.Name, "31378-03a.htm"):
		if p.AlliedWithKetra() {
			htmltext = "31378-02a.htm"
			break
		}
		st.SetStatus(questlog.StatusStarted)
		s.PlaySound(p, script.SoundAccept)
		for i := int32(varkaAlliance1); i <= varkaAlliance5; i++ {
			if p.HasItem(i) {
				// The mark i is the alliance level 7220-i, and the stage i-7219.
				st.SetCond(i - 7219)
				p.SetVarkaKetraAlliance(7220 - i)
				return "31378-0" + strconv.Itoa(int(i-7217)) + ".htm"
			}
		}
		st.SetCond(1)
	case strings.EqualFold(e.Name, "31378-10-1.htm"):
		if !q.raise(s, p, st, 1) {
			htmltext = "31378-03b.htm"
		}
	case strings.EqualFold(e.Name, "31378-10-2.htm"):
		if !q.raise(s, p, st, 2) {
			htmltext = "31378-12.htm"
		}
	case strings.EqualFold(e.Name, "31378-10-3.htm"):
		if !q.raise(s, p, st, 3) {
			htmltext = "31378-15.htm"
		}
	case strings.EqualFold(e.Name, "31378-10-4.htm"):
		if !q.raise(s, p, st, 4) {
			htmltext = "31378-21.htm"
		}
	case strings.EqualFold(e.Name, "31378-20.htm"):
		q.quit(s, p, st)
	}
	return htmltext
}

// onTalk offers the quest to a character of level 74 or more, and answers a
// started one by the badges it holds for its stage. At stage 5 holding
// everything, it raises the alliance to its top level.
func (q q611) onTalk(s *script.Script, e script.Talk) string {
	htmltext := script.NoQuestMsg()
	st := s.QuestState(e.Player, q611Name)
	if st == nil {
		return htmltext
	}
	p := e.Player
	switch st.Status() {
	case questlog.StatusCreated:
		if p.Level() >= 74 {
			htmltext = "31378-01.htm"
		} else {
			htmltext = "31378-02b.htm"
			st.Exit(true)
			p.SetVarkaKetraAlliance(0)
		}
	case questlog.StatusStarted:
		switch st.Cond() {
		case 1:
			htmltext = pick(q.ready(p, 1), "31378-09.htm", "31378-03b.htm")
		case 2:
			htmltext = pick(q.ready(p, 2), "31378-13.htm", "31378-12.htm")
		case 3:
			htmltext = pick(q.ready(p, 3), "31378-16.htm", "31378-15.htm")
		case 4:
			htmltext = pick(q.ready(p, 4), "31378-22.htm", "31378-21.htm")
		case 5:
			htmltext = pick(q.raise(s, p, st, 5), "31378-10-5.htm", "31378-17.htm")
		case 6:
			htmltext = "31378-08.htm"
		}
	}
	return htmltext
}
