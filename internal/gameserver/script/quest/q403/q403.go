// Package q403 is the quest Path to a Rogue: a human fighter earns
// Bezique's recommendation by killing the skeletons with Neti's weapons and
// then the Catseye Bandit until it has dropped all four stolen items.
package q403

import (
	"strings"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/questlog"
	"github.com/fatal10110/acis_golang/internal/gameserver/script"
)

const questName = "Q403_PathToARogue"

// Quest items.
const (
	beziqueLetter         = 1180
	netiBow               = 1181
	netiDagger            = 1182
	spartoiBones          = 1183
	horseshoeOfLight      = 1184
	mostWantedList        = 1185
	stolenJewelry         = 1186
	stolenTomes           = 1187
	stolenRing            = 1188
	stolenNecklace        = 1189
	beziqueRecommendation = 1190
)

// Npcs.
const (
	bezique = 30379
	neti    = 30425
)

// Monsters.
const (
	trackerSkeleton       = 20035
	trackerSkeletonLeader = 20042
	scoutSkeleton         = 20045
	sniperSkeleton        = 20051
	ruinSpartoi           = 20054
	ragingSpartoi         = 20060
	catseyeBandit         = 27038
)

// What the Catseye Bandit says when it is first hit and when it dies.
const (
	sayAttacked = 40306
	sayDying    = 40307
)

// The classes the quest checks.
const (
	classHumanFighter = 0x00
	classRogue        = 0x07
)

// New returns the quest.
func New() script.Script {
	return script.Script{
		Title:   "Path to a Rogue",
		QuestID: 403,
		Items:   []int32{beziqueLetter, netiBow, netiDagger, spartoiBones, horseshoeOfLight, mostWantedList, stolenJewelry, stolenTomes, stolenRing, stolenNecklace},
		Bind: script.Bindings{
			script.EventQuestStart: {bezique},
			script.EventTalked:     {bezique, neti},
			script.EventAttacked: {
				trackerSkeleton, trackerSkeletonLeader, scoutSkeleton, sniperSkeleton, ruinSpartoi, ragingSpartoi, catseyeBandit,
			},
			script.EventMyDying: {
				trackerSkeleton, trackerSkeletonLeader, scoutSkeleton, sniperSkeleton, ruinSpartoi, ragingSpartoi, catseyeBandit,
			},
		},
		Hooks: script.Hooks{OnEvent: onAdvEvent, OnTalk: onTalk, OnAttacked: onAttacked, OnMyDying: onMyDying},
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
	// Bezique
	case strings.EqualFold(e.Name, "30379-05.htm"):
		switch {
		case p.ClassID() != classHumanFighter:
			if p.ClassID() == classRogue {
				htmltext = "30379-02a.htm"
			} else {
				htmltext = "30379-02.htm"
			}
		case p.Level() < 19:
			htmltext = "30379-02.htm"
		case p.HasItem(beziqueRecommendation):
			htmltext = "30379-04.htm"
		}
	case strings.EqualFold(e.Name, "30379-06.htm"):
		st.SetStatus(questlog.StatusStarted)
		st.SetCond(1)
		s.PlaySound(p, script.SoundAccept)
		s.GiveItems(p, beziqueLetter, 1)
	// Neti
	case strings.EqualFold(e.Name, "30425-05.htm"):
		st.SetCond(2)
		s.PlaySound(p, script.SoundMiddle)
		s.TakeItems(p, beziqueLetter, 1)
		s.GiveItems(p, netiBow, 1)
		s.GiveItems(p, netiDagger, 1)
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
		htmltext = "30379-01.htm"
	case questlog.StatusStarted:
		cond := st.Cond()
		switch e.NPC.NpcID() {
		case bezique:
			switch {
			case cond == 1:
				htmltext = "30379-07.htm"
			case cond == 2 || cond == 3:
				htmltext = "30379-10.htm"
			case cond == 4:
				htmltext = "30379-08.htm"
				st.SetCond(5)
				s.PlaySound(p, script.SoundMiddle)
				s.TakeItems(p, horseshoeOfLight, 1)
				s.GiveItems(p, mostWantedList, 1)
			case cond == 5:
				htmltext = "30379-11.htm"
			case cond == 6:
				htmltext = "30379-09.htm"
				s.TakeItems(p, netiBow, 1)
				s.TakeItems(p, netiDagger, 1)
				s.TakeItems(p, stolenJewelry, 1)
				s.TakeItems(p, stolenNecklace, 1)
				s.TakeItems(p, stolenRing, 1)
				s.TakeItems(p, stolenTomes, 1)
				s.GiveItems(p, beziqueRecommendation, 1)
				s.RewardExpAndSp(p, 3200, 1500)
				p.SocialAction(3)
				s.PlaySound(p, script.SoundFinish)
				st.Exit(true)
			}
		case neti:
			switch {
			case cond == 1:
				htmltext = "30425-01.htm"
			case cond == 2:
				htmltext = "30425-06.htm"
			case cond == 3:
				htmltext = "30425-07.htm"
				st.SetCond(4)
				s.PlaySound(p, script.SoundMiddle)
				s.TakeItems(p, spartoiBones, 10)
				s.GiveItems(p, horseshoeOfLight, 1)
			case cond > 3:
				htmltext = "30425-08.htm"
			}
		}
	}
	return htmltext
}

func onAttacked(s *script.Script, e script.Attacked) {
	// Attack condition already failed, skip other checks.
	condition := e.NPC.ScriptValue()
	if condition < 0 {
		return
	}

	// Attacker must be player with started quest.
	p := script.ActingPlayer(e.Attacker)
	if p == nil || s.CheckPlayerState(p, e.NPC, questlog.StatusStarted) == nil {
		return
	}

	// Player must use Neti's Bow or Neti's Dagger, otherwise mark invalid attack condition.
	equippedItemID := p.RightHandItemID()
	if equippedItemID != netiBow && equippedItemID != netiDagger {
		e.NPC.SetScriptValue(-1)
		return
	}

	if condition == 0 {
		// First valid attack. Save player object ID.
		if e.NPC.NpcID() == catseyeBandit {
			e.NPC.Say(script.NpcString(sayAttacked))
		}

		e.NPC.SetScriptValue(p.ObjectID())
	} else if condition != p.ObjectID() {
		// Repeated valid attack. Mark invalid attack condition, if another player also attacks the monster.
		e.NPC.SetScriptValue(-1)
	}
}

func onMyDying(s *script.Script, e script.MyDying) {
	p := script.ActingPlayer(e.Killer)
	st := s.CheckPlayerState(p, e.NPC, questlog.StatusStarted)
	if st == nil || e.NPC.ScriptValue() < 0 {
		return
	}
	switch e.NPC.NpcID() {
	case trackerSkeleton, scoutSkeleton, sniperSkeleton:
		if st.Cond() == 2 && s.DropItems(p, spartoiBones, 1, 10, 200000) {
			st.SetCond(3)
		}
	case trackerSkeletonLeader:
		if st.Cond() == 2 && s.DropItems(p, spartoiBones, 1, 10, 300000) {
			st.SetCond(3)
		}
	case ruinSpartoi, ragingSpartoi:
		if st.Cond() == 2 && s.DropItems(p, spartoiBones, 1, 10, 800000) {
			st.SetCond(3)
		}
	case catseyeBandit:
		e.NPC.Say(script.NpcString(sayDying))

		if st.Cond() == 5 {
			randomItem := int32(stolenJewelry + s.Rnd(stolenNecklace-stolenJewelry+1))

			if !p.HasItem(randomItem) {
				s.GiveItems(p, randomItem, 1)

				if p.HasItems(stolenJewelry, stolenTomes, stolenRing, stolenNecklace) {
					st.SetCond(6)
					s.PlaySound(p, script.SoundMiddle)
				} else {
					s.PlaySound(p, script.SoundItemGet)
				}
			}
		}
	}
}
