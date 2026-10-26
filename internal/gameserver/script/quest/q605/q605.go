// Package q605 is the quest Alliance with Ketra Orcs: Wahkan raises a
// character's standing with the Ketra Orcs one level at a time, for Varka
// badges and the totems of the other Varka quests. Its kill hook also
// serves War with Varka Silenos, which drops its manes here.
package q605

import (
	"strconv"
	"strings"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/questlog"
	"github.com/fatal10110/acis_golang/internal/gameserver/script"
)

const (
	questName = "Q605_AllianceWithKetraOrcs"
	// maneQuestName is the quest whose manes drop here.
	maneQuestName = "Q606_WarWithVarkaSilenos"
)

// wahkan is the quest giver.
const wahkan = 31371

// Items.
const (
	varkaBadgeSoldier = 7216
	varkaBadgeOfficer = 7217
	varkaBadgeCaptain = 7218

	ketraAlliance1 = 7211
	ketraAlliance2 = 7212
	ketraAlliance3 = 7213
	ketraAlliance4 = 7214
	ketraAlliance5 = 7215

	totemOfValor  = 7219
	totemOfWisdom = 7220

	varkaMane = 7233
)

// New returns the quest.
func New() script.Script {
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
		Hooks: script.Hooks{OnEvent: onAdvEvent, OnTalk: onTalk, OnMyDying: onMyDying},
	}
}

// chance is the badge drop chance of a Varka warrior, in millionths.
func chance(npcID int32) int32 {
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
func onAdvEvent(s *script.Script, e script.Event) string {
	htmltext := e.Name
	st := s.QuestState(e.Player, questName)
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
		if p.ItemCount(varkaBadgeSoldier) >= 100 {
			st.SetCond(2)
			s.PlaySound(p, script.SoundMiddle)
			s.TakeItems(p, varkaBadgeSoldier, -1)
			s.GiveItems(p, ketraAlliance1, 1)
			p.SetVarkaKetraAlliance(1)
		} else {
			htmltext = "31371-03b.htm"
		}
	case strings.EqualFold(e.Name, "31371-10-2.htm"):
		if p.ItemCount(varkaBadgeSoldier) >= 200 && p.ItemCount(varkaBadgeOfficer) >= 100 {
			st.SetCond(3)
			s.PlaySound(p, script.SoundMiddle)
			s.TakeItems(p, varkaBadgeSoldier, -1)
			s.TakeItems(p, varkaBadgeOfficer, -1)
			s.TakeItems(p, ketraAlliance1, -1)
			s.GiveItems(p, ketraAlliance2, 1)
			p.SetVarkaKetraAlliance(2)
		} else {
			htmltext = "31371-12.htm"
		}
	case strings.EqualFold(e.Name, "31371-10-3.htm"):
		if p.ItemCount(varkaBadgeSoldier) >= 300 && p.ItemCount(varkaBadgeOfficer) >= 200 && p.ItemCount(varkaBadgeCaptain) >= 100 {
			st.SetCond(4)
			s.PlaySound(p, script.SoundMiddle)
			s.TakeItems(p, varkaBadgeSoldier, -1)
			s.TakeItems(p, varkaBadgeOfficer, -1)
			s.TakeItems(p, varkaBadgeCaptain, -1)
			s.TakeItems(p, ketraAlliance2, -1)
			s.GiveItems(p, ketraAlliance3, 1)
			p.SetVarkaKetraAlliance(3)
		} else {
			htmltext = "31371-15.htm"
		}
	case strings.EqualFold(e.Name, "31371-10-4.htm"):
		if p.ItemCount(varkaBadgeSoldier) >= 300 && p.ItemCount(varkaBadgeOfficer) >= 300 && p.ItemCount(varkaBadgeCaptain) >= 200 && p.ItemCount(totemOfValor) >= 1 {
			st.SetCond(5)
			s.PlaySound(p, script.SoundMiddle)
			s.TakeItems(p, varkaBadgeSoldier, -1)
			s.TakeItems(p, varkaBadgeOfficer, -1)
			s.TakeItems(p, varkaBadgeCaptain, -1)
			s.TakeItems(p, totemOfValor, -1)
			s.TakeItems(p, ketraAlliance3, -1)
			s.GiveItems(p, ketraAlliance4, 1)
			p.SetVarkaKetraAlliance(4)
		} else {
			htmltext = "31371-21.htm"
		}
	case strings.EqualFold(e.Name, "31371-20.htm"):
		s.TakeItems(p, ketraAlliance1, -1)
		s.TakeItems(p, ketraAlliance2, -1)
		s.TakeItems(p, ketraAlliance3, -1)
		s.TakeItems(p, ketraAlliance4, -1)
		s.TakeItems(p, ketraAlliance5, -1)
		s.TakeItems(p, totemOfValor, -1)
		s.TakeItems(p, totemOfWisdom, -1)
		p.SetVarkaKetraAlliance(0)
		st.Exit(true)
	}
	return htmltext
}

// onTalk offers the quest to a character of level 74 or more, and answers a
// started one by the badges it holds for its stage. At stage 5 holding
// everything, it raises the alliance to its top level.
func onTalk(s *script.Script, e script.Talk) string {
	htmltext := script.NoQuestMsg()
	st := s.QuestState(e.Player, questName)
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
			if p.ItemCount(varkaBadgeSoldier) < 100 {
				htmltext = "31371-03b.htm"
			} else {
				htmltext = "31371-09.htm"
			}
		case 2:
			if p.ItemCount(varkaBadgeSoldier) < 200 || p.ItemCount(varkaBadgeOfficer) < 100 {
				htmltext = "31371-12.htm"
			} else {
				htmltext = "31371-13.htm"
			}
		case 3:
			if p.ItemCount(varkaBadgeSoldier) < 300 || p.ItemCount(varkaBadgeOfficer) < 200 || p.ItemCount(varkaBadgeCaptain) < 100 {
				htmltext = "31371-15.htm"
			} else {
				htmltext = "31371-16.htm"
			}
		case 4:
			if p.ItemCount(varkaBadgeSoldier) < 300 || p.ItemCount(varkaBadgeOfficer) < 300 || p.ItemCount(varkaBadgeCaptain) < 200 || !p.HasItem(totemOfValor) {
				htmltext = "31371-21.htm"
			} else {
				htmltext = "31371-22.htm"
			}
		case 5:
			if p.ItemCount(varkaBadgeSoldier) < 400 || p.ItemCount(varkaBadgeOfficer) < 400 || p.ItemCount(varkaBadgeCaptain) < 200 || !p.HasItem(totemOfWisdom) {
				htmltext = "31371-17.htm"
			} else {
				htmltext = "31371-10-5.htm"
				st.SetCond(6)
				s.PlaySound(p, script.SoundMiddle)
				s.TakeItems(p, varkaBadgeSoldier, 400)
				s.TakeItems(p, varkaBadgeOfficer, 400)
				s.TakeItems(p, varkaBadgeCaptain, 200)
				s.TakeItems(p, totemOfWisdom, -1)
				s.TakeItems(p, ketraAlliance4, -1)
				s.GiveItems(p, ketraAlliance5, 1)
				p.SetVarkaKetraAlliance(5)
			}
		case 6:
			htmltext = "31371-08.htm"
		}
	}
	return htmltext
}

// onMyDying gives a started member of the killer's party a Varka mane when
// it also holds War with Varka Silenos, and else a badge for its stage.
func onMyDying(s *script.Script, e script.MyDying) {
	killer := script.ActingPlayer(e.Killer)
	st := s.RandomPartyMemberState(killer, e.NPC, questlog.StatusStarted)
	if st == nil {
		return
	}
	p := st.Player()
	npcID := e.NPC.NpcID()

	if mane := s.QuestState(p, maneQuestName); mane != nil && s.RndBool() {
		if c, ok := maneChance(npcID); ok {
			s.DropItems(p, varkaMane, 1, 0, c)
			return
		}
	}

	cond := st.Cond()
	if cond == 6 {
		return
	}
	switch npcID {
	case 21350, 21351, 21353, 21354, 21355:
		switch cond {
		case 1:
			s.DropItems(p, varkaBadgeSoldier, 1, 100, chance(npcID))
		case 2:
			s.DropItems(p, varkaBadgeSoldier, 1, 200, chance(npcID))
		case 3, 4:
			s.DropItems(p, varkaBadgeSoldier, 1, 300, chance(npcID))
		case 5:
			s.DropItems(p, varkaBadgeSoldier, 1, 400, chance(npcID))
		}
	case 21357, 21358, 21360, 21361, 21362, 21364, 21369, 21370:
		switch cond {
		case 2:
			s.DropItems(p, varkaBadgeOfficer, 1, 100, chance(npcID))
		case 3:
			s.DropItems(p, varkaBadgeOfficer, 1, 200, chance(npcID))
		case 4:
			s.DropItems(p, varkaBadgeOfficer, 1, 300, chance(npcID))
		case 5:
			s.DropItems(p, varkaBadgeOfficer, 1, 400, chance(npcID))
		}
	case 21365, 21366, 21368, 21371, 21372, 21373, 21374, 21375:
		switch cond {
		case 3:
			s.DropItems(p, varkaBadgeCaptain, 1, 100, chance(npcID))
		case 4, 5:
			s.DropItems(p, varkaBadgeCaptain, 1, 200, chance(npcID))
		}
	}
}
