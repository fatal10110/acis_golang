// Package q213 is the quest Trial of the Seeker, a second class-change
// trial: Dufner sends a rogue to Terry, who has it gather runestones and
// takes it through Viktor, Marina and Brunon before the Mark of Seeker.
package q213

import (
	"strings"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/questlog"
	"github.com/fatal10110/acis_golang/internal/gameserver/script"
	"github.com/fatal10110/acis_golang/internal/gameserver/script/quest/secondclass"
)

const questName = "Q213_TrialOfTheSeeker"

// Items.
const (
	dufnerLetter          = 2647
	terryOrder1           = 2648
	terryOrder2           = 2649
	terryLetter           = 2650
	viktorLetter          = 2651
	hawkeyeLetter         = 2652
	mysteriousRunestone   = 2653
	olMahumRunestone      = 2654
	turekRunestone        = 2655
	antRunestone          = 2656
	turakBugbearRunestone = 2657
	terryBox              = 2658
	viktorRequest         = 2659
	medusaScales          = 2660
	shilenRunestone       = 2661
	analysisRequest       = 2662
	marinaLetter          = 2663
	experimentTools       = 2664
	analysisResult        = 2665
	terryOrder3           = 2666
	listOfHost            = 2667
	abyssRunestone1       = 2668
	abyssRunestone2       = 2669
	abyssRunestone3       = 2670
	abyssRunestone4       = 2671
	terryReport           = 2672
)

// Rewards.
const markOfSeeker = 2673

// NPCs.
const (
	terry  = 30064
	dufner = 30106
	brunon = 30526
	viktor = 30684
	marina = 30715
)

// Monsters.
const (
	neerGhoulBerserker   = 20198
	antCaptain           = 20080
	olMahumCaptain       = 20211
	turakBugbearWarrior  = 20249
	turekOrcWarlord      = 20495
	medusa               = 20158
	antWarriorCaptain    = 20088
	marshStakatoDrone    = 20234
	brekaOrcOverlord     = 20270
	letoLizardmanWarrior = 20580
)

// The classes the trial is open to.
const (
	classRogue      = 7
	classElvenScout = 22
	classAssassin   = 35
)

// New returns the quest.
func New() script.Script {
	return script.Script{
		Title:   "Trial of the Seeker",
		QuestID: 213,
		Items: []int32{
			dufnerLetter, terryOrder1, terryOrder2, terryLetter, viktorLetter, hawkeyeLetter, mysteriousRunestone,
			olMahumRunestone, turekRunestone, antRunestone, turakBugbearRunestone, terryBox, viktorRequest, medusaScales,
			shilenRunestone, analysisRequest, marinaLetter, experimentTools, analysisResult, terryOrder3, listOfHost,
			abyssRunestone1, abyssRunestone2, abyssRunestone3, abyssRunestone4, terryReport,
		},
		Bind: script.Bindings{
			script.EventQuestStart: {dufner},
			script.EventTalked:     {terry, dufner, brunon, viktor, marina},
			script.EventMyDying: {
				neerGhoulBerserker, antCaptain, olMahumCaptain, turakBugbearWarrior, turekOrcWarlord,
				antWarriorCaptain, marshStakatoDrone, brekaOrcOverlord, letoLizardmanWarrior, medusa,
			},
		},
		Hooks: script.Hooks{OnEvent: onAdvEvent, OnTalk: onTalk, OnMyDying: onMyDying},
	}
}

func onAdvEvent(s *script.Script, e script.Event) string {
	htmltext := e.Name
	st := s.QuestState(e.Player, questName)
	if st == nil {
		return htmltext
	}
	p := e.Player
	switch {
	// Dufner.
	case strings.EqualFold(e.Name, "30106-05.htm"):
		st.SetStatus(questlog.StatusStarted)
		st.SetCond(1)
		s.PlaySound(p, script.SoundAccept)
		s.GiveItems(p, dufnerLetter, 1)
		if secondclass.GiveDimensionalDiamonds35(s, p) {
			htmltext = "30106-05a.htm"
		}
	// Terry.
	case strings.EqualFold(e.Name, "30064-03.htm"):
		st.SetCond(2)
		s.PlaySound(p, script.SoundMiddle)
		s.TakeItems(p, dufnerLetter, 1)
		s.GiveItems(p, terryOrder1, 1)
	case strings.EqualFold(e.Name, "30064-06.htm"):
		st.SetCond(4)
		s.PlaySound(p, script.SoundMiddle)
		s.TakeItems(p, mysteriousRunestone, 1)
		s.TakeItems(p, terryOrder1, 1)
		s.GiveItems(p, terryOrder2, 1)
	case strings.EqualFold(e.Name, "30064-10.htm"):
		st.SetCond(6)
		s.PlaySound(p, script.SoundMiddle)
		s.TakeItems(p, antRunestone, 1)
		s.TakeItems(p, olMahumRunestone, 1)
		s.TakeItems(p, turakBugbearRunestone, 1)
		s.TakeItems(p, turekRunestone, 1)
		s.TakeItems(p, terryOrder2, 1)
		s.GiveItems(p, terryBox, 1)
		s.GiveItems(p, terryLetter, 1)
	case strings.EqualFold(e.Name, "30064-18.htm"):
		if p.Level() < 36 {
			htmltext = "30064-17.htm"
			s.PlaySound(p, script.SoundItemGet)
			s.TakeItems(p, analysisResult, 1)
			s.GiveItems(p, terryOrder3, 1)
		} else {
			st.SetCond(16)
			s.PlaySound(p, script.SoundMiddle)
			s.TakeItems(p, analysisResult, 1)
			s.GiveItems(p, listOfHost, 1)
		}
	// Viktor.
	case strings.EqualFold(e.Name, "30684-05.htm"):
		st.SetCond(7)
		s.PlaySound(p, script.SoundMiddle)
		s.TakeItems(p, terryLetter, 1)
		s.GiveItems(p, viktorLetter, 1)
	case strings.EqualFold(e.Name, "30684-11.htm"):
		st.SetCond(9)
		s.PlaySound(p, script.SoundMiddle)
		s.TakeItems(p, terryLetter, 1)
		s.TakeItems(p, terryBox, 1)
		s.TakeItems(p, hawkeyeLetter, 1)
		s.TakeItems(p, viktorLetter, 1)
		s.GiveItems(p, viktorRequest, 1)
	case strings.EqualFold(e.Name, "30684-15.htm"):
		st.SetCond(11)
		s.PlaySound(p, script.SoundMiddle)
		s.TakeItems(p, viktorRequest, 1)
		s.TakeItems(p, medusaScales, 10)
		s.GiveItems(p, analysisRequest, 1)
		s.GiveItems(p, shilenRunestone, 1)
	// Marina.
	case strings.EqualFold(e.Name, "30715-02.htm"):
		st.SetCond(12)
		s.PlaySound(p, script.SoundMiddle)
		s.TakeItems(p, shilenRunestone, 1)
		s.TakeItems(p, analysisRequest, 1)
		s.GiveItems(p, marinaLetter, 1)
	case strings.EqualFold(e.Name, "30715-05.htm"):
		st.SetCond(14)
		s.PlaySound(p, script.SoundMiddle)
		s.TakeItems(p, experimentTools, 1)
		s.GiveItems(p, analysisResult, 1)
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
		if class := p.ClassID(); class == classRogue || class == classElvenScout || class == classAssassin {
			if p.Level() < 35 {
				htmltext = "30106-02.htm"
			} else {
				htmltext = "30106-03.htm"
			}
		} else {
			htmltext = "30106-00.htm"
		}
	case questlog.StatusStarted:
		cond := st.Cond()
		switch e.NPC.NpcID() {
		case dufner:
			switch {
			case cond == 1:
				htmltext = "30106-06.htm"
			case cond > 1:
				if !p.HasItem(terryReport) {
					htmltext = "30106-07.htm"
				} else {
					htmltext = "30106-08.htm"
					s.TakeItems(p, terryReport, 1)
					s.GiveItems(p, markOfSeeker, 1)
					s.RewardExpAndSp(p, 72126, 11000)
					p.SocialAction(3)
					s.PlaySound(p, script.SoundFinish)
					st.Exit(false)
				}
			}
		case terry:
			switch {
			case cond == 1:
				htmltext = "30064-01.htm"
			case cond == 2:
				htmltext = "30064-04.htm"
			case cond == 3:
				htmltext = "30064-05.htm"
			case cond == 4:
				htmltext = "30064-08.htm"
			case cond == 5:
				htmltext = "30064-09.htm"
			case cond == 6:
				htmltext = "30064-11.htm"
			case cond == 7:
				htmltext = "30064-12.htm"
				st.SetCond(8)
				s.PlaySound(p, script.SoundMiddle)
				s.TakeItems(p, viktorLetter, 1)
				s.GiveItems(p, hawkeyeLetter, 1)
			case cond == 8:
				htmltext = "30064-13.htm"
			case cond > 8 && cond < 14:
				htmltext = "30064-14.htm"
			case cond == 14:
				switch {
				case !p.HasItem(terryOrder3):
					htmltext = "30064-15.htm"
				case p.Level() < 36:
					htmltext = "30064-20.htm"
				default:
					htmltext = "30064-21.htm"
					st.SetCond(15)
					s.PlaySound(p, script.SoundMiddle)
					s.TakeItems(p, terryOrder3, 1)
					s.GiveItems(p, listOfHost, 1)
				}
			case cond == 15 || cond == 16:
				htmltext = "30064-22.htm"
			case cond == 17:
				if !p.HasItem(terryReport) {
					htmltext = "30064-23.htm"
					s.PlaySound(p, script.SoundMiddle)
					s.TakeItems(p, listOfHost, 1)
					s.TakeItems(p, abyssRunestone1, 1)
					s.TakeItems(p, abyssRunestone2, 1)
					s.TakeItems(p, abyssRunestone3, 1)
					s.TakeItems(p, abyssRunestone4, 1)
					s.GiveItems(p, terryReport, 1)
				} else {
					htmltext = "30064-24.htm"
				}
			}
		case viktor:
			switch {
			case cond == 6:
				htmltext = "30684-01.htm"
			case cond == 7:
				htmltext = "30684-05.htm"
			case cond == 8:
				htmltext = "30684-12.htm"
			case cond == 9:
				htmltext = "30684-13.htm"
			case cond == 10:
				htmltext = "30684-14.htm"
			case cond == 11:
				htmltext = "30684-16.htm"
			case cond > 11:
				htmltext = "30684-17.htm"
			}
		case marina:
			switch {
			case cond == 11:
				htmltext = "30715-01.htm"
			case cond == 12:
				htmltext = "30715-03.htm"
			case cond == 13:
				htmltext = "30715-04.htm"
			case p.HasItem(analysisResult):
				htmltext = "30715-06.htm"
			}
		case brunon:
			switch cond {
			case 12:
				htmltext = "30526-01.htm"
				st.SetCond(13)
				s.PlaySound(p, script.SoundMiddle)
				s.TakeItems(p, marinaLetter, 1)
				s.GiveItems(p, experimentTools, 1)
			case 13:
				htmltext = "30526-02.htm"
			}
		}
	case questlog.StatusCompleted:
		htmltext = script.AlreadyCompletedMsg()
	}
	return htmltext
}

func onMyDying(s *script.Script, e script.MyDying) {
	p := script.ActingPlayer(e.Killer)
	st := s.CheckPlayerState(p, e.NPC, questlog.StatusStarted)
	if st == nil {
		return
	}
	cond := st.Cond()
	switch e.NPC.NpcID() {
	case neerGhoulBerserker:
		if cond == 2 && s.DropItems(p, mysteriousRunestone, 1, 1, 100000) {
			st.SetCond(3)
		}
	case antCaptain:
		if cond == 4 && s.DropItems(p, antRunestone, 1, 1, 250000) && p.HasItems(olMahumRunestone, turakBugbearRunestone, turekRunestone) {
			st.SetCond(5)
		}
	case olMahumCaptain:
		if cond == 4 && s.DropItems(p, olMahumRunestone, 1, 1, 250000) && p.HasItems(antRunestone, turakBugbearRunestone, turekRunestone) {
			st.SetCond(5)
		}
	case turakBugbearWarrior:
		if cond == 4 && s.DropItems(p, turakBugbearRunestone, 1, 1, 250000) && p.HasItems(antRunestone, olMahumRunestone, turekRunestone) {
			st.SetCond(5)
		}
	case turekOrcWarlord:
		if cond == 4 && s.DropItems(p, turekRunestone, 1, 1, 250000) && p.HasItems(antRunestone, olMahumRunestone, turakBugbearRunestone) {
			st.SetCond(5)
		}
	case medusa:
		if cond == 9 && s.DropItems(p, medusaScales, 1, 10, 300000) {
			st.SetCond(10)
		}
	case marshStakatoDrone:
		if (cond == 15 || cond == 16) && s.DropItems(p, abyssRunestone1, 1, 1, 250000) && p.HasItems(abyssRunestone2, abyssRunestone3, abyssRunestone4) {
			st.SetCond(17)
		}
	case brekaOrcOverlord:
		if (cond == 15 || cond == 16) && s.DropItems(p, abyssRunestone2, 1, 1, 250000) && p.HasItems(abyssRunestone1, abyssRunestone3, abyssRunestone4) {
			st.SetCond(17)
		}
	case antWarriorCaptain:
		if (cond == 15 || cond == 16) && s.DropItems(p, abyssRunestone3, 1, 1, 250000) && p.HasItems(abyssRunestone1, abyssRunestone2, abyssRunestone4) {
			st.SetCond(17)
		}
	case letoLizardmanWarrior:
		if (cond == 15 || cond == 16) && s.DropItems(p, abyssRunestone4, 1, 1, 250000) && p.HasItems(abyssRunestone1, abyssRunestone2, abyssRunestone3) {
			st.SetCond(17)
		}
	}
}
