// Package q345 is the quest Method to Raise the Dead: Dorothy gathers a
// victim's bones to call back her friend's soul.
package q345

import (
	"strings"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/questlog"
	"github.com/fatal10110/acis_golang/internal/gameserver/script"
)

const questName = "Q345_MethodToRaiseTheDead"

// Items.
const (
	victimArmBone           = 4274
	victimThighBone         = 4275
	victimSkull             = 4276
	victimRibBone           = 4277
	victimSpine             = 4278
	uselessBonePieces       = 4280
	powderToSummonDeadSouls = 4281
)

// NPCs.
const (
	xenovia   = 30912
	dorothy   = 30970
	orpheus   = 30971
	mediumJar = 30973
)

// Rewards.
const (
	billOfIasonHeine = 4310
	imperialDiamond  = 3456
)

// New returns the quest.
func New() script.Script {
	return script.Script{
		Title:   "Method to Raise the Dead",
		QuestID: 345,
		Items:   []int32{victimArmBone, victimThighBone, victimSkull, victimRibBone, victimSpine, powderToSummonDeadSouls, uselessBonePieces},
		Bind: script.Bindings{
			script.EventQuestStart: {dorothy},
			script.EventTalked:     {dorothy, xenovia, mediumJar, orpheus},
			script.EventMyDying:    {20789, 20791},
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
	case strings.EqualFold(e.Name, "30970-03.htm"):
		st.SetStatus(questlog.StatusStarted)
		st.SetCond(1)
		s.PlaySound(p, script.SoundAccept)
	case strings.EqualFold(e.Name, "30970-06.htm"):
		st.SetCond(2)
		s.PlaySound(p, script.SoundMiddle)
	case strings.EqualFold(e.Name, "30912-04.htm"):
		if p.Adena() >= 1000 {
			htmltext = "30912-03.htm"
			st.SetCond(3)
			s.PlaySound(p, script.SoundMiddle)
			s.TakeItems(p, 57, 1000)
			s.GiveItems(p, powderToSummonDeadSouls, 1)
		}
	case strings.EqualFold(e.Name, "30973-04.htm"):
		if st.Cond() == 3 {
			chance := s.Rnd(3)
			switch chance {
			case 0:
				st.SetCond(6)
				htmltext = "30973-02a.htm"
			case 1:
				st.SetCond(6)
				htmltext = "30973-02b.htm"
			default:
				st.SetCond(7)
				htmltext = "30973-02c.htm"
			}
			s.TakeItems(p, powderToSummonDeadSouls, -1)
			s.TakeItems(p, victimArmBone, -1)
			s.TakeItems(p, victimThighBone, -1)
			s.TakeItems(p, victimSkull, -1)
			s.TakeItems(p, victimRibBone, -1)
			s.TakeItems(p, victimSpine, -1)
			s.PlaySound(p, script.SoundMiddle)
		}
	case strings.EqualFold(e.Name, "30971-02a.htm"):
		if p.HasItem(uselessBonePieces) {
			htmltext = "30971-02.htm"
		}
	case strings.EqualFold(e.Name, "30971-03.htm"):
		if p.HasItem(uselessBonePieces) {
			amount := p.ItemCount(uselessBonePieces) * 104
			s.TakeItems(p, uselessBonePieces, -1)
			s.RewardItems(p, 57, amount)
		} else {
			htmltext = "30971-02a.htm"
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
		if p.Level() < 35 {
			htmltext = "30970-00.htm"
		} else {
			htmltext = "30970-01.htm"
		}
	case questlog.StatusStarted:
		cond := st.Cond()
		switch e.NPC.NpcID() {
		case dorothy:
			switch {
			case cond == 1:
				if !p.HasItems(victimArmBone, victimThighBone, victimSkull, victimRibBone, victimSpine) {
					htmltext = "30970-04.htm"
				} else {
					htmltext = "30970-05.htm"
				}
			case cond == 2:
				htmltext = "30970-07.htm"
			case cond > 2 && cond < 6:
				htmltext = "30970-08.htm"
			default:
				// Shared part of cond 6 and 7.
				amount := p.ItemCount(uselessBonePieces) * 70
				s.TakeItems(p, uselessBonePieces, -1)
				if cond == 7 {
					// The scared little girl.
					htmltext = "30970-10.htm"
					s.RewardItems(p, 57, 3040+amount)
					// An Imperial Diamond or bills.
					if s.Rnd(100) < 10 {
						s.GiveItems(p, imperialDiamond, 1)
					} else {
						s.GiveItems(p, billOfIasonHeine, 5)
					}
				} else {
					// Dorothy's friends.
					htmltext = "30970-09.htm"
					s.RewardItems(p, 57, 5390+amount)
					s.GiveItems(p, billOfIasonHeine, 3)
				}
				s.PlaySound(p, script.SoundFinish)
				st.Exit(true)
			}
		case xenovia:
			if cond == 2 {
				htmltext = "30912-01.htm"
			} else if cond > 2 {
				htmltext = "30912-06.htm"
			}
		case mediumJar:
			htmltext = "30973-01.htm"
		case orpheus:
			htmltext = "30971-01.htm"
		}
	}
	return htmltext
}

func onMyDying(s *script.Script, e script.MyDying) {
	p := script.ActingPlayer(e.Killer)
	if s.CheckPlayerCondition(p, e.NPC, 1) == nil {
		return
	}
	if s.Rnd(4) == 0 {
		randomPart := rndBetween(s, victimArmBone, victimSpine)
		if !p.HasItem(randomPart) {
			s.PlaySound(p, script.SoundItemGet)
			s.GiveItems(p, randomPart, 1)
			return
		}
	}
	s.DropItemsAlways(p, uselessBonePieces, 1, 0)
}

// rndBetween returns a number from lo to hi, both included.
func rndBetween(s *script.Script, lo, hi int32) int32 {
	return lo + int32(s.Rnd(int(hi-lo+1)))
}
