// Package q226 is the quest Test of the Healer, a second class-change test
// for knights, clerics and oracles: Bandellos sends the traveller to Perrin
// and Tatoma, through Allana, Gupu, Windy and the golden statue, to Sorius
// and Daurin's Leto lizardmen, the mysterious dark elf's three ambushes and
// Kristina's letter, for the Mark of Healer.
package q226

import (
	"strconv"
	"strings"
	"time"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/questlog"
	"github.com/fatal10110/acis_golang/internal/gameserver/script"
	"github.com/fatal10110/acis_golang/internal/gameserver/script/quest/secondclass"
)

const questName = "Q226_TestOfTheHealer"

// Items.
const (
	reportOfPerrin = 2810
	kristinaLetter = 2811
	pictureOfWindy = 2812
	goldenStatue   = 2813
	windyPebbles   = 2814
	orderOfSorius  = 2815
	secretLetter1  = 2816
	secretLetter2  = 2817
	secretLetter3  = 2818
	secretLetter4  = 2819
)

// Rewards.
const markOfHealer = 2820

// The adena Gupu is paid.
const adena = 57

// NPCs.
const (
	bandellos         = 30473
	sorius            = 30327
	allana            = 30424
	perrin            = 30428
	gupu              = 30658
	orphanGirl        = 30659
	windyShaoring     = 30660
	mysteriousDarkelf = 30661
	piperLongbow      = 30662
	sleinShiningBlade = 30663
	kainFlyingKnife   = 30664
	kristina          = 30665
	daurinHammercrush = 30674
)

// Monsters.
const (
	letoLizardmanLeader   = 27123
	letoLizardmanAssassin = 27124
	letoLizardmanSniper   = 27125
	letoLizardmanWizard   = 27126
	letoLizardmanLord     = 27127
	tatoma                = 27134
)

// The classes the test is open to.
const (
	classKnight      = 4
	classCleric      = 15
	classElvenKnight = 19
	classElvenOracle = 29
)

// quest holds the Tatoma and the Leto lizardman leader the dialogs called,
// each shared by every player: one at a time, until it decays.
type quest struct{ tatoma, letoLeader *script.NPCCell }

// New returns the quest.
func New() script.Script {
	q := quest{tatoma: new(script.NPCCell), letoLeader: new(script.NPCCell)}
	return script.Script{
		Title:   "Test of the Healer",
		QuestID: 226,
		Items: []int32{
			reportOfPerrin, kristinaLetter, pictureOfWindy, goldenStatue, windyPebbles, orderOfSorius,
			secretLetter1, secretLetter2, secretLetter3, secretLetter4,
		},
		Bind: script.Bindings{
			script.EventQuestStart: {bandellos},
			script.EventTalked: {
				bandellos, sorius, allana, perrin, gupu, orphanGirl, windyShaoring, mysteriousDarkelf, piperLongbow,
				sleinShiningBlade, kainFlyingKnife, kristina, daurinHammercrush,
			},
			script.EventDecayed: {letoLizardmanLeader, tatoma},
			script.EventMyDying: {
				letoLizardmanLeader, letoLizardmanAssassin, letoLizardmanSniper, letoLizardmanWizard, letoLizardmanLord,
				tatoma,
			},
		},
		Hooks: script.Hooks{OnEvent: q.onAdvEvent, OnTalk: q.onTalk, OnDecayed: q.onDecayed, OnMyDying: onMyDying},
	}
}

// keep holds the NPC a dialog just called in cell; one another player's
// dialog placed meanwhile wins, and the extra one leaves.
func keep(cell *script.NPCCell, n *script.NPC) {
	if n != nil && !cell.SetIfEmpty(n) {
		n.DeleteMe()
	}
}

func (q quest) onAdvEvent(s *script.Script, e script.Event) string {
	htmltext := e.Name
	st := s.QuestState(e.Player, questName)
	if st == nil {
		return htmltext
	}
	p := e.Player
	switch {
	// Bandellos.
	case strings.EqualFold(e.Name, "30473-04.htm"):
		st.SetStatus(questlog.StatusStarted)
		st.SetCond(1)
		s.PlaySound(p, script.SoundAccept)
		s.GiveItems(p, reportOfPerrin, 1)
		if secondclass.GiveDimensionalDiamonds39(s, p) {
			htmltext = "30473-04a.htm"
		}
	case strings.EqualFold(e.Name, "30473-09.htm"):
		s.TakeItems(p, goldenStatue, 1)
		s.GiveItems(p, markOfHealer, 1)
		s.RewardExpAndSp(p, 134839, 50000)
		p.SocialAction(3)
		s.PlaySound(p, script.SoundFinish)
		st.Exit(false)
	// Perrin.
	case strings.EqualFold(e.Name, "30428-02.htm"):
		st.SetCond(2)
		s.PlaySound(p, script.SoundMiddle)
		if q.tatoma.Empty() {
			keep(q.tatoma, s.Spawner().AddSpawn(tatoma, script.Loc{X: -93254, Y: 147559, Z: -2679, Heading: 0}, false,
				200000*time.Millisecond))
		}
	// Gupu.
	case strings.EqualFold(e.Name, "30658-02.htm"):
		if p.Adena() >= 100000 {
			st.SetCond(7)
			s.PlaySound(p, script.SoundMiddle)
			s.TakeItems(p, adena, 100000)
			s.GiveItems(p, pictureOfWindy, 1)
		} else {
			htmltext = "30658-05.htm"
		}
	case strings.EqualFold(e.Name, "30658-03.htm"):
		st.Set("gupu", "1")
	case strings.EqualFold(e.Name, "30658-07.htm"):
		st.SetCond(9)
		s.PlaySound(p, script.SoundMiddle)
	// Windy Shaoring.
	case strings.EqualFold(e.Name, "30660-03.htm"):
		st.SetCond(8)
		s.PlaySound(p, script.SoundMiddle)
		s.TakeItems(p, pictureOfWindy, 1)
		s.GiveItems(p, windyPebbles, 1)
	// Daurin Hammercrush.
	case strings.EqualFold(e.Name, "30674-02.htm"):
		st.SetCond(11)
		s.PlaySound(p, script.SoundBeforeBattle)
		s.TakeItems(p, orderOfSorius, 1)
		if q.letoLeader.Empty() {
			keep(q.letoLeader, s.Spawner().AddSpawn(letoLizardmanLeader,
				script.Loc{X: -97441, Y: 106585, Z: -3405, Heading: 0}, false, 200000*time.Millisecond))
		}
	// Kristina.
	case strings.EqualFold(e.Name, "30665-02.htm"):
		st.SetCond(22)
		s.PlaySound(p, script.SoundMiddle)
		s.TakeItems(p, secretLetter1, 1)
		s.TakeItems(p, secretLetter2, 1)
		s.TakeItems(p, secretLetter3, 1)
		s.TakeItems(p, secretLetter4, 1)
		s.GiveItems(p, kristinaLetter, 1)
	}
	return htmltext
}

func (q quest) onTalk(s *script.Script, e script.Talk) string {
	htmltext := script.NoQuestMsg()
	st := s.QuestState(e.Player, questName)
	if st == nil {
		return htmltext
	}
	p := e.Player
	switch st.Status() {
	case questlog.StatusCreated:
		switch class := p.ClassID(); {
		case class != classKnight && class != classElvenKnight && class != classCleric && class != classElvenOracle:
			htmltext = "30473-01.htm"
		case p.Level() < 39:
			htmltext = "30473-02.htm"
		default:
			htmltext = "30473-03.htm"
		}
	case questlog.StatusStarted:
		htmltext = q.talkStarted(s, e.NPC.NpcID(), p, st)
	case questlog.StatusCompleted:
		htmltext = script.AlreadyCompletedMsg()
	}
	return htmltext
}

// talkStarted is the page npcID gives a started test, with the step it
// takes.
func (q quest) talkStarted(s *script.Script, npcID int32, p *script.Player, st *script.QuestState) string {
	htmltext := script.NoQuestMsg()
	cond := st.Cond()
	switch npcID {
	case bandellos:
		switch {
		case cond < 23:
			htmltext = "30473-05.htm"
		case !p.HasItem(goldenStatue):
			htmltext = "30473-06.htm"
			s.GiveItems(p, markOfHealer, 1)
			s.RewardExpAndSp(p, 118304, 26250)
			p.SocialAction(3)
			s.PlaySound(p, script.SoundFinish)
			st.Exit(false)
		default:
			htmltext = "30473-07.htm"
		}
	case perrin:
		switch {
		case cond < 3:
			htmltext = "30428-01.htm"
		case cond == 3:
			htmltext = "30428-03.htm"
			st.SetCond(4)
			s.PlaySound(p, script.SoundMiddle)
			s.TakeItems(p, reportOfPerrin, 1)
		default:
			htmltext = "30428-04.htm"
		}
	case orphanGirl:
		htmltext = "30659-0" + strconv.Itoa(1+s.Rnd(5)) + ".htm"
	case allana:
		switch {
		case cond == 4:
			htmltext = "30424-01.htm"
			st.SetCond(5)
			s.PlaySound(p, script.SoundMiddle)
		case cond > 4:
			htmltext = "30424-02.htm"
		}
	case gupu:
		switch {
		case st.GetInt("gupu") == 1 && cond != 9:
			htmltext = "30658-07.htm"
			st.SetCond(9)
			s.PlaySound(p, script.SoundMiddle)
		case cond == 5:
			htmltext = "30658-01.htm"
			st.SetCond(6)
			s.PlaySound(p, script.SoundMiddle)
		case cond == 6:
			htmltext = "30658-01.htm"
		case cond == 7:
			htmltext = "30658-04.htm"
		case cond == 8:
			htmltext = "30658-06.htm"
			s.PlaySound(p, script.SoundItemGet)
			s.TakeItems(p, windyPebbles, 1)
			s.GiveItems(p, goldenStatue, 1)
		case cond > 8:
			htmltext = "30658-07.htm"
		}
	case windyShaoring:
		switch {
		case cond == 7:
			htmltext = "30660-01.htm"
		case p.HasItem(windyPebbles):
			htmltext = "30660-04.htm"
		}
	case sorius:
		switch {
		case cond == 9:
			htmltext = "30327-01.htm"
			st.SetCond(10)
			s.PlaySound(p, script.SoundMiddle)
			s.GiveItems(p, orderOfSorius, 1)
		case cond > 9 && cond < 22:
			htmltext = "30327-02.htm"
		case cond == 22:
			htmltext = "30327-03.htm"
			st.SetCond(23)
			s.PlaySound(p, script.SoundMiddle)
			s.TakeItems(p, kristinaLetter, 1)
		case cond == 23:
			htmltext = "30327-04.htm"
		}
	case daurinHammercrush:
		switch {
		case cond == 10:
			htmltext = "30674-01.htm"
		case cond == 11:
			htmltext = "30674-02a.htm"
			// This call passes 200000 where the heading goes, and no despawn time.
			if q.letoLeader.Empty() {
				keep(q.letoLeader, s.Spawner().AddSpawn(letoLizardmanLeader,
					script.Loc{X: -97441, Y: 106585, Z: -3405, Heading: 200000}, false, 0))
			}
		case cond == 12:
			htmltext = "30674-03.htm"
			st.SetCond(13)
			s.PlaySound(p, script.SoundMiddle)
		case cond > 12:
			htmltext = "30674-04.htm"
		}
	case piperLongbow, sleinShiningBlade, kainFlyingKnife:
		switch {
		case cond == 13 || cond == 14:
			htmltext = strconv.Itoa(int(npcID)) + "-01.htm"
		case cond > 14 && cond < 19:
			htmltext = strconv.Itoa(int(npcID)) + "-02.htm"
		case cond > 18 && cond < 22:
			htmltext = strconv.Itoa(int(npcID)) + "-03.htm"
			st.SetCond(21)
			s.PlaySound(p, script.SoundMiddle)
		}
	case mysteriousDarkelf:
		htmltext = talkDarkelf(s, p, st, cond)
	case kristina:
		switch {
		case cond > 18 && cond < 22:
			htmltext = "30665-01.htm"
		case cond > 21:
			htmltext = "30665-04.htm"
		default:
			htmltext = "30665-03.htm"
		}
	}
	return htmltext
}

// talkDarkelf is the mysterious dark elf's page at cond, with the ambush it
// calls around the player: three assassins, three snipers, then two wizards
// and a lord.
func talkDarkelf(s *script.Script, p *script.Player, st *script.QuestState, cond int32) string {
	htmltext := script.NoQuestMsg()
	ambush := func(npcIDs ...int32) {
		for _, id := range npcIDs {
			s.Spawner().AddSpawn(id, p, true, 0)
		}
	}
	switch cond {
	case 13:
		htmltext = "30661-01.htm"
		st.SetCond(14)
		s.PlaySound(p, script.SoundBeforeBattle)
		ambush(letoLizardmanAssassin, letoLizardmanAssassin, letoLizardmanAssassin)
	case 14:
		htmltext = "30661-01.htm"
	case 15:
		htmltext = "30661-02.htm"
		st.SetCond(16)
		s.PlaySound(p, script.SoundBeforeBattle)
		ambush(letoLizardmanSniper, letoLizardmanSniper, letoLizardmanSniper)
	case 16:
		htmltext = "30661-02.htm"
	case 17:
		htmltext = "30661-03.htm"
		st.SetCond(18)
		s.PlaySound(p, script.SoundBeforeBattle)
		ambush(letoLizardmanWizard, letoLizardmanWizard, letoLizardmanLord)
	case 18:
		htmltext = "30661-03.htm"
	case 19:
		htmltext = "30661-04.htm"
		st.SetCond(20)
		s.PlaySound(p, script.SoundMiddle)
	case 20, 21:
		htmltext = "30661-04.htm"
	}
	return htmltext
}

// onDecayed lets the dialogs call a Tatoma or a Leto lizardman leader again
// once the one that stood has left.
func (q quest) onDecayed(_ *script.Script, e script.Decayed) {
	if !q.tatoma.ClearIf(e.NPC) {
		q.letoLeader.ClearIf(e.NPC)
	}
}

func onMyDying(s *script.Script, e script.MyDying) {
	p := script.ActingPlayer(e.Killer)
	st := s.CheckPlayerState(p, e.NPC, questlog.StatusStarted)
	if st == nil {
		return
	}
	cond := st.Cond()
	// kill is the step a monster's death takes from cond or cond+1 to next,
	// with the secret letter it gives (none for 0).
	kill := func(first int32, next int32, letter int32) {
		if cond != first && cond != first+1 {
			return
		}
		st.SetCond(next)
		s.PlaySound(p, script.SoundMiddle)
		if letter != 0 {
			s.GiveItems(p, letter, 1)
		}
	}
	switch e.NPC.NpcID() {
	case tatoma:
		kill(1, 3, 0)
	case letoLizardmanLeader:
		kill(10, 12, secretLetter1)
	case letoLizardmanAssassin:
		kill(13, 15, secretLetter2)
	case letoLizardmanSniper:
		kill(15, 17, secretLetter3)
	case letoLizardmanLord:
		kill(17, 19, secretLetter4)
	}
}
