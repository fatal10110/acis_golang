// Package q105 is the quest Skirmish with the Orcs: an elf follows two of
// Kendell's orders, each naming a Kaboo chief to slay for its torque, for
// a Red Sunset weapon.
package q105

import (
	"strings"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/player"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/questlog"
	"github.com/fatal10110/acis_golang/internal/gameserver/script"
)

const questName = "Q105_SkirmishWithTheOrcs"

// kendell is the quest giver.
const kendell = 30218

// Items.
const (
	kendellOrder1   = 1836
	kendellOrder2   = 1837
	kendellOrder3   = 1838
	kendellOrder4   = 1839
	kendellOrder5   = 1840
	kendellOrder6   = 1841
	kendellOrder7   = 1842
	kendellOrder8   = 1843
	kabooChiefTorc1 = 1844
	kabooChiefTorc2 = 1845
)

// Monsters.
const (
	kabooChiefUoph    = 27059
	kabooChiefKracha  = 27060
	kabooChiefBatoh   = 27061
	kabooChiefTanukia = 27062
	kabooChiefTurel   = 27064
	kabooChiefRoko    = 27065
	kabooChiefKamut   = 27067
	kabooChiefMurtika = 27068
)

// Rewards.
const (
	redSunsetStaff    = 754
	redSunsetSword    = 981
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
		Title:   "Skirmish with the Orcs",
		QuestID: 105,
		Items: []int32{
			kendellOrder1, kendellOrder2, kendellOrder3, kendellOrder4, kendellOrder5,
			kendellOrder6, kendellOrder7, kendellOrder8, kabooChiefTorc1, kabooChiefTorc2,
		},
		Bind: script.Bindings{
			script.EventQuestStart: {kendell},
			script.EventTalked:     {kendell},
			script.EventMyDying: {
				kabooChiefUoph, kabooChiefKracha, kabooChiefBatoh, kabooChiefTanukia,
				kabooChiefTurel, kabooChiefRoko, kabooChiefKamut, kabooChiefMurtika,
			},
		},
		Hooks: script.Hooks{OnEvent: onAdvEvent, OnTalk: onTalk, OnMyDying: onMyDying},
	}
}

// rndBetween returns a random number of [lo, hi].
func rndBetween(s *script.Script, lo, hi int32) int32 {
	return lo + int32(s.Rnd(int(hi-lo+1)))
}

func onAdvEvent(s *script.Script, e script.Event) string {
	htmltext := e.Name
	st := s.QuestState(e.Player, questName)
	if st == nil {
		return htmltext
	}
	if strings.EqualFold(e.Name, "30218-03.htm") {
		st.SetStatus(questlog.StatusStarted)
		st.SetCond(1)
		s.PlaySound(e.Player, script.SoundAccept)
		s.GiveItems(e.Player, rndBetween(s, 1836, 1839), 1) // Kendell's orders 1 to 4.
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
		case p.Race() != player.RaceElf:
			htmltext = "30218-00.htm"
		case p.Level() < 10:
			htmltext = "30218-01.htm"
		default:
			htmltext = "30218-02.htm"
		}
	case questlog.StatusStarted:
		switch st.Cond() {
		case 1:
			htmltext = "30218-05.htm"
		case 2:
			htmltext = "30218-06.htm"
			st.SetCond(3)
			s.PlaySound(p, script.SoundMiddle)
			s.TakeItems(p, kabooChiefTorc1, 1)
			s.TakeItems(p, kendellOrder1, 1)
			s.TakeItems(p, kendellOrder2, 1)
			s.TakeItems(p, kendellOrder3, 1)
			s.TakeItems(p, kendellOrder4, 1)
			s.GiveItems(p, rndBetween(s, 1840, 1843), 1) // Kendell's orders 5 to 8.
		case 3:
			htmltext = "30218-07.htm"
		case 4:
			htmltext = "30218-08.htm"
			s.TakeItems(p, kabooChiefTorc2, 1)
			s.TakeItems(p, kendellOrder5, 1)
			s.TakeItems(p, kendellOrder6, 1)
			s.TakeItems(p, kendellOrder7, 1)
			s.TakeItems(p, kendellOrder8, 1)

			if p.IsMageClass() {
				s.GiveItems(p, redSunsetStaff, 1)
				s.RewardItems(p, spiritshotNoGrade, 500)
			} else {
				s.GiveItems(p, redSunsetSword, 1)
				s.RewardItems(p, soulshotNoGrade, 1000)
			}

			s.RewardNewbieShots(p, 7000, 3000)
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
	npcID := e.NPC.NpcID()
	switch npcID {
	case kabooChiefUoph, kabooChiefKracha, kabooChiefBatoh, kabooChiefTanukia:
		if st.Cond() == 1 && p.HasItem(npcID-25223) { // npcId - 25223 = itemId to verify.
			st.SetCond(2)
			s.PlaySound(p, script.SoundMiddle)
			s.GiveItems(p, kabooChiefTorc1, 1)
		}
	case kabooChiefTurel, kabooChiefRoko:
		if st.Cond() == 3 && p.HasItem(npcID-25224) { // npcId - 25224 = itemId to verify.
			st.SetCond(4)
			s.PlaySound(p, script.SoundMiddle)
			s.GiveItems(p, kabooChiefTorc2, 1)
		}
	case kabooChiefKamut, kabooChiefMurtika:
		if st.Cond() == 3 && p.HasItem(npcID-25225) { // npcId - 25225 = itemId to verify.
			st.SetCond(4)
			s.PlaySound(p, script.SoundMiddle)
			s.GiveItems(p, kabooChiefTorc2, 1)
		}
	}
}
