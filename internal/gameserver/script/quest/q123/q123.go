// Package q123 is the quest The Leader and the Follower: Newyear has a
// clan academy apprentice gather Bruin lizardman blood, choose a clan oath
// armor set that its sponsor pays D-grade crystals for, then hunt legs of
// the Picot araneid beside its sponsor.
package q123

import (
	"strconv"
	"strings"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/questlog"
	"github.com/fatal10110/acis_golang/internal/gameserver/script"
)

const (
	questName = "Q123_TheLeaderAndTheFollower"
	qn2       = "Q118_ToLeadAndBeLed"
)

// newyear starts and ends the quest.
const newyear = 31961

// Monsters.
const (
	bruinLizardman = 27321
	picotAraneid   = 27322
)

// Items.
const (
	bruinLizardmanBlood = 8549
	picotAraneidLeg     = 8550
	crystalD            = 1458
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
		Title:   "The Leader and the Follower",
		QuestID: 123,
		Items:   []int32{bruinLizardmanBlood, picotAraneidLeg},
		Bind: script.Bindings{
			script.EventQuestStart: {newyear},
			script.EventTalked:     {newyear},
			script.EventMyDying:    {bruinLizardman, picotAraneid},
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
	case strings.EqualFold(e.Name, "31961-03.htm"):
		st.SetStatus(questlog.StatusStarted)
		st.SetCond(1)
		st.Set("state", strconv.Itoa(1))
		s.PlaySound(p, script.SoundAccept)
	case strings.EqualFold(e.Name, "31961-05d.htm"):
		chooseArmor(s, st, p, 3, 1)
	case strings.EqualFold(e.Name, "31961-05e.htm"):
		chooseArmor(s, st, p, 4, 2)
	case strings.EqualFold(e.Name, "31961-05f.htm"):
		chooseArmor(s, st, p, 5, 3)
	case strings.EqualFold(e.Name, "31961-10.htm"):
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
				htmltext = "31961-11.htm"
			}
		} else {
			if p.ItemCount(crystalD) > 770 {
				s.TakeItems(p, crystalD, 771)
				st2.SetCond(6)
				st2.Set("state", strconv.Itoa(3))
				s.PlaySound(academic, script.SoundMiddle)
			} else {
				htmltext = "31961-11a.htm"
			}
		}
	}
	return htmltext
}

// chooseArmor moves a quest holding ten bloods to cond, on the armor set
// stateEx, taking every blood.
func chooseArmor(s *script.Script, st *script.QuestState, p *script.Player, cond, stateEx int32) {
	if p.ItemCount(bruinLizardmanBlood) > 9 {
		st.SetCond(cond)
		st.Set("state", strconv.Itoa(2))
		st.Set("stateEx", strconv.Itoa(int(stateEx)))
		s.PlaySound(p, script.SoundMiddle)
		s.TakeItems(p, bruinLizardmanBlood, -1)
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
			if p.ItemCount(bruinLizardmanBlood) < 10 {
				htmltext = "31961-04.htm"
			} else {
				htmltext = "31961-05.htm"
			}
		case 2:
			htmltext = talkArmorChosen(s, st, p, htmltext)
		case 3:
			st.SetCond(7)
			st.Set("state", strconv.Itoa(4))
			s.PlaySound(p, script.SoundMiddle)
			htmltext = "31961-15.htm"
		case 4:
			if p.ItemCount(picotAraneidLeg) > 7 {
				htmltext = "31961-17.htm"
				reward(s, st, p)
			} else {
				htmltext = "31961-16.htm"
			}
		}
	case questlog.StatusCompleted:
		htmltext = script.AlreadyCompletedMsg()
	}
	return htmltext
}

// talkCreated answers a talk to Newyear before the quest starts: an
// apprentice may start it, a sponsor follows its apprentice's quest.
func talkCreated(s *script.Script, p *script.Player, htmltext string) string {
	switch {
	case p.Sponsor() > 0:
		if st2 := s.QuestState(p, qn2); st2 != nil {
			if st2.IsCompleted() {
				htmltext = "31961-02a.htm"
			} else {
				htmltext = "31961-02b.htm"
			}
		} else if p.Level() > 18 {
			htmltext = "31961-01.htm"
		} else {
			htmltext = "31961-02.htm"
		}
	case p.Apprentice() > 0:
		academic := s.ApprenticeInRange(p)
		if academic == nil {
			return "31961-09.htm"
		}
		if st3 := s.QuestState(academic, questName); st3 != nil {
			switch st3.GetInt("state") {
			case 2:
				htmltext = "31961-08.htm"
			case 3:
				htmltext = "31961-12.htm"
			default:
				htmltext = "31961-14.htm"
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
			htmltext = "31961-06a.htm"
		case 2:
			htmltext = "31961-06b.htm"
		case 3:
			htmltext = "31961-06c.htm"
		}
		return htmltext
	}
	if !s.SponsorInRange(p) {
		return "31961-07.htm"
	}
	switch stateEx {
	case 1:
		htmltext = "31961-06.htm"
	case 2:
		htmltext = "31961-06d.htm"
	case 3:
		htmltext = "31961-06e.htm"
	}
	return htmltext
}

// reward trades the legs for the helm and the chosen armor set and ends
// the quest.
func reward(s *script.Script, st *script.QuestState, p *script.Player) {
	s.TakeItems(p, picotAraneidLeg, -1)
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
	case bruinLizardman:
		if st.Cond() == 1 && s.DropItems(p, bruinLizardmanBlood, 1, 10, 700000) {
			st.SetCond(2)
		}
	case picotAraneid:
		if st.Cond() == 7 && s.SponsorInRange(p) && s.DropItems(p, picotAraneidLeg, 1, 8, 700000) {
			st.SetCond(8)
		}
	}
}
