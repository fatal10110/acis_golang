// Package q367 is the quest Electrifying Recharge: Lorain's titan lamp is
// charged by the lightning of the Catherok.
package q367

import (
	"strings"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/questlog"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/script"
)

const questName = "Q367_ElectrifyingRecharge"

// lorain is the quest giver.
const lorain = 30673

// Quest items.
const (
	initialTitanLamp = 5875
	titanLamp1       = 5876
	titanLamp2       = 5877
	titanLamp3       = 5878
	finalTitanLamp   = 5879
	brokenTitanLamp  = 5880
)

// catherok is the monster that charges the lamp.
const catherok = 21035

// New returns the quest.
func New() script.Script {
	return script.Script{
		Title:   "Electrifying Recharge!",
		QuestID: 367,
		Items:   []int32{initialTitanLamp, titanLamp1, titanLamp2, titanLamp3, finalTitanLamp, brokenTitanLamp},
		Bind: script.Bindings{
			script.EventQuestStart: {lorain},
			script.EventTalked:     {lorain},
			script.EventAttacked:   {catherok},
		},
		Hooks: script.Hooks{OnEvent: onAdvEvent, OnTalk: onTalk, OnAttacked: onAttacked},
	}
}

func onAdvEvent(s *script.Script, e script.Event) string {
	htmltext := e.Name
	st := s.QuestState(e.Player, questName)
	if st == nil {
		return htmltext
	}
	switch {
	case strings.EqualFold(e.Name, "30673-03.htm"):
		st.SetStatus(questlog.StatusStarted)
		st.SetCond(1)
		s.PlaySound(e.Player, script.SoundAccept)
		s.GiveItems(e.Player, initialTitanLamp, 1)
	case strings.EqualFold(e.Name, "30673-08.htm"):
		s.PlaySound(e.Player, script.SoundGiveUp)
		st.Exit(true)
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
		if p.Level() < 37 {
			htmltext = "30673-02.htm"
		} else {
			htmltext = "30673-01.htm"
		}
	case questlog.StatusStarted:
		switch st.Cond() {
		case 1:
			switch {
			case p.HasItem(brokenTitanLamp):
				htmltext = "30673-05.htm"
				s.TakeItems(p, brokenTitanLamp, -1)
				s.GiveItems(p, initialTitanLamp, 1)
				s.PlaySound(p, script.SoundAccept)
			case p.HasAtLeastOneItem(titanLamp1, titanLamp2, titanLamp3):
				htmltext = "30673-04.htm"
			default:
				htmltext = "30673-03.htm"
			}
		case 2:
			htmltext = "30673-06.htm"
			st.SetCond(1)
			s.TakeItems(p, finalTitanLamp, -1)
			s.GiveItems(p, initialTitanLamp, 1)
			// Dye reward.
			switch s.Rnd(14) {
			case 0:
				s.RewardItems(p, 4553, 1)
			case 1:
				s.RewardItems(p, 4554, 1)
			case 2:
				s.RewardItems(p, 4555, 1)
			case 3:
				s.RewardItems(p, 4556, 1)
			case 4:
				s.RewardItems(p, 4557, 1)
			case 5:
				s.RewardItems(p, 4558, 1)
			case 6:
				s.RewardItems(p, 4559, 1)
			case 7:
				s.RewardItems(p, 4560, 1)
			case 8:
				s.RewardItems(p, 4561, 1)
			case 9:
				s.RewardItems(p, 4562, 1)
			case 10:
				s.RewardItems(p, 4563, 1)
			case 11:
				s.RewardItems(p, 4564, 1)
			default:
				s.RewardItems(p, 4445, 1)
			}
		}
	}
	return htmltext
}

func onAttacked(s *script.Script, e script.Attacked) {
	attacker := script.ActingPlayer(e.Attacker)
	st := s.RandomPartyMember(attacker, e.NPC, script.ByCond(1))
	if st == nil {
		return
	}
	// For every occurred attack, the NPC tries to cast skill 4072. The only
	// restrictions are inherent skill restrictions (mp cost, skill reuse, etc).
	e.NPC.AddCastDesire(attacker, skill.Ref{ID: 4072, Level: 1}, 1000000)
	p := st.Player()
	if p.HasItem(finalTitanLamp) {
		return
	}
	switch i0 := s.Rnd(37); {
	case i0 == 0:
		switch {
		case p.HasItem(initialTitanLamp):
			s.TakeItems(p, initialTitanLamp, -1)
			s.GiveItems(p, titanLamp1, 1)
			s.PlaySound(p, script.SoundItemGet)
		case p.HasItem(titanLamp1):
			s.TakeItems(p, titanLamp1, -1)
			s.GiveItems(p, titanLamp2, 1)
			s.PlaySound(p, script.SoundItemGet)
		case p.HasItem(titanLamp2):
			s.TakeItems(p, titanLamp2, -1)
			s.GiveItems(p, titanLamp3, 1)
			s.PlaySound(p, script.SoundItemGet)
		case p.HasItem(titanLamp3):
			st.SetCond(2)
			s.PlaySound(p, script.SoundMiddle)
			s.TakeItems(p, titanLamp3, -1)
			s.GiveItems(p, finalTitanLamp, 1)
		}
	case i0 == 1 && !p.HasItem(brokenTitanLamp):
		s.TakeItems(p, initialTitanLamp, -1)
		s.TakeItems(p, titanLamp1, -1)
		s.TakeItems(p, titanLamp2, -1)
		s.TakeItems(p, titanLamp3, -1)
		s.GiveItems(p, brokenTitanLamp, 1)
		s.PlaySound(p, script.SoundItemGet)
	}
}
