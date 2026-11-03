// Package q024 is the quest Inhabitants of the Forest of the Dead: Dorian
// sends the player with flowers to Lidia's tombstone and with a silver cross
// to meet his night self; Lidia's maid then trades her letter for the
// hairpin, and the totem doll of the forest's undead leads to the Mysterious
// Wizard.
package q024

import (
	"strconv"
	"strings"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/questlog"
	"github.com/fatal10110/acis_golang/internal/gameserver/script"
)

const questName = "Q024_InhabitantsOfTheForestOfTheDead"

// lidiasHeart is the quest Inhabitants of the Forest of the Dead follows.
const lidiasHeart = "Q023_LidiasHeart"

// NPCs.
const (
	nightDorian      = 25332
	dorian           = 31389
	mysteriousWizard = 31522
	tombstone        = 31531
	lidiaMaid        = 31532
)

// Monsters.
const (
	boneSnatcher   = 21557
	boneSnatcherA  = 21558
	boneShaper     = 21560
	boneCollector  = 21563
	skullCollector = 21564
	boneAnimator   = 21565
	skullAnimator  = 21566
	boneSlayer     = 21567
)

// Items.
const (
	lidiasLetter                = 7065
	lidiasHairpin               = 7148
	suspiciousTotemDoll         = 7151
	flowerBouquet               = 7152
	silverCrossOfEinhasad       = 7153
	brokenSilverCrossOfEinhasad = 7154
	suspiciousTotemDoll2        = 7156
)

// sayCrossBreaks is the NpcString night Dorian says as the cross breaks.
const sayCrossBreaks = 2450

// New returns the quest.
func New() script.Script {
	return script.Script{
		Title:   "Inhabitants of the Forest of the Dead",
		QuestID: 24,
		Items:   []int32{lidiasLetter, lidiasHairpin, suspiciousTotemDoll, flowerBouquet, silverCrossOfEinhasad, brokenSilverCrossOfEinhasad},
		Bind: script.Bindings{
			script.EventQuestStart:  {dorian},
			script.EventTalked:      {dorian, mysteriousWizard, lidiaMaid, tombstone},
			script.EventMyDying:     {boneSnatcher, boneSnatcherA, boneShaper, boneCollector, skullCollector, boneAnimator, skullAnimator, boneSlayer},
			script.EventSeeCreature: {nightDorian},
		},
		Hooks: script.Hooks{
			OnEvent: onAdvEvent, OnTalk: onTalk, OnMyDying: onMyDying, OnSeeCreature: onSeeCreature,
		},
	}
}

func setState(st *script.QuestState, state int) { st.Set("state", strconv.Itoa(state)) }

func onAdvEvent(s *script.Script, e script.Event) string {
	htmltext := e.Name
	st := s.QuestState(e.Player, questName)
	if st == nil {
		return htmltext
	}
	p := e.Player
	switch {
	case strings.EqualFold(e.Name, "31389-03.htm"):
		st.SetStatus(questlog.StatusStarted)
		st.SetCond(1)
		setState(st, 1)
		s.PlaySound(p, script.SoundAccept)
		s.GiveItems(p, flowerBouquet, 1)
	case strings.EqualFold(e.Name, "31389-08.htm"):
		setState(st, 3)
	case strings.EqualFold(e.Name, "31389-13.htm"):
		st.SetCond(3)
		setState(st, 4)
		s.PlaySound(p, script.SoundMiddle)
		s.GiveItems(p, silverCrossOfEinhasad, 1)
	case strings.EqualFold(e.Name, "31389-18.htm"):
		s.PlaySound(p, "InterfaceSound.charstat_open_01")
	case strings.EqualFold(e.Name, "31389-19.htm"):
		st.SetCond(5)
		setState(st, 5)
		s.TakeItems(p, brokenSilverCrossOfEinhasad, -1)
		s.PlaySound(p, script.SoundMiddle)
	case strings.EqualFold(e.Name, "31522-03.htm"):
		setState(st, 12)
		s.TakeItems(p, suspiciousTotemDoll, -1)
	case strings.EqualFold(e.Name, "31522-08.htm"):
		st.SetCond(11)
		setState(st, 13)
		s.PlaySound(p, script.SoundMiddle)
	case strings.EqualFold(e.Name, "31522-17.htm"):
		setState(st, 14)
	case strings.EqualFold(e.Name, "31522-21.htm"):
		s.GiveItems(p, suspiciousTotemDoll2, 1)
		s.PlaySound(p, script.SoundFinish)
		st.Exit(false)
	case strings.EqualFold(e.Name, "31532-04.htm"):
		st.SetCond(6)
		setState(st, 6)
		s.GiveItems(p, lidiasLetter, 1)
		s.PlaySound(p, script.SoundMiddle)
	case strings.EqualFold(e.Name, "31532-06.htm"):
		if p.HasItem(lidiasHairpin) {
			setState(st, 8)
			s.TakeItems(p, lidiasLetter, -1)
			s.TakeItems(p, lidiasHairpin, -1)
		} else {
			st.SetCond(7)
			setState(st, 7)
			htmltext = "31532-07.htm"
		}
	case strings.EqualFold(e.Name, "31532-10.htm"):
		setState(st, 9)
	case strings.EqualFold(e.Name, "31532-14.htm"):
		setState(st, 10)
	case strings.EqualFold(e.Name, "31532-19.htm"):
		st.SetCond(9)
		setState(st, 11)
		s.PlaySound(p, script.SoundMiddle)
	case strings.EqualFold(e.Name, "31531-02.htm"):
		st.SetCond(2)
		setState(st, 2)
		s.TakeItems(p, flowerBouquet, -1)
		s.PlaySound(p, script.SoundMiddle)
	}
	return htmltext
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
		if st2 := s.QuestState(p, lidiasHeart); st2 == nil || !st2.IsCompleted() || p.Level() < 65 {
			htmltext = "31389-02.htm"
		} else {
			htmltext = "31389-01.htm"
		}
	case questlog.StatusStarted:
		state := st.GetInt("state")
		switch e.NPC.NpcID() {
		case dorian:
			switch {
			case state == 1:
				htmltext = "31389-04.htm"
			case state == 2:
				htmltext = "31389-05.htm"
			case state == 3:
				htmltext = "31389-09.htm"
			case state == 4:
				if p.HasItem(silverCrossOfEinhasad) {
					htmltext = "31389-14.htm"
				} else if p.HasItem(brokenSilverCrossOfEinhasad) {
					htmltext = "31389-15.htm"
				}
			case state == 5:
				htmltext = "31389-20.htm"
			case state == 7 && !p.HasItem(lidiasHairpin):
				htmltext = "31389-21.htm"
				st.SetCond(8)
				s.GiveItems(p, lidiasHairpin, 1)
				s.PlaySound(p, script.SoundMiddle)
			case (state == 7 && p.HasItem(lidiasHairpin)) || state == 6:
				htmltext = "31389-22.htm"
			}
		case mysteriousWizard:
			switch {
			case state == 11 && p.HasItem(suspiciousTotemDoll):
				htmltext = "31522-01.htm"
			case state == 12:
				htmltext = "31522-04.htm"
			case state == 13:
				htmltext = "31522-09.htm"
			case state == 14:
				htmltext = "31522-18.htm"
			}
		case lidiaMaid:
			switch {
			case state == 5:
				htmltext = "31532-01.htm"
			case state == 6 && p.HasItem(lidiasLetter):
				htmltext = "31532-05.htm"
			case state == 7:
				htmltext = "31532-07a.htm"
			case state == 8:
				htmltext = "31532-08.htm"
			case state == 9:
				htmltext = "31532-11.htm"
			case state == 10:
				htmltext = "31532-15.htm"
			case state == 11:
				htmltext = "31532-20.htm"
			}
		case tombstone:
			switch {
			case state == 1 && p.HasItem(flowerBouquet):
				htmltext = "31531-01.htm"
				s.PlaySound(p, "AmdSound.d_wind_loot_02")
			case state == 2:
				htmltext = "31531-03.htm"
			}
		}
	case questlog.StatusCompleted:
		if e.NPC.NpcID() == mysteriousWizard {
			htmltext = "31522-22.htm"
		} else {
			htmltext = script.AlreadyCompletedMsg()
		}
	}
	return htmltext
}

// onMyDying drops a suspicious totem doll at cond 9.
func onMyDying(s *script.Script, e script.MyDying) {
	p := script.ActingPlayer(e.Killer)
	st := s.CheckPlayerCondition(p, e.NPC, 9)
	if st == nil {
		return
	}
	if s.DropItems(p, suspiciousTotemDoll, 1, 1, 100000) {
		st.SetCond(10)
	}
}

// onSeeCreature breaks the silver cross of a player at cond 3 that night
// Dorian sees.
func onSeeCreature(s *script.Script, e script.SeeCreature) {
	p, ok := e.Creature.(*script.Player)
	if !ok {
		return
	}
	st := s.QuestState(p, questName)
	if st == nil {
		return
	}
	if st.Cond() == 3 {
		st.SetCond(4)
		s.TakeItems(p, silverCrossOfEinhasad, -1)
		s.GiveItems(p, brokenSilverCrossOfEinhasad, 1)
		s.PlaySound(p, script.SoundMiddle)
		e.NPC.Say(script.NpcString(sayCrossBreaks))
	}
}
