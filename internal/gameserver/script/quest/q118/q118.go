// Package q118 is the quest To Lead and Be Led: Pinter has a clan academy
// apprentice gather Maille lizardman blood, choose a clan oath armor set
// that its sponsor pays D-grade crystals for, then hunt legs of the King of
// the Araneid beside its sponsor.
package q118

import (
	"strconv"
	"strings"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/questlog"
	"github.com/fatal10110/acis_golang/internal/gameserver/script"
)

const (
	questName = "Q118_ToLeadAndBeLed"
	qn2       = "Q123_TheLeaderAndTheFollower"
)

// pinter starts and ends the quest.
const pinter = 30298

// Monsters.
const (
	mailleLizardman      = 20919
	mailleLizardmanScout = 20920
	mailleLizardmanGuard = 20921
	kingOfTheAraneid     = 20927
)

// Items.
const (
	bloodOfMailleLizardman = 8062
	legOfKingAraneid       = 8063
	crystalD               = 1458
)

// Rewards.
const (
	clanOathHelm          = 7850
	clanOathArmor         = 7851
	clanOathGauntlets     = 7852
	clanOathSabaton       = 7853
	clanOathBrigandine    = 7854
	clanOathLeatherGloves = 7855
	clanOathBoots         = 7856
	clanOathAketon        = 7857
	clanOathPaddedGloves  = 7858
	clanOathSandals       = 7859
)

// New returns the quest.
func New() script.Script {
	return script.Script{
		Title:   "To Lead and Be Led",
		QuestID: 118,
		Items:   []int32{bloodOfMailleLizardman, legOfKingAraneid},
		Bind: script.Bindings{
			script.EventQuestStart: {pinter},
			script.EventTalked:     {pinter},
			script.EventMyDying:    {mailleLizardman, mailleLizardmanScout, mailleLizardmanGuard, kingOfTheAraneid},
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
	case strings.EqualFold(e.Name, "30298-03.htm"):
		st.SetStatus(questlog.StatusStarted)
		st.SetCond(1)
		st.Set("state", strconv.Itoa(1))
		s.PlaySound(p, script.SoundAccept)
	case strings.EqualFold(e.Name, "30298-05d.htm"):
		chooseArmor(s, st, p, 3, 1)
	case strings.EqualFold(e.Name, "30298-05e.htm"):
		chooseArmor(s, st, p, 4, 2)
	case strings.EqualFold(e.Name, "30298-05f.htm"):
		chooseArmor(s, st, p, 5, 3)
	case strings.EqualFold(e.Name, "30298-10.htm"):
		academic := s.ApprenticeInRange(p)
		if academic == nil {
			break
		}
		st2 := s.QuestState(academic, questName)
		if st2 == nil || st2.GetInt("state") != 2 {
			break
		}
		if st2.GetInt("stateEx") == 1 {
			if p.ItemCount(crystalD) > 921 {
				s.TakeItems(p, crystalD, 922)
				st2.SetCond(6)
				st2.Set("state", strconv.Itoa(3))
				s.PlaySound(academic, script.SoundMiddle)
			} else {
				htmltext = "30298-11.htm"
			}
		} else {
			if p.ItemCount(crystalD) > 770 {
				s.TakeItems(p, crystalD, 771)
				st2.SetCond(6)
				st2.Set("state", strconv.Itoa(3))
				s.PlaySound(academic, script.SoundMiddle)
			} else {
				htmltext = "30298-11a.htm"
			}
		}
	}
	return htmltext
}

// chooseArmor moves a quest holding ten bloods to cond, on the armor set
// stateEx, taking every blood.
func chooseArmor(s *script.Script, st *script.QuestState, p *script.Player, cond, stateEx int32) {
	if p.ItemCount(bloodOfMailleLizardman) > 9 {
		st.SetCond(cond)
		st.Set("state", strconv.Itoa(2))
		st.Set("stateEx", strconv.Itoa(int(stateEx)))
		s.PlaySound(p, script.SoundMiddle)
		s.TakeItems(p, bloodOfMailleLizardman, -1)
	}
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
		htmltext = talkCreated(s, p, htmltext)
	case questlog.StatusStarted:
		switch st.GetInt("state") {
		case 1:
			if p.ItemCount(bloodOfMailleLizardman) < 10 {
				htmltext = "30298-04.htm"
			} else {
				htmltext = "30298-05.htm"
			}
		case 2:
			htmltext = talkArmorChosen(s, st, p, htmltext)
		case 3:
			st.SetCond(7)
			st.Set("state", strconv.Itoa(4))
			s.PlaySound(p, script.SoundMiddle)
			htmltext = "30298-15.htm"
		case 4:
			if p.ItemCount(legOfKingAraneid) > 7 {
				htmltext = "30298-17.htm"
				reward(s, st, p)
			} else {
				htmltext = "30298-16.htm"
			}
		}
	case questlog.StatusCompleted:
		htmltext = script.AlreadyCompletedMsg()
	}
	return htmltext
}

// talkCreated answers a talk to Pinter before the quest starts: an
// apprentice may start it, a sponsor follows its apprentice's quest.
func talkCreated(s *script.Script, p *script.Player, htmltext string) string {
	switch {
	case p.Sponsor() > 0:
		if st2 := s.QuestState(p, qn2); st2 != nil {
			if st2.IsCompleted() {
				htmltext = "30298-02a.htm"
			} else {
				htmltext = "30298-02b.htm"
			}
		} else if p.Level() > 18 {
			htmltext = "30298-01.htm"
		} else {
			htmltext = "30298-02.htm"
		}
	case p.Apprentice() > 0:
		academic := s.ApprenticeInRange(p)
		if academic == nil {
			return "30298-09.htm"
		}
		if st3 := s.QuestState(academic, questName); st3 != nil {
			switch st3.GetInt("state") {
			case 2:
				htmltext = "30298-08.htm"
			case 3:
				htmltext = "30298-12.htm"
			default:
				htmltext = "30298-14.htm"
			}
		}
	}
	return htmltext
}

// talkArmorChosen answers a talk once the armor set is chosen, waiting
// for the sponsor to pay.
func talkArmorChosen(s *script.Script, st *script.QuestState, p *script.Player, htmltext string) string {
	stateEx := st.GetInt("stateEx")
	if p.Sponsor() == 0 {
		switch stateEx {
		case 1:
			htmltext = "30298-06a.htm"
		case 2:
			htmltext = "30298-06b.htm"
		case 3:
			htmltext = "30298-06c.htm"
		}
		return htmltext
	}
	if !s.SponsorInRange(p) {
		return "30298-07.htm"
	}
	switch stateEx {
	case 1:
		htmltext = "30298-06.htm"
	case 2:
		htmltext = "30298-06d.htm"
	case 3:
		htmltext = "30298-06e.htm"
	}
	return htmltext
}

// reward trades the legs for the helm and the chosen armor set and ends
// the quest.
func reward(s *script.Script, st *script.QuestState, p *script.Player) {
	s.TakeItems(p, legOfKingAraneid, -1)
	s.GiveItems(p, clanOathHelm, 1)
	switch st.GetInt("stateEx") {
	case 1:
		s.GiveItems(p, clanOathArmor, 1)
		s.GiveItems(p, clanOathGauntlets, 1)
		s.GiveItems(p, clanOathSabaton, 1)
	case 2:
		s.GiveItems(p, clanOathBrigandine, 1)
		s.GiveItems(p, clanOathLeatherGloves, 1)
		s.GiveItems(p, clanOathBoots, 1)
	case 3:
		s.GiveItems(p, clanOathAketon, 1)
		s.GiveItems(p, clanOathPaddedGloves, 1)
		s.GiveItems(p, clanOathSandals, 1)
	}
	s.PlaySound(p, script.SoundFinish)
	st.Exit(false)
}

func onMyDying(s *script.Script, e script.MyDying) {
	p := script.ActingPlayer(e.Killer)
	st := s.CheckPlayerState(p, e.NPC, questlog.StatusStarted)
	if st == nil {
		return
	}
	if p.Sponsor() == 0 {
		st.Exit(true)
		return
	}
	switch e.NPC.NpcID() {
	case mailleLizardman, mailleLizardmanScout, mailleLizardmanGuard:
		if st.Cond() == 1 && s.DropItems(p, bloodOfMailleLizardman, 1, 10, 700000) {
			st.SetCond(2)
		}
	case kingOfTheAraneid:
		if st.Cond() == 7 && s.SponsorInRange(p) && s.DropItems(p, legOfKingAraneid, 1, 8, 700000) {
			st.SetCond(8)
		}
	}
}
