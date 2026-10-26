// Package q611 is the quest Alliance with Varka Silenos: Naran Ashanuk
// raises a character's standing with the Varka Silenos one level at a time,
// for Ketra badges and the feathers of the other Ketra quests. Its kill hook
// also serves War with Ketra Orcs, which drops its molars here.
package q611

import (
	"strconv"
	"strings"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/questlog"
	"github.com/fatal10110/acis_golang/internal/gameserver/script"
)

const (
	questName = "Q611_AllianceWithVarkaSilenos"
	// molarQuestName is the quest whose molars drop here.
	molarQuestName = "Q612_WarWithKetraOrcs"
)

// naran is the quest giver, Naran Ashanuk.
const naran = 31378

// Items.
const (
	ketraBadgeSoldier = 7226
	ketraBadgeOfficer = 7227
	ketraBadgeCaptain = 7228

	varkaAlliance1 = 7221
	varkaAlliance2 = 7222
	varkaAlliance3 = 7223
	varkaAlliance4 = 7224
	varkaAlliance5 = 7225

	featherOfValor  = 7229
	featherOfWisdom = 7230

	ketraMolar = 7234
)

// New returns the quest.
func New() script.Script {
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
		Hooks: script.Hooks{OnEvent: onAdvEvent, OnTalk: onTalk, OnMyDying: onMyDying},
	}
}

// chance is the badge drop chance of a Ketra warrior, in millionths.
func chance(npcID int32) int32 {
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
func onAdvEvent(s *script.Script, e script.Event) string {
	htmltext := e.Name
	st := s.QuestState(e.Player, questName)
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
		if p.ItemCount(ketraBadgeSoldier) >= 100 {
			st.SetCond(2)
			s.PlaySound(p, script.SoundMiddle)
			s.TakeItems(p, ketraBadgeSoldier, -1)
			s.GiveItems(p, varkaAlliance1, 1)
			p.SetVarkaKetraAlliance(-1)
		} else {
			htmltext = "31378-03b.htm"
		}
	case strings.EqualFold(e.Name, "31378-10-2.htm"):
		if p.ItemCount(ketraBadgeSoldier) >= 200 && p.ItemCount(ketraBadgeOfficer) >= 100 {
			st.SetCond(3)
			s.PlaySound(p, script.SoundMiddle)
			s.TakeItems(p, ketraBadgeSoldier, -1)
			s.TakeItems(p, ketraBadgeOfficer, -1)
			s.TakeItems(p, varkaAlliance1, -1)
			s.GiveItems(p, varkaAlliance2, 1)
			p.SetVarkaKetraAlliance(-2)
		} else {
			htmltext = "31378-12.htm"
		}
	case strings.EqualFold(e.Name, "31378-10-3.htm"):
		if p.ItemCount(ketraBadgeSoldier) >= 300 && p.ItemCount(ketraBadgeOfficer) >= 200 && p.ItemCount(ketraBadgeCaptain) >= 100 {
			st.SetCond(4)
			s.PlaySound(p, script.SoundMiddle)
			s.TakeItems(p, ketraBadgeSoldier, -1)
			s.TakeItems(p, ketraBadgeOfficer, -1)
			s.TakeItems(p, ketraBadgeCaptain, -1)
			s.TakeItems(p, varkaAlliance2, -1)
			s.GiveItems(p, varkaAlliance3, 1)
			p.SetVarkaKetraAlliance(-3)
		} else {
			htmltext = "31378-15.htm"
		}
	case strings.EqualFold(e.Name, "31378-10-4.htm"):
		if p.ItemCount(ketraBadgeSoldier) >= 300 && p.ItemCount(ketraBadgeOfficer) >= 300 && p.ItemCount(ketraBadgeCaptain) >= 200 && p.ItemCount(featherOfValor) >= 1 {
			st.SetCond(5)
			s.PlaySound(p, script.SoundMiddle)
			s.TakeItems(p, ketraBadgeSoldier, -1)
			s.TakeItems(p, ketraBadgeOfficer, -1)
			s.TakeItems(p, ketraBadgeCaptain, -1)
			s.TakeItems(p, featherOfValor, -1)
			s.TakeItems(p, varkaAlliance3, -1)
			s.GiveItems(p, varkaAlliance4, 1)
			p.SetVarkaKetraAlliance(-4)
		} else {
			htmltext = "31378-21.htm"
		}
	case strings.EqualFold(e.Name, "31378-20.htm"):
		s.TakeItems(p, varkaAlliance1, -1)
		s.TakeItems(p, varkaAlliance2, -1)
		s.TakeItems(p, varkaAlliance3, -1)
		s.TakeItems(p, varkaAlliance4, -1)
		s.TakeItems(p, varkaAlliance5, -1)
		s.TakeItems(p, featherOfValor, -1)
		s.TakeItems(p, featherOfWisdom, -1)
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
			htmltext = "31378-01.htm"
		} else {
			htmltext = "31378-02b.htm"
			st.Exit(true)
			p.SetVarkaKetraAlliance(0)
		}
	case questlog.StatusStarted:
		switch st.Cond() {
		case 1:
			if p.ItemCount(ketraBadgeSoldier) < 100 {
				htmltext = "31378-03b.htm"
			} else {
				htmltext = "31378-09.htm"
			}
		case 2:
			if p.ItemCount(ketraBadgeSoldier) < 200 || p.ItemCount(ketraBadgeOfficer) < 100 {
				htmltext = "31378-12.htm"
			} else {
				htmltext = "31378-13.htm"
			}
		case 3:
			if p.ItemCount(ketraBadgeSoldier) < 300 || p.ItemCount(ketraBadgeOfficer) < 200 || p.ItemCount(ketraBadgeCaptain) < 100 {
				htmltext = "31378-15.htm"
			} else {
				htmltext = "31378-16.htm"
			}
		case 4:
			if p.ItemCount(ketraBadgeSoldier) < 300 || p.ItemCount(ketraBadgeOfficer) < 300 || p.ItemCount(ketraBadgeCaptain) < 200 || !p.HasItem(featherOfValor) {
				htmltext = "31378-21.htm"
			} else {
				htmltext = "31378-22.htm"
			}
		case 5:
			if p.ItemCount(ketraBadgeSoldier) < 400 || p.ItemCount(ketraBadgeOfficer) < 400 || p.ItemCount(ketraBadgeCaptain) < 200 || !p.HasItem(featherOfWisdom) {
				htmltext = "31378-17.htm"
			} else {
				htmltext = "31378-10-5.htm"
				st.SetCond(6)
				s.PlaySound(p, script.SoundMiddle)
				s.TakeItems(p, ketraBadgeSoldier, 400)
				s.TakeItems(p, ketraBadgeOfficer, 400)
				s.TakeItems(p, ketraBadgeCaptain, 200)
				s.TakeItems(p, featherOfWisdom, -1)
				s.TakeItems(p, varkaAlliance4, -1)
				s.GiveItems(p, varkaAlliance5, 1)
				p.SetVarkaKetraAlliance(-5)
			}
		case 6:
			htmltext = "31378-08.htm"
		}
	}
	return htmltext
}

// onMyDying gives a started member of the killer's party a Ketra molar when
// it also holds War with Ketra Orcs, and else a badge for its stage.
func onMyDying(s *script.Script, e script.MyDying) {
	killer := script.ActingPlayer(e.Killer)
	st := s.RandomPartyMemberState(killer, e.NPC, questlog.StatusStarted)
	if st == nil {
		return
	}
	p := st.Player()
	npcID := e.NPC.NpcID()

	if molar := s.QuestState(p, molarQuestName); molar != nil && s.RndBool() {
		if c, ok := molarChance(npcID); ok {
			s.DropItems(p, ketraMolar, 1, 0, c)
			return
		}
	}

	cond := st.Cond()
	if cond == 6 {
		return
	}
	switch npcID {
	case 21324, 21325, 21327, 21328, 21329:
		switch cond {
		case 1:
			s.DropItems(p, ketraBadgeSoldier, 1, 100, chance(npcID))
		case 2:
			s.DropItems(p, ketraBadgeSoldier, 1, 200, chance(npcID))
		case 3, 4:
			s.DropItems(p, ketraBadgeSoldier, 1, 300, chance(npcID))
		case 5:
			s.DropItems(p, ketraBadgeSoldier, 1, 400, chance(npcID))
		}
	case 21331, 21332, 21334, 21335, 21336, 21338, 21343, 21344:
		switch cond {
		case 2:
			s.DropItems(p, ketraBadgeOfficer, 1, 100, chance(npcID))
		case 3:
			s.DropItems(p, ketraBadgeOfficer, 1, 200, chance(npcID))
		case 4:
			s.DropItems(p, ketraBadgeOfficer, 1, 300, chance(npcID))
		case 5:
			s.DropItems(p, ketraBadgeOfficer, 1, 400, chance(npcID))
		}
	case 21339, 21340, 21342, 21345, 21346, 21347, 21348, 21349:
		switch cond {
		case 3:
			s.DropItems(p, ketraBadgeCaptain, 1, 100, chance(npcID))
		case 4, 5:
			s.DropItems(p, ketraBadgeCaptain, 1, 200, chance(npcID))
		}
	}
}
