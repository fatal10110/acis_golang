// Package q104 is the quest Spirit of Mirrors: a human takes three of
// Gallint's oak wands, binds a spirit into each by slaying it with the wand
// in hand, and brings them back for the Wand of Adept.
package q104

import (
	"strconv"
	"strings"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/player"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/questlog"
	"github.com/fatal10110/acis_golang/internal/gameserver/script"
)

const questName = "Q104_SpiritOfMirrors"

// Items.
const (
	gallinsOakWand   = 748
	wandSpiritbound1 = 1135
	wandSpiritbound2 = 1136
	wandSpiritbound3 = 1137
)

// Rewards.
const (
	wandOfAdept       = 747
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
	gallint   = 30017
	arnold    = 30041
	johnstone = 30043
	kenyos    = 30045
)

// New returns the quest.
func New() script.Script {
	return script.Script{
		Title:   "Spirit of Mirrors",
		QuestID: 104,
		Items:   []int32{gallinsOakWand, wandSpiritbound1, wandSpiritbound2, wandSpiritbound3},
		Bind: script.Bindings{
			script.EventQuestStart: {gallint},
			script.EventTalked:     {gallint, arnold, johnstone, kenyos},
			script.EventMyDying:    {27003, 27004, 27005},
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
	if strings.EqualFold(e.Name, "30017-03.htm") {
		st.SetStatus(questlog.StatusStarted)
		st.SetCond(1)
		s.PlaySound(e.Player, script.SoundAccept)
		s.GiveItems(e.Player, gallinsOakWand, 1)
		s.GiveItems(e.Player, gallinsOakWand, 1)
		s.GiveItems(e.Player, gallinsOakWand, 1)
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
		case p.Race() != player.RaceHuman:
			htmltext = "30017-00.htm"
		case p.Level() < 10:
			htmltext = "30017-01.htm"
		default:
			htmltext = "30017-02.htm"
		}
	case questlog.StatusStarted:
		cond := st.Cond()
		switch e.NPC.NpcID() {
		case gallint:
			switch cond {
			case 1, 2:
				htmltext = "30017-04.htm"
			case 3:
				htmltext = "30017-05.htm"

				s.TakeItems(p, wandSpiritbound1, -1)
				s.TakeItems(p, wandSpiritbound2, -1)
				s.TakeItems(p, wandSpiritbound3, -1)

				s.GiveItems(p, wandOfAdept, 1)

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
		case kenyos, johnstone, arnold:
			htmltext = strconv.Itoa(int(e.NPC.NpcID())) + "-01.htm"
			if cond == 1 {
				st.SetCond(2)
				s.PlaySound(p, script.SoundMiddle)
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
	if p.RightHandItemID() != gallinsOakWand {
		return
	}
	switch e.NPC.NpcID() {
	case 27003:
		if !p.HasItem(wandSpiritbound1) {
			s.TakeItems(p, gallinsOakWand, 1)
			s.GiveItems(p, wandSpiritbound1, 1)

			if p.HasItems(wandSpiritbound2, wandSpiritbound3) {
				st.SetCond(3)
				s.PlaySound(p, script.SoundMiddle)
			} else {
				s.PlaySound(p, script.SoundItemGet)
			}
		}
	case 27004:
		if !p.HasItem(wandSpiritbound2) {
			s.TakeItems(p, gallinsOakWand, 1)
			s.GiveItems(p, wandSpiritbound2, 1)

			if p.HasItems(wandSpiritbound1, wandSpiritbound3) {
				st.SetCond(3)
				s.PlaySound(p, script.SoundMiddle)
			} else {
				s.PlaySound(p, script.SoundItemGet)
			}
		}
	case 27005:
		if !p.HasItem(wandSpiritbound3) {
			s.TakeItems(p, gallinsOakWand, 1)
			s.GiveItems(p, wandSpiritbound3, 1)

			if p.HasItems(wandSpiritbound1, wandSpiritbound2) {
				st.SetCond(3)
				s.PlaySound(p, script.SoundMiddle)
			} else {
				s.PlaySound(p, script.SoundItemGet)
			}
		}
	}
}
