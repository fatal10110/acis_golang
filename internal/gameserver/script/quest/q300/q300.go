// Package q300 is the quest Hunting Leto Lizardman: Rath rewards sixty
// bracelets of the Leto lizardmen with one of three prizes, drawn at random.
package q300

import (
	"strings"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/questlog"
	"github.com/fatal10110/acis_golang/internal/gameserver/script"
)

const questName = "Q300_HuntingLetoLizardman"

// Items.
const bracelet = 7139

// Monsters.
const (
	letoLizardman         = 20577
	letoLizardmanArcher   = 20578
	letoLizardmanSoldier  = 20579
	letoLizardmanWarrior  = 20580
	letoLizardmanOverlord = 20582
)

// New returns the quest.
func New() script.Script {
	return script.Script{
		Title:   "Hunting Leto Lizardman",
		QuestID: 300,
		Items:   []int32{bracelet},
		Bind: script.Bindings{
			script.EventQuestStart: {30126}, // Rath
			script.EventTalked:     {30126},
			script.EventMyDying:    {letoLizardman, letoLizardmanArcher, letoLizardmanSoldier, letoLizardmanWarrior, letoLizardmanOverlord},
		},
		Hooks: script.Hooks{OnEvent: onAdvEvent, OnTalk: onTalk, OnMyDying: onMyDying},
	}
}

// dropChance is the bracelet drop chance of each bound monster.
func dropChance(npcID int32) int32 {
	switch npcID {
	case letoLizardman:
		return 300000
	case letoLizardmanArcher:
		return 320000
	case letoLizardmanSoldier:
		return 350000
	case letoLizardmanWarrior:
		return 650000
	case letoLizardmanOverlord:
		return 700000
	}
	return 0
}

func onAdvEvent(s *script.Script, e script.Event) string {
	htmltext := e.Name
	st := s.QuestState(e.Player, questName)
	if st == nil {
		return htmltext
	}
	p := e.Player
	switch {
	case strings.EqualFold(e.Name, "30126-03.htm"):
		st.SetStatus(questlog.StatusStarted)
		st.SetCond(1)
		s.PlaySound(p, script.SoundAccept)
	case strings.EqualFold(e.Name, "30126-05.htm"):
		if p.ItemCount(bracelet) >= 60 {
			htmltext = "30126-06.htm"
			s.TakeItems(p, bracelet, -1)

			switch s.Rnd(3) {
			case 0:
				s.RewardItems(p, 57, 30000)
			case 1:
				s.RewardItems(p, 1867, 50)
			case 2:
				s.RewardItems(p, 1872, 50)
			}

			s.PlaySound(p, script.SoundFinish)
			st.Exit(true)
		}
	}
	return htmltext
}

func onTalk(s *script.Script, e script.Talk) string {
	st := s.QuestState(e.Player, questName)
	htmltext := script.NoQuestMsg()
	if st == nil {
		return htmltext
	}
	switch st.Status() {
	case questlog.StatusCreated:
		if e.Player.Level() < 34 {
			htmltext = "30126-01.htm"
		} else {
			htmltext = "30126-02.htm"
		}
	case questlog.StatusStarted:
		if st.Cond() == 1 {
			htmltext = "30126-04a.htm"
		} else {
			htmltext = "30126-04.htm"
		}
	}
	return htmltext
}

func onMyDying(s *script.Script, e script.MyDying) {
	st := s.RandomPartyMember(script.ActingPlayer(e.Killer), e.NPC, script.ByCond(1))
	if st == nil {
		return
	}
	if s.DropItems(st.Player(), bracelet, 1, 60, dropChance(e.NPC.NpcID())) {
		st.SetCond(2)
	}
}
