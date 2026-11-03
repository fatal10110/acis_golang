// Package q409 is the quest Path to an Elven Oracle: an elven mystic proves
// itself by defeating the lizardmen Allana calls, and by Tamil, the swindler
// Perrin points out.
package q409

import (
	"strings"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/questlog"
	"github.com/fatal10110/acis_golang/internal/gameserver/script"
)

const questName = "Q409_PathToAnElvenOracle"

// Quest items.
const (
	crystalMedallion   = 1231
	swindlerMoney      = 1232
	allanaDiary        = 1233
	lizardCaptainOrder = 1234
	leafOfOracle       = 1235
	halfOfDiary        = 1236
	tamilNecklace      = 1275
)

// Npcs.
const (
	manuel = 30293
	allana = 30424
	perrin = 30428
)

// Quest monsters.
const (
	lizardmanWarrior = 27032
	lizardmanScout   = 27033
	lizardman        = 27034
	tamil            = 27035
)

// What the monsters say when they are first hit and when one dies.
const (
	sayLizardman        = 40912
	sayLizardmanScout   = 40911
	sayLizardmanWarrior = 40909
	sayTamil            = 40913
	sayLizardmanDying   = 40910
)

// The classes the quest checks.
const (
	classElvenMystic = 25
	classElvenOracle = 29
)

// quest holds the monsters it called, each with the object id of the player
// it was called for, until it decays.
type quest struct{ spawns *script.IntMapCell[int32] }

// New returns the quest.
func New() script.Script {
	q := quest{spawns: new(script.IntMapCell[int32])}
	monsters := []int32{lizardmanWarrior, lizardmanScout, lizardman, tamil}
	return script.Script{
		Title:   "Path to an Elven Oracle",
		QuestID: 409,
		Items:   []int32{crystalMedallion, swindlerMoney, allanaDiary, lizardCaptainOrder, halfOfDiary, tamilNecklace},
		Bind: script.Bindings{
			script.EventQuestStart: {manuel},
			script.EventTalked:     {manuel, allana, perrin},
			script.EventAttacked:   monsters,
			script.EventDecayed:    monsters,
			script.EventMyDying:    monsters,
		},
		Hooks: script.Hooks{
			OnEvent: q.onAdvEvent, OnTalk: q.onTalk, OnAttacked: onAttacked, OnDecayed: q.onDecayed, OnMyDying: onMyDying,
		},
	}
}

// call spawns the monster id at the npc for the player, and remembers it.
func (q quest) call(s *script.Script, id int32, at *script.NPC, p *script.Player) {
	if m := s.Spawner().AddSpawn(id, at, true, 0); m != nil {
		q.spawns.Set(m.ObjectID(), p.ObjectID())
	}
}

// hasCalled reports whether a monster called for the player still stands.
func (q quest) hasCalled(p *script.Player) bool {
	oid := p.ObjectID()
	for _, v := range q.spawns.Values() {
		if v == oid {
			return true
		}
	}
	return false
}

func (q quest) onAdvEvent(s *script.Script, e script.Event) string {
	htmltext := e.Name
	st := s.QuestState(e.Player, questName)
	if st == nil {
		return htmltext
	}
	p := e.Player
	switch {
	// Manuel
	case strings.EqualFold(e.Name, "30293-05.htm"):
		st.SetStatus(questlog.StatusStarted)
		st.SetCond(1)
		s.PlaySound(p, script.SoundAccept)
		s.GiveItems(p, crystalMedallion, 1)
	// Allana
	case strings.EqualFold(e.Name, "spawn_lizards"):
		st.SetCond(2)
		s.PlaySound(p, script.SoundMiddle)

		q.call(s, lizardmanWarrior, e.NPC, p)
		q.call(s, lizardmanScout, e.NPC, p)
		q.call(s, lizardman, e.NPC, p)
		return ""
	// Perrin
	case strings.EqualFold(e.Name, "30428-06.htm"):
		q.call(s, tamil, e.NPC, p)
	}
	return htmltext
}

func (q quest) onTalk(s *script.Script, e script.Talk) string {
	htmltext := script.NoQuestMsg()
	st := s.QuestState(e.Player, questName)
	if st == nil {
		return htmltext
	}
	p := e.Player
	switch st.Status() {
	case questlog.StatusCreated:
		switch {
		case p.ClassID() != classElvenMystic:
			if p.ClassID() == classElvenOracle {
				htmltext = "30293-02a.htm"
			} else {
				htmltext = "30293-02.htm"
			}
		case p.Level() < 19:
			htmltext = "30293-03.htm"
		case p.HasItem(leafOfOracle):
			htmltext = "30293-04.htm"
		default:
			htmltext = "30293-01.htm"
		}
	case questlog.StatusStarted:
		cond := st.Cond()
		switch e.NPC.NpcID() {
		case manuel:
			switch {
			case cond == 1:
				htmltext = "30293-06.htm"
			case cond == 2 || cond == 3:
				htmltext = "30293-09.htm"
			case cond > 3 && cond < 7:
				htmltext = "30293-07.htm"
			case cond == 7:
				htmltext = "30293-08.htm"
				s.TakeItems(p, allanaDiary, 1)
				s.TakeItems(p, crystalMedallion, 1)
				s.TakeItems(p, lizardCaptainOrder, 1)
				s.TakeItems(p, swindlerMoney, 1)
				s.GiveItems(p, leafOfOracle, 1)
				s.RewardExpAndSp(p, 3200, 1130)
				p.SocialAction(3)
				s.PlaySound(p, script.SoundFinish)
				st.Exit(true)
			}
		case allana:
			switch cond {
			case 1, 2:
				if q.hasCalled(p) {
					return ""
				}
				htmltext = "30424-01.htm"
			case 3:
				htmltext = "30424-02.htm"
				st.SetCond(4)
				s.PlaySound(p, script.SoundMiddle)
				s.GiveItems(p, halfOfDiary, 1)
			case 4:
				htmltext = "30424-03.htm"
			case 5:
				htmltext = "30424-06.htm"
			case 6:
				htmltext = "30424-04.htm"
				st.SetCond(7)
				s.PlaySound(p, script.SoundMiddle)
				s.TakeItems(p, halfOfDiary, -1)
				s.GiveItems(p, allanaDiary, 1)
			case 7:
				htmltext = "30424-05.htm"
			}
		case perrin:
			switch {
			case cond == 4:
				if q.hasCalled(p) {
					htmltext = "30428-06.htm"
				} else {
					htmltext = "30428-01.htm"
				}
			case cond == 5:
				htmltext = "30428-04.htm"
				st.SetCond(6)
				s.PlaySound(p, script.SoundMiddle)
				s.TakeItems(p, tamilNecklace, -1)
				s.GiveItems(p, swindlerMoney, 1)
			case cond > 5:
				htmltext = "30428-05.htm"
			}
		}
	}
	return htmltext
}

func onAttacked(s *script.Script, e script.Attacked) {
	condition := e.NPC.ScriptValue()
	if condition < 0 {
		return
	}

	// Attacker must be player with started quest.
	p := script.ActingPlayer(e.Attacker)
	if p == nil || s.CheckPlayerState(p, e.NPC, questlog.StatusStarted) == nil {
		return
	}

	if condition == 0 {
		// First valid attack. Save player object ID.
		switch e.NPC.NpcID() {
		case lizardman:
			e.NPC.Say(script.NpcString(sayLizardman))
		case lizardmanScout:
			e.NPC.Say(script.NpcString(sayLizardmanScout))
		case lizardmanWarrior:
			e.NPC.Say(script.NpcString(sayLizardmanWarrior))
		case tamil:
			e.NPC.Say(script.NpcString(sayTamil))
		}

		e.NPC.SetScriptValue(p.ObjectID())
	} else if condition != p.ObjectID() {
		// Repeated valid attack. Mark invalid attack condition, if another player also attacks the monster.
		e.NPC.SetScriptValue(-1)
	}
}

func (q quest) onDecayed(_ *script.Script, e script.Decayed) {
	q.spawns.Delete(e.NPC.ObjectID())
}

func onMyDying(s *script.Script, e script.MyDying) {
	p := script.ActingPlayer(e.Killer)
	st := s.CheckPlayerState(p, e.NPC, questlog.StatusStarted)
	if st == nil || e.NPC.ScriptValue() < 0 {
		return
	}
	switch e.NPC.NpcID() {
	case lizardmanWarrior, lizardmanScout, lizardman:
		if st.Cond() == 2 {
			e.NPC.Say(script.NpcString(sayLizardmanDying))

			st.SetCond(3)
			s.PlaySound(p, script.SoundMiddle)
			s.GiveItems(p, lizardCaptainOrder, 1)
		}
	case tamil:
		if st.Cond() == 4 {
			st.SetCond(5)
			s.PlaySound(p, script.SoundMiddle)
			s.GiveItems(p, tamilNecklace, 1)
		}
	}
}
