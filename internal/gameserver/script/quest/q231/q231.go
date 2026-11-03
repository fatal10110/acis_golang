// Package q231 is the quest Test of the Maestro, a second class-change test
// for artisans: Lockirin sends the traveller to Balanki and Croto for the
// soldiers' letter, to Arin and Toma for the repaired teleport device, and
// to Filaur and Lorain for the antidote trophies, for the Mark of Maestro.
package q231

import (
	"strings"
	"time"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/questlog"
	"github.com/fatal10110/acis_golang/internal/gameserver/script"
	"github.com/fatal10110/acis_golang/internal/gameserver/script/quest/secondclass"
)

const questName = "Q231_TestOfTheMaestro"

// Items.
const (
	recommendationOfBalanki = 2864
	recommendationOfFilaur  = 2865
	recommendationOfArin    = 2866
	letterOfSolderDetach    = 2868
	paintOfKamuru           = 2869
	necklaceOfKamuru        = 2870
	paintOfTeleportDevice   = 2871
	teleportDevice          = 2872
	architectureOfKruma     = 2873
	reportOfKruma           = 2874
	ingredientsOfAntidote   = 2875
	stingerWaspNeedle       = 2876
	marshSpiderWeb          = 2877
	bloodOfLeech            = 2878
	brokenTeleportDevice    = 2916
)

// Rewards.
const markOfMaestro = 2867

// NPCs.
const (
	lockirin = 30531
	spiron   = 30532
	balanki  = 30533
	keef     = 30534
	filaur   = 30535
	arin     = 30536
	toma     = 30556
	croto    = 30671
	dubabah  = 30672
	lorain   = 30673
)

// Monsters.
const (
	kingBugbear    = 20150
	giantMistLeech = 20225
	stingerWasp    = 20229
	marshSpider    = 20233
	evilEyeLord    = 27133
)

// The timer that calls the King Bugbears.
const spawnBugbearsID = "spawn_bugbears"

// The class the test is open to.
const classArtisan = 56

// New returns the quest.
func New() script.Script {
	return script.Script{
		Title:   "Test Of The Maestro",
		QuestID: 231,
		Items: []int32{
			recommendationOfBalanki, recommendationOfFilaur, recommendationOfArin, letterOfSolderDetach,
			paintOfKamuru, necklaceOfKamuru, paintOfTeleportDevice, teleportDevice, architectureOfKruma,
			reportOfKruma, ingredientsOfAntidote, stingerWaspNeedle, marshSpiderWeb, bloodOfLeech,
			brokenTeleportDevice,
		},
		Bind: script.Bindings{
			script.EventQuestStart: {lockirin},
			script.EventTalked:     {lockirin, spiron, balanki, keef, filaur, arin, toma, croto, dubabah, lorain},
			script.EventMyDying:    {giantMistLeech, stingerWasp, marshSpider, evilEyeLord},
		},
		Hooks: script.Hooks{OnEvent: onAdvEvent, OnTimer: onTimer, OnTalk: onTalk, OnMyDying: onMyDying},
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
	// Lockirin.
	case strings.EqualFold(e.Name, "30531-04.htm"):
		st.SetStatus(questlog.StatusStarted)
		st.SetCond(1)
		s.PlaySound(p, script.SoundAccept)
		if secondclass.GiveDimensionalDiamonds39(s, p) {
			htmltext = "30531-04a.htm"
		}
	// Balanki.
	case strings.EqualFold(e.Name, "30533-02.htm"):
		st.Set("bCond", "1")
	// Croto.
	case strings.EqualFold(e.Name, "30671-02.htm"):
		s.PlaySound(p, script.SoundItemGet)
		s.GiveItems(p, paintOfKamuru, 1)
	// Toma.
	case strings.EqualFold(e.Name, "30556-05.htm"):
		s.PlaySound(p, script.SoundItemGet)
		s.TakeItems(p, paintOfTeleportDevice, 1)
		s.GiveItems(p, brokenTeleportDevice, 1)
		p.TeleportTo(script.Loc{X: 140352, Y: -194133, Z: -3146}, 0)
		s.StartTimer(spawnBugbearsID, nil, p, 5000*time.Millisecond)
	// Lorain.
	case strings.EqualFold(e.Name, "30673-04.htm"):
		st.Set("fCond", "2")
		s.PlaySound(p, script.SoundItemGet)
		s.TakeItems(p, bloodOfLeech, -1)
		s.TakeItems(p, ingredientsOfAntidote, 1)
		s.TakeItems(p, marshSpiderWeb, -1)
		s.TakeItems(p, stingerWaspNeedle, -1)
		s.GiveItems(p, reportOfKruma, 1)
	}
	return htmltext
}

// onTimer calls the three King Bugbears that wait where Toma's device
// sends the traveller, each set on the traveller.
func onTimer(s *script.Script, e script.Timer) string {
	if strings.EqualFold(e.Name, spawnBugbearsID) {
		for _, at := range [...]script.Loc{
			{X: 140333, Y: -194153, Z: -3138},
			{X: 140395, Y: -194147, Z: -3146},
			{X: 140304, Y: -194082, Z: -3157},
		} {
			if bugbear := s.Spawner().AddSpawn(kingBugbear, at, false, 200000*time.Millisecond); bugbear != nil {
				bugbear.AddAttackDesire(e.Player, 2000)
			}
		}
	}
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
		switch {
		case p.ClassID() != classArtisan:
			htmltext = "30531-01.htm"
		case p.Level() < 39:
			htmltext = "30531-02.htm"
		default:
			htmltext = "30531-03.htm"
		}
	case questlog.StatusStarted:
		htmltext = talkStarted(s, e.NPC.NpcID(), p, st)
	case questlog.StatusCompleted:
		htmltext = script.AlreadyCompletedMsg()
	}
	return htmltext
}

// recommended answers a part just finished: the quest advances once the
// other two recommendations are held (done), else the traveller only hears of
// the item.
func recommended(s *script.Script, p *script.Player, st *script.QuestState, done bool) {
	if done {
		st.SetCond(2)
		s.PlaySound(p, script.SoundMiddle)
	} else {
		s.PlaySound(p, script.SoundItemGet)
	}
}

// talkStarted is the page npcID gives a started test, with the step it
// takes.
func talkStarted(s *script.Script, npcID int32, p *script.Player, st *script.QuestState) string {
	htmltext := script.NoQuestMsg()
	switch npcID {
	case lockirin:
		switch cond := st.Cond(); cond {
		case 1:
			htmltext = "30531-05.htm"
		case 2:
			htmltext = "30531-06.htm"
			s.TakeItems(p, recommendationOfArin, 1)
			s.TakeItems(p, recommendationOfBalanki, 1)
			s.TakeItems(p, recommendationOfFilaur, 1)
			s.GiveItems(p, markOfMaestro, 1)
			s.RewardExpAndSp(p, 46000, 5900)
			p.SocialAction(3)
			s.PlaySound(p, script.SoundFinish)
			st.Exit(false)
		}
	case spiron:
		htmltext = "30532-01.htm"
	case keef:
		htmltext = "30534-01.htm"
	// Part 1.
	case balanki:
		switch bCond := st.GetInt("bCond"); bCond {
		case 0:
			htmltext = "30533-01.htm"
		case 1:
			htmltext = "30533-03.htm"
		case 2:
			htmltext = "30533-04.htm"
			st.Set("bCond", "3")
			s.TakeItems(p, letterOfSolderDetach, 1)
			s.GiveItems(p, recommendationOfBalanki, 1)
			recommended(s, p, st, p.HasItems(recommendationOfArin, recommendationOfFilaur))
		case 3:
			htmltext = "30533-05.htm"
		}
	case croto:
		bCond := st.GetInt("bCond")
		if bCond == 1 {
			switch {
			case !p.HasItem(paintOfKamuru):
				htmltext = "30671-01.htm"
			case !p.HasItem(necklaceOfKamuru):
				htmltext = "30671-03.htm"
			default:
				htmltext = "30671-04.htm"
				st.Set("bCond", "2")
				s.PlaySound(p, script.SoundItemGet)
				s.TakeItems(p, necklaceOfKamuru, 1)
				s.TakeItems(p, paintOfKamuru, 1)
				s.GiveItems(p, letterOfSolderDetach, 1)
			}
		} else if bCond > 1 {
			htmltext = "30671-05.htm"
		}
	case dubabah:
		htmltext = "30672-01.htm"
	// Part 2.
	case arin:
		switch aCond := st.GetInt("aCond"); aCond {
		case 0:
			htmltext = "30536-01.htm"
			st.Set("aCond", "1")
			s.GiveItems(p, paintOfTeleportDevice, 1)
		case 1:
			htmltext = "30536-02.htm"
		case 2:
			htmltext = "30536-03.htm"
			st.Set("aCond", "3")
			s.TakeItems(p, teleportDevice, -1)
			s.GiveItems(p, recommendationOfArin, 1)
			recommended(s, p, st, p.HasItems(recommendationOfBalanki, recommendationOfFilaur))
		case 3:
			htmltext = "30536-04.htm"
		}
	case toma:
		aCond := st.GetInt("aCond")
		if aCond == 1 {
			switch {
			case !p.HasItem(brokenTeleportDevice):
				htmltext = "30556-01.htm"
			case !p.HasItem(teleportDevice):
				htmltext = "30556-06.htm"
				st.Set("aCond", "2")
				s.PlaySound(p, script.SoundItemGet)
				s.TakeItems(p, brokenTeleportDevice, 1)
				s.GiveItems(p, teleportDevice, 5)
			}
		} else if aCond > 1 {
			htmltext = "30556-07.htm"
		}
	// Part 3.
	case filaur:
		switch fCond := st.GetInt("fCond"); fCond {
		case 0:
			htmltext = "30535-01.htm"
			st.Set("fCond", "1")
			s.PlaySound(p, script.SoundItemGet)
			s.GiveItems(p, architectureOfKruma, 1)
		case 1:
			htmltext = "30535-02.htm"
		case 2:
			htmltext = "30535-03.htm"
			st.Set("fCond", "3")
			s.TakeItems(p, reportOfKruma, 1)
			s.GiveItems(p, recommendationOfFilaur, 1)
			recommended(s, p, st, p.HasItems(recommendationOfBalanki, recommendationOfArin))
		case 3:
			htmltext = "30535-04.htm"
		}
	case lorain:
		fCond := st.GetInt("fCond")
		if fCond == 1 {
			if !p.HasItem(reportOfKruma) {
				switch {
				case !p.HasItem(ingredientsOfAntidote):
					htmltext = "30673-01.htm"
					s.PlaySound(p, script.SoundItemGet)
					s.TakeItems(p, architectureOfKruma, 1)
					s.GiveItems(p, ingredientsOfAntidote, 1)
				case p.ItemCount(stingerWaspNeedle) < 10 || p.ItemCount(marshSpiderWeb) < 10 ||
					p.ItemCount(bloodOfLeech) < 10:
					htmltext = "30673-02.htm"
				default:
					htmltext = "30673-03.htm"
				}
			}
		} else if fCond > 1 {
			htmltext = "30673-05.htm"
		}
	}
	return htmltext
}

func onMyDying(s *script.Script, e script.MyDying) {
	p := script.ActingPlayer(e.Killer)
	if s.CheckPlayerCondition(p, e.NPC, 1) == nil {
		return
	}
	switch e.NPC.NpcID() {
	case giantMistLeech:
		if p.HasItem(ingredientsOfAntidote) {
			s.DropItemsAlways(p, bloodOfLeech, 1, 10)
		}
	case stingerWasp:
		if p.HasItem(ingredientsOfAntidote) {
			s.DropItemsAlways(p, stingerWaspNeedle, 1, 10)
		}
	case marshSpider:
		if p.HasItem(ingredientsOfAntidote) {
			s.DropItemsAlways(p, marshSpiderWeb, 1, 10)
		}
	case evilEyeLord:
		if p.HasItem(paintOfKamuru) {
			s.DropItemsAlways(p, necklaceOfKamuru, 1, 1)
		}
	}
}
