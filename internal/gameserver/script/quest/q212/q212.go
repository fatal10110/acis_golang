// Package q212 is the quest Trial of Duty, a second class-change trial:
// Hannavalt sends a knight through the trials of Sir Aron, Sir Kiel,
// Silvershadow, Dustin and Sir Collin for the Mark of Duty.
package q212

import (
	"strings"
	"time"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/questlog"
	"github.com/fatal10110/acis_golang/internal/gameserver/script"
	"github.com/fatal10110/acis_golang/internal/gameserver/script/quest/secondclass"
)

const questName = "Q212_TrialOfDuty"

// Items.
const (
	letterOfDustin    = 2634
	knightsTear       = 2635
	mirrorOfOrpic     = 2636
	tearOfConfession  = 2637
	reportPiece1      = 2638
	reportPiece2      = 2639
	tearOfLoyalty     = 2640
	militasArticle    = 2641
	saintsAshesUrn    = 2642
	athebaldtSkull    = 2643
	athebaldtRibs     = 2644
	athebaldtShin     = 2645
	letterOfWindawood = 2646
	oldKnightSword    = 3027
)

// Rewards.
const markOfDuty = 2633

// NPCs.
const (
	hannavalt      = 30109
	dustin         = 30116
	sirCollin      = 30311
	sirAron        = 30653
	sirKiel        = 30654
	silvershadow   = 30655
	spiritTalianus = 30656
)

// Monsters.
const (
	hangmanTree           = 20144
	skeletonMarauder      = 20190
	skeletonRaider        = 20191
	strain                = 20200
	ghoul                 = 20201
	brekaOrcOverlord      = 20270
	spiritOfSirHerod      = 27119
	letoLizardman         = 20577
	letoLizardmanArcher   = 20578
	letoLizardmanSoldier  = 20579
	letoLizardmanWarrior  = 20580
	letoLizardmanShaman   = 20581
	letoLizardmanOverlord = 20582
)

// The classes the trial is open to.
const (
	classKnight      = 4
	classElvenKnight = 19
	classPalusKnight = 32
)

// New returns the quest.
func New() script.Script {
	return script.Script{
		Title:   "Trial of Duty",
		QuestID: 212,
		Items: []int32{
			letterOfDustin, knightsTear, mirrorOfOrpic, tearOfConfession, reportPiece1, reportPiece2, tearOfLoyalty,
			militasArticle, saintsAshesUrn, athebaldtSkull, athebaldtRibs, athebaldtShin, letterOfWindawood, oldKnightSword,
		},
		Bind: script.Bindings{
			script.EventQuestStart: {hannavalt},
			script.EventTalked:     {hannavalt, dustin, sirCollin, sirAron, sirKiel, silvershadow, spiritTalianus},
			script.EventMyDying: {
				hangmanTree, skeletonMarauder, skeletonRaider, strain, ghoul, brekaOrcOverlord, spiritOfSirHerod,
				letoLizardman, letoLizardmanArcher, letoLizardmanSoldier, letoLizardmanWarrior, letoLizardmanShaman, letoLizardmanOverlord,
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
	case strings.EqualFold(e.Name, "30109-04.htm"):
		st.SetStatus(questlog.StatusStarted)
		st.SetCond(1)
		s.PlaySound(p, script.SoundAccept)
		if secondclass.GiveDimensionalDiamonds35(s, p) {
			htmltext = "30109-04a.htm"
		}
	case strings.EqualFold(e.Name, "30116-05.htm"):
		st.SetCond(14)
		s.PlaySound(p, script.SoundMiddle)
		s.TakeItems(p, tearOfLoyalty, 1)
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
		switch class := p.ClassID(); {
		case class != classKnight && class != classElvenKnight && class != classPalusKnight:
			htmltext = "30109-02.htm"
		case p.Level() < 35:
			htmltext = "30109-01.htm"
		default:
			htmltext = "30109-03.htm"
		}
	case questlog.StatusStarted:
		cond := st.Cond()
		switch e.NPC.NpcID() {
		case hannavalt:
			if cond == 18 {
				htmltext = "30109-05.htm"
				s.TakeItems(p, letterOfDustin, 1)
				s.GiveItems(p, markOfDuty, 1)
				s.RewardExpAndSp(p, 79832, 3750)
				p.SocialAction(3)
				s.PlaySound(p, script.SoundFinish)
				st.Exit(false)
			} else {
				htmltext = "30109-04a.htm"
			}
		case sirAron:
			switch {
			case cond == 1:
				htmltext = "30653-01.htm"
				st.SetCond(2)
				s.PlaySound(p, script.SoundMiddle)
				s.GiveItems(p, oldKnightSword, 1)
			case cond == 2:
				htmltext = "30653-02.htm"
			case cond == 3:
				htmltext = "30653-03.htm"
				st.SetCond(4)
				s.PlaySound(p, script.SoundMiddle)
				s.TakeItems(p, knightsTear, 1)
				s.TakeItems(p, oldKnightSword, 1)
			case cond > 3:
				htmltext = "30653-04.htm"
			}
		case sirKiel:
			switch {
			case cond == 4:
				htmltext = "30654-01.htm"
				st.SetCond(5)
				s.PlaySound(p, script.SoundMiddle)
			case cond == 5:
				htmltext = "30654-02.htm"
			case cond == 6:
				htmltext = "30654-03.htm"
				st.SetCond(7)
				s.PlaySound(p, script.SoundMiddle)
				s.GiveItems(p, mirrorOfOrpic, 1)
			case cond == 7:
				htmltext = "30654-04.htm"
			case cond == 9:
				htmltext = "30654-05.htm"
				st.SetCond(10)
				s.PlaySound(p, script.SoundMiddle)
				s.TakeItems(p, tearOfConfession, 1)
			case cond > 9:
				htmltext = "30654-06.htm"
			}
		case spiritTalianus:
			if cond == 8 {
				htmltext = "30656-01.htm"
				st.SetCond(9)
				s.PlaySound(p, script.SoundMiddle)
				s.TakeItems(p, mirrorOfOrpic, 1)
				s.TakeItems(p, reportPiece2, 1)
				s.GiveItems(p, tearOfConfession, 1)
				// The spirit leaves once it has confessed.
				e.NPC.DeleteMe()
			}
		case silvershadow:
			switch cond {
			case 10:
				if p.Level() < 35 {
					htmltext = "30655-01.htm"
				} else {
					htmltext = "30655-02.htm"
					st.SetCond(11)
					s.PlaySound(p, script.SoundMiddle)
				}
			case 11:
				htmltext = "30655-03.htm"
			case 12:
				htmltext = "30655-04.htm"
				st.SetCond(13)
				s.PlaySound(p, script.SoundMiddle)
				s.TakeItems(p, militasArticle, -1)
				s.GiveItems(p, tearOfLoyalty, 1)
			case 13:
				htmltext = "30655-05.htm"
			}
		case dustin:
			switch cond {
			case 13:
				htmltext = "30116-01.htm"
			case 14:
				htmltext = "30116-06.htm"
			case 15:
				htmltext = "30116-07.htm"
				st.SetCond(16)
				s.PlaySound(p, script.SoundMiddle)
				s.TakeItems(p, athebaldtSkull, 1)
				s.TakeItems(p, athebaldtRibs, 1)
				s.TakeItems(p, athebaldtShin, 1)
				s.GiveItems(p, saintsAshesUrn, 1)
			case 16:
				htmltext = "30116-09.htm"
			case 17:
				htmltext = "30116-08.htm"
				st.SetCond(18)
				s.PlaySound(p, script.SoundMiddle)
				s.TakeItems(p, letterOfWindawood, 1)
				s.GiveItems(p, letterOfDustin, 1)
			case 18:
				htmltext = "30116-10.htm"
			}
		case sirCollin:
			switch {
			case cond == 16:
				htmltext = "30311-01.htm"
				st.SetCond(17)
				s.PlaySound(p, script.SoundMiddle)
				s.TakeItems(p, saintsAshesUrn, 1)
				s.GiveItems(p, letterOfWindawood, 1)
			case cond > 16:
				htmltext = "30311-02.htm"
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
	case skeletonMarauder, skeletonRaider:
		if cond == 2 && s.Rnd(10) < 1 {
			s.PlaySound(p, script.SoundBeforeBattle)
			s.Spawner().AddSpawn(spiritOfSirHerod, e.NPC, false, 120000*time.Millisecond)
		}
	case spiritOfSirHerod:
		if cond == 2 && p.RightHandItemID() == oldKnightSword {
			st.SetCond(3)
			s.PlaySound(p, script.SoundMiddle)
			s.GiveItems(p, knightsTear, 1)
		}
	case strain, ghoul:
		if cond == 5 && s.DropItemsAlways(p, reportPiece1, 1, 10) {
			st.SetCond(6)
			s.TakeItems(p, reportPiece1, -1)
			s.GiveItems(p, reportPiece2, 1)
		}
	case hangmanTree:
		if (cond == 7 || cond == 8) && s.Rnd(100) < 33 {
			if cond == 7 {
				st.SetCond(8)
				s.PlaySound(p, script.SoundMiddle)
			}
			s.Spawner().AddSpawn(spiritTalianus, e.NPC, false, 300000*time.Millisecond)
		}
	case letoLizardman, letoLizardmanArcher, letoLizardmanSoldier, letoLizardmanWarrior, letoLizardmanShaman, letoLizardmanOverlord:
		if cond == 11 && s.DropItemsAlways(p, militasArticle, 1, 20) {
			st.SetCond(12)
		}
	case brekaOrcOverlord:
		if cond == 14 && s.RndBool() {
			switch {
			case !p.HasItem(athebaldtSkull):
				s.PlaySound(p, script.SoundItemGet)
				s.GiveItems(p, athebaldtSkull, 1)
			case !p.HasItem(athebaldtRibs):
				s.PlaySound(p, script.SoundItemGet)
				s.GiveItems(p, athebaldtRibs, 1)
			case !p.HasItem(athebaldtShin):
				st.SetCond(15)
				s.PlaySound(p, script.SoundMiddle)
				s.GiveItems(p, athebaldtShin, 1)
			}
		}
	}
}
