// Package q241 is the quest Possessor of a Precious Soul - 1: on a
// subclass of level 50 or more, the player follows Talien's errands across
// the continent, from the legend of the seventeen to Noel's lunargent and
// hellfire oil, and Caradine closes it with a letter for Virgil.
package q241

import (
	"strings"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/questlog"
	"github.com/fatal10110/acis_golang/internal/gameserver/script"
)

const questName = "Q241_PossessorOfAPreciousSoul"

// NPCs.
const (
	talien     = 31739
	gabrielle  = 30753
	gilmore    = 30754
	kantabilon = 31042
	stedmiel   = 30692
	virgil     = 31742
	ogmar      = 31744
	rahorakti  = 31336
	kassandra  = 31743
	caradine   = 31740
	noel       = 31272
)

// Monsters.
const (
	baraham                = 27113
	malrukSuccubus1        = 20244
	malrukSuccubusTuren1   = 20245
	malrukSuccubus2        = 20283
	malrukSuccubusTuren2   = 20284
	splinterStakato        = 21508
	splinterStakatoWalker  = 21509
	splinterStakatoSoldier = 21510
	splinterStakatoDrone1  = 21511
	splinterStakatoDrone2  = 21512
)

// Items.
const (
	legendOfSeventeen  = 7587
	malrukSuccubusClaw = 7597
	echoCrystal        = 7589
	poetryBook         = 7588
	crimsonMoss        = 7598
	rahoraktiMedicine  = 7599
	lunargent          = 6029
	hellfireOil        = 6033
	virgilLetter       = 7677
)

// New returns the quest.
func New() script.Script {
	return script.Script{
		Title:   "Possessor of a Precious Soul - 1",
		QuestID: 241,
		Items:   []int32{legendOfSeventeen, malrukSuccubusClaw, echoCrystal, poetryBook, crimsonMoss, rahoraktiMedicine},
		Bind: script.Bindings{
			script.EventQuestStart: {talien},
			script.EventTalked:     {talien, gabrielle, gilmore, kantabilon, stedmiel, virgil, ogmar, rahorakti, kassandra, caradine, noel},
			script.EventMyDying: {
				baraham, malrukSuccubus1, malrukSuccubus2, malrukSuccubusTuren1, malrukSuccubusTuren2,
				splinterStakato, splinterStakatoWalker, splinterStakatoSoldier, splinterStakatoDrone1, splinterStakatoDrone2,
			},
		},
		Hooks: script.Hooks{OnEvent: onAdvEvent, OnTalk: onTalk, OnMyDying: onMyDying},
	}
}

func onAdvEvent(s *script.Script, e script.Event) string {
	htmltext := e.Name
	p := e.Player
	st := s.QuestState(p, questName)
	if st == nil {
		return htmltext
	}
	switch {
	// Talien.
	case strings.EqualFold(e.Name, "31739-03.htm"):
		st.SetStatus(questlog.StatusStarted)
		st.SetCond(1)
		s.PlaySound(p, script.SoundAccept)
	case strings.EqualFold(e.Name, "31739-07.htm"):
		st.SetCond(5)
		s.PlaySound(p, script.SoundMiddle)
		s.TakeItems(p, legendOfSeventeen, 1)
	case strings.EqualFold(e.Name, "31739-10.htm"):
		st.SetCond(9)
		s.PlaySound(p, script.SoundMiddle)
		s.TakeItems(p, echoCrystal, 1)
	case strings.EqualFold(e.Name, "31739-13.htm"):
		st.SetCond(11)
		s.PlaySound(p, script.SoundMiddle)
		s.TakeItems(p, poetryBook, 1)
	// Gabrielle.
	case strings.EqualFold(e.Name, "30753-02.htm"):
		st.SetCond(2)
		s.PlaySound(p, script.SoundMiddle)
	// Gilmore.
	case strings.EqualFold(e.Name, "30754-02.htm"):
		st.SetCond(3)
		s.PlaySound(p, script.SoundMiddle)
	// Kantabilon.
	case strings.EqualFold(e.Name, "31042-02.htm"):
		st.SetCond(6)
		s.PlaySound(p, script.SoundMiddle)
	case strings.EqualFold(e.Name, "31042-05.htm"):
		st.SetCond(8)
		s.PlaySound(p, script.SoundMiddle)
		s.TakeItems(p, malrukSuccubusClaw, -1)
		s.GiveItems(p, echoCrystal, 1)
	// Stedmiel.
	case strings.EqualFold(e.Name, "30692-02.htm"):
		st.SetCond(10)
		s.PlaySound(p, script.SoundMiddle)
		s.GiveItems(p, poetryBook, 1)
	// Virgil.
	case strings.EqualFold(e.Name, "31742-02.htm"):
		st.SetCond(12)
		s.PlaySound(p, script.SoundMiddle)
	case strings.EqualFold(e.Name, "31742-05.htm"):
		st.SetCond(18)
		s.PlaySound(p, script.SoundMiddle)
	// Ogmar.
	case strings.EqualFold(e.Name, "31744-02.htm"):
		st.SetCond(13)
		s.PlaySound(p, script.SoundMiddle)
	// Rahorakti.
	case strings.EqualFold(e.Name, "31336-02.htm"):
		st.SetCond(14)
		s.PlaySound(p, script.SoundMiddle)
	case strings.EqualFold(e.Name, "31336-05.htm"):
		st.SetCond(16)
		s.PlaySound(p, script.SoundMiddle)
		s.TakeItems(p, crimsonMoss, -1)
		s.GiveItems(p, rahoraktiMedicine, 1)
	// Kassandra.
	case strings.EqualFold(e.Name, "31743-02.htm"):
		st.SetCond(17)
		s.PlaySound(p, script.SoundMiddle)
		s.TakeItems(p, rahoraktiMedicine, 1)
	// Caradine.
	case strings.EqualFold(e.Name, "31740-02.htm"):
		st.SetCond(19)
		s.PlaySound(p, script.SoundMiddle)
	case strings.EqualFold(e.Name, "31740-05.htm"):
		s.GiveItems(p, virgilLetter, 1)
		s.RewardExpAndSp(p, 263043, 0)
		p.SocialAction(3)
		s.PlaySound(p, script.SoundFinish)
		st.Exit(false)
	// Noel.
	case strings.EqualFold(e.Name, "31272-02.htm"):
		st.SetCond(20)
		s.PlaySound(p, script.SoundMiddle)
	case strings.EqualFold(e.Name, "31272-05.htm"):
		if p.HasItem(hellfireOil) && p.ItemCount(lunargent) >= 5 {
			st.SetCond(21)
			s.PlaySound(p, script.SoundMiddle)
			s.TakeItems(p, lunargent, 5)
			s.TakeItems(p, hellfireOil, 1)
		} else {
			htmltext = "31272-07.htm"
		}
	}
	return htmltext
}

func onTalk(s *script.Script, e script.Talk) string {
	htmltext := script.NoQuestMsg()
	p := e.Player
	st := s.QuestState(p, questName)
	if st == nil {
		return htmltext
	}
	switch st.Status() {
	case questlog.StatusCreated:
		if !p.SubClassActive() || p.Level() < 50 {
			htmltext = "31739-02.htm"
		} else {
			htmltext = "31739-01.htm"
		}
	case questlog.StatusStarted:
		if !p.SubClassActive() {
			break
		}
		if page := startedPage(e.NPC.NpcID(), st.Cond(), p); page != "" {
			htmltext = page
		}
	case questlog.StatusCompleted:
		htmltext = script.AlreadyCompletedMsg()
	}
	return htmltext
}

// startedPage is the page npc gives a started subclass player at cond,
// empty when it has none to give.
func startedPage(npc, cond int32, p *script.Player) string {
	switch npc {
	case talien:
		switch cond {
		case 1:
			return "31739-04.htm"
		case 2, 3:
			return "31739-05.htm"
		case 4:
			return "31739-06.htm"
		case 5:
			return "31739-08.htm"
		case 8:
			return "31739-09.htm"
		case 9:
			return "31739-11.htm"
		case 10:
			return "31739-12.htm"
		case 11:
			return "31739-14.htm"
		}
	case gabrielle:
		switch cond {
		case 1:
			return "30753-01.htm"
		case 2:
			return "30753-03.htm"
		}
	case gilmore:
		switch cond {
		case 2:
			return "30754-01.htm"
		case 3:
			return "30754-03.htm"
		}
	case kantabilon:
		switch cond {
		case 5:
			return "31042-01.htm"
		case 6:
			return "31042-03.htm"
		case 7:
			return "31042-04.htm"
		case 8:
			return "31042-06.htm"
		}
	case stedmiel:
		switch cond {
		case 9:
			return "30692-01.htm"
		case 10:
			return "30692-03.htm"
		}
	case virgil:
		switch cond {
		case 11:
			return "31742-01.htm"
		case 12:
			return "31742-03.htm"
		case 17:
			return "31742-04.htm"
		case 18:
			return "31742-06.htm"
		}
	case ogmar:
		switch cond {
		case 12:
			return "31744-01.htm"
		case 13:
			return "31744-03.htm"
		}
	case rahorakti:
		switch cond {
		case 13:
			return "31336-01.htm"
		case 14:
			return "31336-03.htm"
		case 15:
			return "31336-04.htm"
		case 16:
			return "31336-06.htm"
		}
	case kassandra:
		switch cond {
		case 16:
			return "31743-01.htm"
		case 17:
			return "31743-03.htm"
		}
	case caradine:
		switch cond {
		case 18:
			return "31740-01.htm"
		case 19:
			return "31740-03.htm"
		case 21:
			return "31740-04.htm"
		}
	case noel:
		switch cond {
		case 19:
			return "31272-01.htm"
		case 20:
			if p.HasItem(hellfireOil) && p.ItemCount(lunargent) >= 5 {
				return "31272-04.htm"
			}
			return "31272-03.htm"
		case 21:
			return "31272-06.htm"
		}
	}
	return ""
}

func onMyDying(s *script.Script, e script.MyDying) {
	p := script.ActingPlayer(e.Killer)
	st := s.CheckPlayerState(p, e.NPC, questlog.StatusStarted)
	if st == nil || !p.SubClassActive() {
		return
	}
	switch e.NPC.NpcID() {
	case baraham:
		if st.Cond() == 3 {
			st.SetCond(4)
			s.GiveItems(p, legendOfSeventeen, 1)
			s.PlaySound(p, script.SoundMiddle)
		}
	case malrukSuccubus1, malrukSuccubus2:
		if st.Cond() == 6 && s.DropItems(p, malrukSuccubusClaw, 1, 10, 100000) {
			st.SetCond(7)
		}
	case malrukSuccubusTuren1, malrukSuccubusTuren2:
		if st.Cond() == 6 && s.DropItems(p, malrukSuccubusClaw, 1, 10, 120000) {
			st.SetCond(7)
		}
	case splinterStakato, splinterStakatoWalker, splinterStakatoSoldier, splinterStakatoDrone1, splinterStakatoDrone2:
		if st.Cond() == 14 && s.DropItems(p, crimsonMoss, 1, 5, 100000) {
			st.SetCond(15)
		}
	}
}
