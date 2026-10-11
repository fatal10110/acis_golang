// Package q211 is the quest Trial of the Challenger, a second class-change
// trial: Kash sends a warrior after Shyslassys, Gorr, Baraham and the
// Succubus Queen, and Raldo hands over the Mark of Challenger.
package q211

import (
	"strings"
	"time"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/questlog"
	"github.com/fatal10110/acis_golang/internal/gameserver/script"
	"github.com/fatal10110/acis_golang/internal/gameserver/script/quest/secondclass"
)

const questName = "Q211_TrialOfTheChallenger"

// mithrilMines is the radar mark Filaur points to.
func mithrilMines() (x, y, z int32) { return 176560, -184969, -3729 }

// Items.
const (
	letterOfKash       = 2628
	watcherEye1        = 2629
	watcherEye2        = 2630
	scrollOfShyslassys = 2631
	brokenKey          = 2632
)

// Rewards.
const (
	adena                       = 57
	elvenNecklaceBeads          = 1904
	whiteTunicPattern           = 1936
	ironBootsDesign             = 1940
	manticorSkinGaitersPattern  = 1943
	ripGauntletsPattern         = 1946
	tomeOfBloodPage             = 2030
	mithrilScaleGaitersMaterial = 2918
	brigandineGauntletsPattern  = 2927
	markOfChallenger            = 2627
)

// NPCs.
const (
	filaur            = 30535
	kash              = 30644
	martien           = 30645
	raldo             = 30646
	chestOfShyslassys = 30647
)

// Monsters.
const (
	shyslassys    = 27110
	gorr          = 27112
	baraham       = 27113
	succubusQueen = 27114
)

// The classes the trial is open to.
const (
	classWarrior     = 1
	classElvenKnight = 19
	classPalusKnight = 32
	classOrcRaider   = 45
	classMonk        = 47
)

// New returns the quest.
func New() script.Script {
	return script.Script{
		Title:   "Trial of the Challenger",
		QuestID: 211,
		Items:   []int32{letterOfKash, watcherEye1, watcherEye2, scrollOfShyslassys, brokenKey},
		Bind: script.Bindings{
			script.EventQuestStart: {kash},
			script.EventTalked:     {filaur, kash, martien, raldo, chestOfShyslassys},
			script.EventMyDying:    {shyslassys, gorr, baraham, succubusQueen},
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
	// Kash.
	case strings.EqualFold(e.Name, "30644-05.htm"):
		st.SetStatus(questlog.StatusStarted)
		st.SetCond(1)
		s.PlaySound(p, script.SoundAccept)
		if secondclass.GiveDimensionalDiamonds35(s, p) {
			htmltext = "30644-05a.htm"
		}
	// Martien.
	case strings.EqualFold(e.Name, "30645-02.htm"):
		st.SetCond(4)
		s.PlaySound(p, script.SoundMiddle)
		s.TakeItems(p, letterOfKash, 1)
	// Raldo.
	case strings.EqualFold(e.Name, "30646-04.htm") || strings.EqualFold(e.Name, "30646-06.htm"):
		st.SetCond(8)
		s.PlaySound(p, script.SoundMiddle)
		s.TakeItems(p, watcherEye2, 1)
	// The chest of Shyslassys.
	case strings.EqualFold(e.Name, "30647-04.htm"):
		if p.HasItem(brokenKey) {
			if s.Rnd(10) < 2 {
				htmltext = "30647-03.htm"
				s.PlaySound(p, script.SoundJackpot)
				s.TakeItems(p, brokenKey, 1)
				chance := s.Rnd(100)
				switch {
				case chance > 90:
					s.RewardItems(p, brigandineGauntletsPattern, 1)
					s.RewardItems(p, ironBootsDesign, 1)
					s.RewardItems(p, manticorSkinGaitersPattern, 1)
					s.RewardItems(p, mithrilScaleGaitersMaterial, 1)
					s.RewardItems(p, ripGauntletsPattern, 1)
				case chance > 70:
					s.RewardItems(p, elvenNecklaceBeads, 1)
					s.RewardItems(p, tomeOfBloodPage, 1)
				case chance > 40:
					s.RewardItems(p, whiteTunicPattern, 1)
				default:
					s.RewardItems(p, ironBootsDesign, 1)
				}
			} else {
				htmltext = "30647-02.htm"
				s.TakeItems(p, brokenKey, 1)
				// Rnd.get(1, 1000): 1 to 1000 inclusive.
				s.RewardItems(p, adena, 1+int32(s.Rnd(1000)))
			}
		}
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
		case class != classWarrior && class != classElvenKnight && class != classPalusKnight &&
			class != classOrcRaider && class != classMonk:
			htmltext = "30644-02.htm"
		case p.Level() < 35:
			htmltext = "30644-01.htm"
		default:
			htmltext = "30644-03.htm"
		}
	case questlog.StatusStarted:
		cond := st.Cond()
		switch e.NPC.NpcID() {
		case kash:
			switch {
			case cond == 1:
				htmltext = "30644-06.htm"
			case cond == 2:
				htmltext = "30644-07.htm"
				st.SetCond(3)
				s.PlaySound(p, script.SoundMiddle)
				s.TakeItems(p, scrollOfShyslassys, 1)
				s.GiveItems(p, letterOfKash, 1)
			case cond == 3:
				htmltext = "30644-08.htm"
			case cond > 3:
				htmltext = "30644-09.htm"
			}
		case chestOfShyslassys:
			htmltext = "30647-01.htm"
		case martien:
			switch {
			case cond == 3:
				htmltext = "30645-01.htm"
			case cond == 4:
				htmltext = "30645-03.htm"
			case cond == 5:
				htmltext = "30645-04.htm"
				st.SetCond(6)
				s.PlaySound(p, script.SoundMiddle)
				s.TakeItems(p, watcherEye1, 1)
			case cond == 6:
				htmltext = "30645-05.htm"
			case cond == 7:
				htmltext = "30645-07.htm"
			case cond > 7:
				htmltext = "30645-06.htm"
			}
		case raldo:
			switch cond {
			case 7:
				htmltext = "30646-01.htm"
			case 8:
				htmltext = "30646-06a.htm"
			case 10:
				htmltext = "30646-07.htm"
				s.TakeItems(p, brokenKey, 1)
				s.GiveItems(p, markOfChallenger, 1)
				s.RewardExpAndSp(p, 72394, 11250)
				p.SocialAction(3)
				s.PlaySound(p, script.SoundFinish)
				st.Exit(false)
			}
		case filaur:
			switch cond {
			case 8:
				if p.Level() >= 36 {
					htmltext = "30535-01.htm"
					st.SetCond(9)
					s.PlaySound(p, script.SoundMiddle)
				} else {
					htmltext = "30535-03.htm"
				}
			case 9:
				htmltext = "30535-02.htm"
				p.AddRadarMarker(mithrilMines())
			case 10:
				htmltext = "30535-04.htm"
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
	switch e.NPC.NpcID() {
	case shyslassys:
		if st.Cond() == 1 {
			st.SetCond(2)
			s.PlaySound(p, script.SoundMiddle)
			s.GiveItems(p, brokenKey, 1)
			s.GiveItems(p, scrollOfShyslassys, 1)
			s.Spawner().AddSpawn(chestOfShyslassys, e.NPC, false, 200000*time.Millisecond)
		}
	case gorr:
		if st.Cond() == 4 && s.DropItemsAlways(p, watcherEye1, 1, 1) {
			st.SetCond(5)
		}
	case baraham:
		if st.Cond() == 6 && s.DropItemsAlways(p, watcherEye2, 1, 1) {
			st.SetCond(7)
		}
		s.Spawner().AddSpawn(raldo, e.NPC, false, 100000*time.Millisecond)
	case succubusQueen:
		if st.Cond() == 9 {
			st.SetCond(10)
			s.PlaySound(p, script.SoundMiddle)
		}
		s.Spawner().AddSpawn(raldo, e.NPC, false, 100000*time.Millisecond)
	}
}
