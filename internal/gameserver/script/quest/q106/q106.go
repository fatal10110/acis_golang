// Package q106 is the quest Forgotten Truth: a dark elf carries Thifiell's
// onyx talisman to Kartia, recovers an ancient scroll and clay tablet from
// the Tumran orc brigands and brings Kartia's translation back, for the
// Eldritch Dagger.
package q106

import (
	"strings"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/player"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/questlog"
	"github.com/fatal10110/acis_golang/internal/gameserver/script"
)

const questName = "Q106_ForgottenTruth"

// NPCs.
const (
	thifiell = 30358
	kartia   = 30133
)

// Items.
const (
	onyxTalisman1     = 984
	onyxTalisman2     = 985
	ancientScroll     = 986
	ancientClayTablet = 987
	kartiaTranslation = 988
)

// Rewards.
const (
	eldritchDagger    = 989
	lesserHealingPot  = 1060
	soulshotNoGrade   = 1835
	spiritshotNoGrade = 2509
	echoBattle        = 4412
	echoLove          = 4413
	echoSolitude      = 4414
	echoFeast         = 4415
	echoCelebration   = 4416
)

// New returns the quest.
func New() script.Script {
	return script.Script{
		Title:   "Forgotten Truth",
		QuestID: 106,
		Items:   []int32{onyxTalisman1, onyxTalisman2, ancientScroll, ancientClayTablet, kartiaTranslation},
		Bind: script.Bindings{
			script.EventQuestStart: {thifiell},
			script.EventTalked:     {thifiell, kartia},
			script.EventMyDying:    {27070}, // Tumran Orc Brigand
		},
		Hooks: script.Hooks{OnEvent: onAdvEvent, OnTalk: onTalk, OnMyDying: onMyDying},
	}
}

func onAdvEvent(s *script.Script, e script.Event) string {
	st := s.QuestState(e.Player, questName)
	htmltext := e.Name
	if st == nil {
		return htmltext
	}
	if strings.EqualFold(e.Name, "30358-05.htm") {
		st.SetStatus(questlog.StatusStarted)
		st.SetCond(1)
		s.PlaySound(e.Player, script.SoundAccept)
		s.GiveItems(e.Player, onyxTalisman1, 1)
	}
	return htmltext
}

func onTalk(s *script.Script, e script.Talk) string {
	st := s.QuestState(e.Player, questName)
	htmltext := script.NoQuestMsg()
	if st == nil {
		return htmltext
	}
	p := e.Player
	switch st.Status() {
	case questlog.StatusCreated:
		switch {
		case p.Race() != player.RaceDarkElf:
			htmltext = "30358-00.htm"
		case p.Level() < 10:
			htmltext = "30358-02.htm"
		default:
			htmltext = "30358-03.htm"
		}
	case questlog.StatusStarted:
		cond := st.Cond()
		switch e.NPC.NpcID() {
		case thifiell:
			switch cond {
			case 1, 2, 3:
				htmltext = "30358-06.htm"
			case 4:
				htmltext = "30358-07.htm"
				s.TakeItems(p, kartiaTranslation, 1)
				s.GiveItems(p, eldritchDagger, 1)

				if p.IsMageClass() {
					s.RewardItems(p, spiritshotNoGrade, 500)
				} else {
					s.RewardItems(p, soulshotNoGrade, 1000)
				}

				s.RewardNewbieShots(p, 0, 3000)
				s.RewardItems(p, lesserHealingPot, 100)
				s.RewardItems(p, echoBattle, 10)
				s.RewardItems(p, echoLove, 10)
				s.RewardItems(p, echoSolitude, 10)
				s.RewardItems(p, echoFeast, 10)
				s.RewardItems(p, echoCelebration, 10)

				p.SocialAction(3)
				s.PlaySound(p, script.SoundFinish)
				st.Exit(false)
			}
		case kartia:
			switch cond {
			case 1:
				htmltext = "30133-01.htm"
				st.SetCond(2)
				s.PlaySound(p, script.SoundMiddle)
				s.TakeItems(p, onyxTalisman1, 1)
				s.GiveItems(p, onyxTalisman2, 1)
			case 2:
				htmltext = "30133-02.htm"
			case 3:
				htmltext = "30133-03.htm"
				st.SetCond(4)
				s.PlaySound(p, script.SoundMiddle)
				s.TakeItems(p, onyxTalisman2, 1)
				s.TakeItems(p, ancientScroll, 1)
				s.TakeItems(p, ancientClayTablet, 1)
				s.GiveItems(p, kartiaTranslation, 1)
			case 4:
				htmltext = "30133-04.htm"
			}
		}
	case questlog.StatusCompleted:
		htmltext = script.AlreadyCompletedMsg()
	}
	return htmltext
}

func onMyDying(s *script.Script, e script.MyDying) {
	p := script.ActingPlayer(e.Killer)
	st := s.CheckPlayerCondition(p, e.NPC, 2)
	if st == nil {
		return
	}
	if !p.HasItem(ancientScroll) {
		s.DropItems(p, ancientScroll, 1, 1, 200000)
	} else if s.DropItems(p, ancientClayTablet, 1, 1, 200000) {
		st.SetCond(3)
	}
}
