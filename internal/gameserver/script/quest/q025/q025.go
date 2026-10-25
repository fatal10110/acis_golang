// Package q025 is the quest Hiding Behind the Truth: Benedict and Agripel
// send the player through the broken bookshelves, whose gem box Triol's pawn
// guards, to Lidia's maid, her tombstone and her coffin, and back to the
// Mysterious Wizard or the maid for the end.
package q025

import (
	"strconv"
	"strings"
	"time"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/questlog"
	"github.com/fatal10110/acis_golang/internal/gameserver/script"
)

const questName = "Q025_HidingBehindTheTruth"

// inhabitants is the quest Hiding Behind the Truth follows.
const inhabitants = "Q024_InhabitantsOfTheForestOfTheDead"

// Items.
const (
	forestOfDeadmanMap   = 7063
	contract             = 7066
	lidiaDress           = 7155
	suspiciousTotemDoll2 = 7156
	gemstoneKey          = 7157
	suspiciousTotemDoll3 = 7158
)

// Rewards.
const (
	earringOfBlessing  = 874
	ringOfBlessing     = 905
	necklaceOfBlessing = 936
)

// NPCs.
const (
	agripel          = 31348
	benedict         = 31349
	mysteriousWizard = 31522
	tombstone        = 31531
	maidOfLidia      = 31532
	brokenBookshelf1 = 31533
	brokenBookshelf2 = 31534
	brokenBookshelf3 = 31535
	coffin           = 31536
)

// Monster.
const triolPawn = 27218

// Sounds.
const (
	soundHorror1 = "SkillSound5.horror_01"
	soundHorror2 = "AmdSound.dd_horror_02"
	soundCry     = "ChrSound.FDElf_Cry"
)

// NpcStrings.
const (
	sayPawnCalled = 2550
	sayPawnFlees  = 2551
)

// triolSpawn is where the broken bookshelf of id bookshelf wakes Triol's
// pawn; false for any other NPC.
func triolSpawn(bookshelf int32) (script.Loc, bool) {
	switch bookshelf {
	case brokenBookshelf1:
		return script.Loc{X: 47142, Y: -35941, Z: -1623, Heading: 0}, true
	case brokenBookshelf2:
		return script.Loc{X: 50055, Y: -47020, Z: -3396, Heading: 0}, true
	case brokenBookshelf3:
		return script.Loc{X: 59712, Y: -47568, Z: -2720, Heading: 0}, true
	}
	return script.Loc{}, false
}

// coffinSpawn is where the tombstone raises the coffin.
func coffinSpawn() script.Loc { return script.Loc{X: 60104, Y: -35820, Z: -681, Heading: 0} }

// New returns the quest.
func New() script.Script {
	return script.Script{
		Title:   "Hiding Behind the Truth",
		QuestID: 25,
		// The map and the second totem doll come from earlier quests and stay.
		Items: []int32{contract, lidiaDress, gemstoneKey, suspiciousTotemDoll3},
		Bind: script.Bindings{
			script.EventQuestStart: {benedict},
			script.EventTalked:     {agripel, benedict, mysteriousWizard, tombstone, maidOfLidia, brokenBookshelf1, brokenBookshelf2, brokenBookshelf3, coffin},
			script.EventFirstTalk:  {maidOfLidia},
			script.EventAttacked:   {triolPawn},
		},
		Hooks: script.Hooks{
			OnEvent: onAdvEvent, OnTalk: onTalk, OnFirstTalk: onFirstTalk, OnAttacked: onAttacked,
		},
	}
}

func setInt(st *script.QuestState, key string, v int32) { st.Set(key, strconv.Itoa(int(v))) }

func onAdvEvent(s *script.Script, e script.Event) string {
	htmltext := e.Name
	st := s.QuestState(e.Player, questName)
	if st == nil {
		return htmltext
	}
	p := e.Player
	switch {
	// Benedict.
	case strings.EqualFold(e.Name, "31349-03.htm"):
		st.SetStatus(questlog.StatusStarted)
		setInt(st, "state", 1)
		st.SetCond(1)
		s.PlaySound(p, script.SoundAccept)
	case strings.EqualFold(e.Name, "31349-04.htm"):
		// The totem doll is lost: the Mysterious Wizard gives it again.
		if !p.HasItem(suspiciousTotemDoll2) {
			htmltext = "31349-05.htm"
			if st.Cond() == 1 {
				st.SetCond(2)
				s.PlaySound(p, script.SoundMiddle)
			}
		}
	case strings.EqualFold(e.Name, "31349-10.htm"):
		setInt(st, "state", 2)
		st.SetCond(4)
		s.PlaySound(p, script.SoundMiddle)
	// Agripel.
	case strings.EqualFold(e.Name, "31348-02.htm"):
		setInt(st, "state", 3)
		s.TakeItems(p, suspiciousTotemDoll2, -1)
	case strings.EqualFold(e.Name, "31348-08.htm"):
		setInt(st, "state", 6)
		st.SetCond(5)
		s.PlaySound(p, script.SoundMiddle)
		s.GiveItems(p, gemstoneKey, 1)
	case strings.EqualFold(e.Name, "31348-10.htm"):
		setInt(st, "state", 21)
		s.TakeItems(p, suspiciousTotemDoll3, -1)
	case strings.EqualFold(e.Name, "31348-13.htm"):
		setInt(st, "state", 22)
	case strings.EqualFold(e.Name, "31348-16.htm"):
		setInt(st, "state", 23)
		st.SetCond(17)
		s.PlaySound(p, script.SoundMiddle)
	case strings.EqualFold(e.Name, "31348-17.htm"):
		setInt(st, "state", 24)
		st.SetCond(18)
		s.PlaySound(p, script.SoundMiddle)
	// Mysterious Wizard.
	case strings.EqualFold(e.Name, "31522-04.htm"):
		setInt(st, "state", 7)
		st.SetCond(6)
		s.PlaySound(p, script.SoundMiddle)
	case strings.EqualFold(e.Name, "31522-10.htm"):
		setInt(st, "state", 19)
	case strings.EqualFold(e.Name, "31522-13.htm"):
		setInt(st, "state", 20)
		st.SetCond(16)
		s.PlaySound(p, script.SoundMiddle)
	case strings.EqualFold(e.Name, "31522-16.htm"):
		s.TakeItems(p, forestOfDeadmanMap, -1)
		s.GiveItems(p, earringOfBlessing, 1)
		s.GiveItems(p, necklaceOfBlessing, 1)
		s.RewardExpAndSp(p, 1607062, 0)
		s.PlaySound(p, script.SoundFinish)
		st.Exit(false)
	// Broken bookshelves.
	case strings.EqualFold(e.Name, "3153x-05.htm"):
		// This bookshelf is open.
		npcID := strconv.Itoa(int(e.NPC.NpcID()))
		setInt(st, npcID, 1)
		if st.GetInt("31533")+st.GetInt("31534")+st.GetInt("31535") == 3 {
			// All are: the last one opened holds the gem box.
			st.Unset("31533")
			st.Unset("31534")
			st.Unset("31535")
			st.Set("bookshelf", npcID)
			setInt(st, "state", 8)
			s.PlaySound(p, soundHorror2)
		} else {
			htmltext = "3153x-03.htm"
		}
	case strings.EqualFold(e.Name, "3153x-07.htm"):
		if !p.HasItem(suspiciousTotemDoll3) {
			if minions := e.NPC.Minions(); len(minions) == 0 {
				// Only a bookshelf wakes the pawn: through any other NPC the
				// event ends here, with nothing shown.
				at, ok := triolSpawn(e.NPC.NpcID())
				if !ok {
					return ""
				}
				pawn := s.Spawner().CreateOnePrivateEx(e.NPC, triolPawn, at, 120000*time.Millisecond, script.Params{})
				pawn.Say(script.NpcString(sayPawnCalled, p.Name()))
				pawn.SetScriptValue(p.ObjectID())

				st.SetCond(7)
				s.PlaySound(p, script.SoundMiddle)
			} else if minions[0].ScriptValue() == p.ObjectID() {
				htmltext = "3153x-08.htm"
			} else {
				htmltext = "3153x-09.htm"
			}
		} else {
			htmltext = "3153x-10.htm"
		}
	case strings.EqualFold(e.Name, "3153x-11.htm"):
		st.Unset("bookshelf")
		setInt(st, "state", 9)
		st.SetCond(9)
		s.PlaySound(p, script.SoundMiddle)
		s.TakeItems(p, gemstoneKey, -1)
		s.GiveItems(p, contract, 1)
	// Maid of Lidia.
	case strings.EqualFold(e.Name, "31532-02.htm"):
		setInt(st, "state", 10)
		s.TakeItems(p, contract, -1)
	case strings.EqualFold(e.Name, "31532-07.htm"):
		setInt(st, "state", 11)
		st.SetCond(11)
		s.PlaySound(p, soundHorror1)
	case strings.EqualFold(e.Name, "31532-12.htm"):
		if sorrow := st.GetInt("sorrow"); sorrow > 0 {
			htmltext = "31532-11.htm"
			setInt(st, "sorrow", sorrow-1)
			s.PlaySound(p, soundCry)
		} else {
			st.Unset("sorrow")
			setInt(st, "state", 14)
		}
	case strings.EqualFold(e.Name, "31532-17.htm"):
		setInt(st, "state", 15)
	case strings.EqualFold(e.Name, "31532-21.htm"):
		setInt(st, "state", 16)
		st.SetCond(15)
		s.PlaySound(p, script.SoundMiddle)
	case strings.EqualFold(e.Name, "31532-25.htm"):
		s.TakeItems(p, forestOfDeadmanMap, -1)
		s.GiveItems(p, earringOfBlessing, 1)
		s.GiveItems(p, ringOfBlessing, 2)
		s.RewardExpAndSp(p, 1607062, 0)
		s.PlaySound(p, script.SoundFinish)
		st.Exit(false)
	// Tombstone.
	case strings.EqualFold(e.Name, "31531-02.htm"):
		st.SetCond(12)
		s.PlaySound(p, script.SoundMiddle)
		if len(e.NPC.Minions()) == 0 {
			s.Spawner().CreateOnePrivateEx(e.NPC, coffin, coffinSpawn(), 20000*time.Millisecond, script.Params{})
		}
	}
	return htmltext
}

// onAttacked gives the third totem doll to the player Triol's pawn was
// called for once its HP falls below 30% at cond 7, and the pawn flees.
func onAttacked(s *script.Script, e script.Attacked) {
	p := script.ActingPlayer(e.Attacker)
	st := s.CheckPlayerCondition(p, e.NPC, 7)
	if st == nil {
		return
	}
	if p.ObjectID() == e.NPC.ScriptValue() && e.NPC.HPRatio() < 0.3 && s.DropItemsAlways(p, suspiciousTotemDoll3, 1, 1) {
		st.SetCond(8)

		e.NPC.Say(script.NpcString(sayPawnFlees))
		e.NPC.DeleteMe()
	}
}

// onFirstTalk frightens a player whose quest left the maid weeping; any
// other player gets her own window.
func onFirstTalk(s *script.Script, e script.FirstTalk) string {
	if st := s.QuestState(e.Player, questName); st != nil && st.GetInt("state") == 11 {
		s.PlaySound(e.Player, soundHorror1)
		return "31532-08.htm"
	}
	e.NPC.ShowChatWindow(e.Player)
	return ""
}

func onTalk(s *script.Script, e script.Talk) string {
	htmltext := script.NoQuestMsg()
	st := s.QuestState(e.Player, questName)
	if st == nil {
		return htmltext
	}
	p := e.Player
	switch st.Status() {
	case questlog.StatusCreated:
		if st2 := s.QuestState(p, inhabitants); st2 != nil && st2.IsCompleted() && p.Level() >= 66 {
			htmltext = "31349-01.htm"
		} else {
			htmltext = "31349-02.htm"
		}
	case questlog.StatusStarted:
		state := st.GetInt("state")
		switch npcID := e.NPC.NpcID(); npcID {
		case benedict:
			switch {
			case state == 1:
				htmltext = "31349-03a.htm"
			case state > 1:
				htmltext = "31349-10.htm"
			}
		case agripel:
			switch {
			case state == 2:
				htmltext = "31348-01.htm"
			case state > 2 && state < 6:
				htmltext = "31348-02.htm"
			case state > 5 && state < 20:
				htmltext = "31348-08a.htm"
			case state == 20:
				htmltext = "31348-09.htm"
			case state == 21:
				htmltext = "31348-10a.htm"
			case state == 22:
				htmltext = "31348-15.htm"
			case state == 23:
				htmltext = "31348-18.htm"
			case state == 24:
				htmltext = "31348-19.htm"
			}
		case mysteriousWizard:
			switch {
			case state == 1:
				if !p.HasItem(suspiciousTotemDoll2) {
					htmltext = "31522-01.htm"
					st.SetCond(3)
					s.PlaySound(p, script.SoundMiddle)
					s.GiveItems(p, suspiciousTotemDoll2, 1)
				} else {
					htmltext = "31522-02.htm"
				}
			case state > 1 && state < 6:
				htmltext = "31522-02.htm"
			case state == 6:
				htmltext = "31522-03.htm"
			case state > 6 && state < 9:
				htmltext = "31522-04.htm"
			case state == 9:
				htmltext = "31522-06.htm"
				if st.Cond() != 10 && p.HasItem(contract) {
					st.SetCond(10)
					s.PlaySound(p, script.SoundMiddle)
				}
			case state > 9 && state < 16:
				htmltext = "31522-06.htm"
			case state == 16:
				htmltext = "31522-06a.htm"
			case state == 19:
				htmltext = "31522-11.htm"
			case state > 19 && state < 23:
				htmltext = "31522-14.htm"
			case state == 23:
				htmltext = "31522-15a.htm"
			case state == 24:
				htmltext = "31522-15.htm"
			}
		case brokenBookshelf1, brokenBookshelf2, brokenBookshelf3:
			switch {
			case state == 7:
				// Searching the bookshelves: has this one been opened?
				if st.GetInt(strconv.Itoa(int(npcID))) == 0 {
					htmltext = "3153x-01.htm"
				} else {
					htmltext = "3153x-03.htm"
				}
			case state == 8:
				// The gem box is found: is it in this bookshelf?
				if st.GetInt("bookshelf") == npcID {
					htmltext = "3153x-05.htm"
				} else {
					htmltext = "3153x-03.htm"
				}
			case state > 8:
				htmltext = "3153x-02.htm"
			}
		case maidOfLidia:
			switch {
			case state == 9:
				htmltext = "31532-01.htm"
			case state > 9 && state < 12:
				htmltext = "31532-03.htm"
			case state == 12:
				htmltext = "31532-09.htm"
				setInt(st, "sorrow", 4)
				setInt(st, "state", 13)
				st.SetCond(14)
				s.PlaySound(p, script.SoundMiddle)
				s.TakeItems(p, lidiaDress, -1)
			case state == 13:
				htmltext = "31532-10.htm"
				s.PlaySound(p, soundCry)
			case state == 14:
				htmltext = "31532-12.htm"
			case state == 15:
				htmltext = "31532-17.htm"
			case state > 15 && state < 23:
				htmltext = "31532-21.htm"
			case state == 23:
				htmltext = "31532-23.htm"
			case state == 24:
				htmltext = "31532-24.htm"
			}
		case tombstone:
			switch {
			case state == 11:
				if len(e.NPC.Minions()) == 0 {
					htmltext = "31531-01.htm"
				} else {
					htmltext = "31531-02.htm"
				}
			case state > 11:
				htmltext = "31531-03.htm"
			}
		case coffin:
			if state == 11 {
				htmltext = "31536-01.htm"
				setInt(st, "state", 12)
				st.SetCond(13)
				s.PlaySound(p, script.SoundMiddle)
				s.GiveItems(p, lidiaDress, 1)

				e.NPC.DeleteMe()
			}
		}
	case questlog.StatusCompleted:
		htmltext = script.AlreadyCompletedMsg()
	}
	return htmltext
}
