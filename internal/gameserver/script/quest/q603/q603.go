// Package q603 is the quest Daimon the White-Eyed, Part 1: the Eye of
// Argos, for a character of level 73 or more, sends the player to the five
// mysterious tablets for a broken crystal each, then for two hundred evil
// spirit beads from the slaves, and pays an unfinished summon crystal.
package q603

import (
	"strings"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/questlog"
	"github.com/fatal10110/acis_golang/internal/gameserver/script"
)

const questName = "Q603_DaimonTheWhiteEyed_Part1"

// Items.
const (
	evilSpiritBeads         = 7190
	brokenCrystal           = 7191
	unfinishedSummonCrystal = 7192
)

// NPCs.
const (
	eyeOfArgos        = 31683
	mysteriousTablet1 = 31548
	mysteriousTablet2 = 31549
	mysteriousTablet3 = 31550
	mysteriousTablet4 = 31551
	mysteriousTablet5 = 31552
)

// Monsters.
const (
	canyonBandersnatchSlave = 21297
	buffaloSlave            = 21299
	grendelSlave            = 21304
)

// chance returns the monster npcID's drop chance of an evil spirit bead.
func chance(npcID int32) int32 {
	switch npcID {
	case canyonBandersnatchSlave:
		return 500000
	case buffaloSlave:
		return 519000
	case grendelSlave:
		return 673000
	}
	return 0
}

// New returns the quest.
func New() script.Script {
	return script.Script{
		Title:   "Daimon the White-Eyed - Part 1",
		QuestID: 603,
		Items:   []int32{evilSpiritBeads, brokenCrystal},
		Bind: script.Bindings{
			script.EventQuestStart: {eyeOfArgos},
			script.EventTalked: {
				eyeOfArgos, mysteriousTablet1, mysteriousTablet2, mysteriousTablet3,
				mysteriousTablet4, mysteriousTablet5,
			},
			script.EventMyDying: {buffaloSlave, grendelSlave, canyonBandersnatchSlave},
		},
		Hooks: script.Hooks{OnEvent: onAdvEvent, OnTalk: onTalk, OnMyDying: onMyDying},
	}
}

// tablet moves the quest to cond and gives the broken crystal of the
// tablet that was read.
func tablet(s *script.Script, e script.Event, st *script.QuestState, cond int32) {
	st.SetCond(cond)
	s.PlaySound(e.Player, script.SoundMiddle)
	s.GiveItems(e.Player, brokenCrystal, 1)
}

// onAdvEvent starts the quest, reads a tablet, hands the five crystals in,
// or hands the beads in for the unfinished summon crystal; with too few the
// quest goes back to cond 7.
func onAdvEvent(s *script.Script, e script.Event) string {
	htmltext := e.Name
	st := s.QuestState(e.Player, questName)
	if st == nil {
		return htmltext
	}
	switch {
	case strings.EqualFold(e.Name, "31683-03.htm"):
		st.SetStatus(questlog.StatusStarted)
		st.SetCond(1)
		s.PlaySound(e.Player, script.SoundAccept)
	case strings.EqualFold(e.Name, "31683-06.htm"):
		if e.Player.ItemCount(brokenCrystal) > 4 {
			st.SetCond(7)
			s.PlaySound(e.Player, script.SoundMiddle)
			s.TakeItems(e.Player, brokenCrystal, -1)
		} else {
			htmltext = "31683-07.htm"
		}
	case strings.EqualFold(e.Name, "31683-10.htm"):
		if e.Player.ItemCount(evilSpiritBeads) > 199 {
			s.TakeItems(e.Player, evilSpiritBeads, -1)
			s.GiveItems(e.Player, unfinishedSummonCrystal, 1)
			s.PlaySound(e.Player, script.SoundFinish)
			st.Exit(true)
		} else {
			st.SetCond(7)
			htmltext = "31683-11.htm"
		}
	case strings.EqualFold(e.Name, "31548-02.htm"):
		tablet(s, e, st, 2)
	case strings.EqualFold(e.Name, "31549-02.htm"):
		tablet(s, e, st, 3)
	case strings.EqualFold(e.Name, "31550-02.htm"):
		tablet(s, e, st, 4)
	case strings.EqualFold(e.Name, "31551-02.htm"):
		tablet(s, e, st, 5)
	case strings.EqualFold(e.Name, "31552-02.htm"):
		tablet(s, e, st, 6)
	}
	return htmltext
}

func onTalk(s *script.Script, e script.Talk) string {
	htmltext := script.NoQuestMsg()
	st := s.QuestState(e.Player, questName)
	if st == nil {
		return htmltext
	}
	switch st.Status() {
	case questlog.StatusCreated:
		if e.Player.Level() < 73 {
			htmltext = "31683-02.htm"
		} else {
			htmltext = "31683-01.htm"
		}
	case questlog.StatusStarted:
		cond := st.Cond()
		switch e.NPC.NpcID() {
		case eyeOfArgos:
			switch {
			case cond < 6:
				htmltext = "31683-04.htm"
			case cond == 6:
				htmltext = "31683-05.htm"
			case cond == 7:
				htmltext = "31683-08.htm"
			case cond == 8:
				htmltext = "31683-09.htm"
			}
		case mysteriousTablet1:
			if cond == 1 {
				htmltext = "31548-01.htm"
			} else {
				htmltext = "31548-03.htm"
			}
		case mysteriousTablet2:
			if cond == 2 {
				htmltext = "31549-01.htm"
			} else if cond > 2 {
				htmltext = "31549-03.htm"
			}
		case mysteriousTablet3:
			if cond == 3 {
				htmltext = "31550-01.htm"
			} else if cond > 3 {
				htmltext = "31550-03.htm"
			}
		case mysteriousTablet4:
			if cond == 4 {
				htmltext = "31551-01.htm"
			} else if cond > 4 {
				htmltext = "31551-03.htm"
			}
		case mysteriousTablet5:
			if cond == 5 {
				htmltext = "31552-01.htm"
			} else if cond > 5 {
				htmltext = "31552-03.htm"
			}
		}
	}
	return htmltext
}

// onMyDying drops an evil spirit bead to a party member at cond 7, and
// moves the one who holds two hundred to cond 8.
func onMyDying(s *script.Script, e script.MyDying) {
	p := script.ActingPlayer(e.Killer)
	st := s.RandomPartyMember(p, e.NPC, script.ByCond(7))
	if st == nil {
		return
	}
	if s.DropItems(st.Player(), evilSpiritBeads, 1, 200, chance(e.NPC.NpcID())) {
		st.SetCond(8)
	}
}
