// Package q103 is the quest Spirit of Craftsman: a dark elf runs Karrod's
// errand between Cecktinon and Harne, from bone fragments to a soul catcher
// and a steelbender's head, for the Blood Saber.
package q103

import (
	"strings"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/player"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/questlog"
	"github.com/fatal10110/acis_golang/internal/gameserver/script"
)

const questName = "Q103_SpiritOfCraftsman"

// Items.
const (
	karrodLetter      = 968
	cecktinonVoucher1 = 969
	cecktinonVoucher2 = 970
	soulCatcher       = 971
	preservingOil     = 972
	zombieHead        = 973
	steelbenderHead   = 974
	boneFragment      = 1107
)

// Rewards.
const (
	bloodSaber        = 975
	lesserHealingPot  = 1060
	soulshotNoGrade   = 1835
	spiritshotNoGrade = 2509
	echoBattle        = 4412
	echoLove          = 4413
	echoSolitude      = 4414
	echoFeast         = 4415
	echoCelebration   = 4416
)

// NPCs.
const (
	karrod    = 30307
	cecktinon = 30132
	harne     = 30144
)

// New returns the quest.
func New() script.Script {
	return script.Script{
		Title:   "Spirit of Craftsman",
		QuestID: 103,
		Items: []int32{
			karrodLetter, cecktinonVoucher1, cecktinonVoucher2, boneFragment,
			soulCatcher, preservingOil, zombieHead, steelbenderHead,
		},
		Bind: script.Bindings{
			script.EventQuestStart: {karrod},
			script.EventTalked:     {karrod, cecktinon, harne},
			script.EventMyDying:    {20015, 20020, 20455, 20517, 20518},
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
	if strings.EqualFold(e.Name, "30307-05.htm") {
		st.SetStatus(questlog.StatusStarted)
		st.SetCond(1)
		s.PlaySound(e.Player, script.SoundAccept)
		s.GiveItems(e.Player, karrodLetter, 1)
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
		switch {
		case p.Race() != player.RaceDarkElf:
			htmltext = "30307-00.htm"
		case p.Level() < 11:
			htmltext = "30307-02.htm"
		default:
			htmltext = "30307-03.htm"
		}
	case questlog.StatusStarted:
		cond := st.Cond()
		switch e.NPC.NpcID() {
		case karrod:
			switch {
			case cond < 8:
				htmltext = "30307-06.htm"
			case cond == 8:
				htmltext = "30307-07.htm"

				s.TakeItems(p, steelbenderHead, 1)
				s.GiveItems(p, bloodSaber, 1)

				if p.IsMageClass() {
					s.RewardItems(p, spiritshotNoGrade, 500)
				} else {
					s.RewardItems(p, soulshotNoGrade, 1000)
				}

				s.RewardNewbieShots(p, 7000, 0)
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
		case cecktinon:
			switch {
			case cond == 1:
				htmltext = "30132-01.htm"
				st.SetCond(2)
				s.PlaySound(p, script.SoundMiddle)
				s.TakeItems(p, karrodLetter, 1)
				s.GiveItems(p, cecktinonVoucher1, 1)
			case cond > 1 && cond < 5:
				htmltext = "30132-02.htm"
			case cond == 5:
				htmltext = "30132-03.htm"
				st.SetCond(6)
				s.PlaySound(p, script.SoundMiddle)
				s.TakeItems(p, soulCatcher, 1)
				s.GiveItems(p, preservingOil, 1)
			case cond == 6:
				htmltext = "30132-04.htm"
			case cond == 7:
				htmltext = "30132-05.htm"
				st.SetCond(8)
				s.PlaySound(p, script.SoundMiddle)
				s.TakeItems(p, zombieHead, 1)
				s.GiveItems(p, steelbenderHead, 1)
			case cond == 8:
				htmltext = "30132-06.htm"
			}
		case harne:
			switch cond {
			case 2:
				htmltext = "30144-01.htm"
				st.SetCond(3)
				s.PlaySound(p, script.SoundMiddle)
				s.TakeItems(p, cecktinonVoucher1, 1)
				s.GiveItems(p, cecktinonVoucher2, 1)
			case 3:
				htmltext = "30144-02.htm"
			case 4:
				htmltext = "30144-03.htm"
				st.SetCond(5)
				s.PlaySound(p, script.SoundMiddle)
				s.TakeItems(p, cecktinonVoucher2, 1)
				s.TakeItems(p, boneFragment, 10)
				s.GiveItems(p, soulCatcher, 1)
			case 5:
				htmltext = "30144-04.htm"
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
	case 20517, 20518, 20455:
		if st.Cond() == 3 && s.DropItems(p, boneFragment, 1, 10, 300000) {
			st.SetCond(4)
		}
	case 20015, 20020:
		if st.Cond() == 6 && s.DropItems(p, zombieHead, 1, 1, 300000) {
			st.SetCond(7)
			s.TakeItems(p, preservingOil, 1)
		}
	}
}
