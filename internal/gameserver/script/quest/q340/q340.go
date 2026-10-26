// Package q340 is the quest Subjugation of Lizardmen: Weisz sends the player
// for cargo from the lizardmen, then for a holy relic and a rosary for Adonius,
// and last for the totem from the chest the Serpent Demon Bifrons leaves.
package q340

import (
	"strings"
	"time"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/questlog"
	"github.com/fatal10110/acis_golang/internal/gameserver/script"
)

const questName = "Q340_SubjugationOfLizardmen"

// Npcs.
const (
	weisz   = 30385
	adonius = 30375
	levian  = 30037
	chest   = 30989
)

// Quest items.
const (
	cargo  = 4255
	holy   = 4256
	rosary = 4257
	totem  = 4258
)

// Monsters.
const (
	bifrons = 25146
)

// adena is the currency item.
const adena = 57

// chestDespawn is how long the chest of Bifrons stands.
const chestDespawn = 30000 * time.Millisecond

// New returns the quest.
func New() script.Script {
	return script.Script{
		Title:   "Subjugation of Lizardmen",
		QuestID: 340,
		Items:   []int32{cargo, holy, rosary, totem},
		Bind: script.Bindings{
			script.EventQuestStart: {weisz},
			script.EventTalked:     {weisz, adonius, levian, chest},
			script.EventMyDying:    {20008, 20010, 20014, 20024, 20027, 20030, bifrons},
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
	case strings.EqualFold(e.Name, "30385-03.htm"):
		st.SetStatus(questlog.StatusStarted)
		st.SetCond(1)
		s.PlaySound(p, script.SoundAccept)
	case strings.EqualFold(e.Name, "30385-07.htm"):
		st.SetCond(2)
		s.PlaySound(p, script.SoundMiddle)
		s.TakeItems(p, cargo, -1)
	case strings.EqualFold(e.Name, "30385-09.htm"):
		s.TakeItems(p, cargo, -1)
		s.RewardItems(p, adena, 4090)
	case strings.EqualFold(e.Name, "30385-10.htm"):
		s.TakeItems(p, cargo, -1)
		s.RewardItems(p, adena, 4090)
		st.Exit(true)
	case strings.EqualFold(e.Name, "30375-02.htm"):
		st.SetCond(3)
		s.PlaySound(p, script.SoundMiddle)
	case strings.EqualFold(e.Name, "30037-02.htm"):
		st.SetCond(5)
		s.PlaySound(p, script.SoundMiddle)
	case strings.EqualFold(e.Name, "30989-02.htm"):
		st.SetCond(6)
		s.PlaySound(p, script.SoundMiddle)
		s.GiveItems(p, totem, 1)
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
		if p.Level() < 17 {
			htmltext = "30385-01.htm"
		} else {
			htmltext = "30385-02.htm"
		}
	case questlog.StatusStarted:
		cond := st.Cond()
		switch e.NPC.NpcID() {
		case weisz:
			switch cond {
			case 1:
				if p.ItemCount(cargo) < 30 {
					htmltext = "30385-05.htm"
				} else {
					htmltext = "30385-06.htm"
				}
			case 2:
				htmltext = "30385-11.htm"
			case 7:
				htmltext = "30385-13.htm"
				s.RewardItems(p, adena, 14700)
				s.PlaySound(p, script.SoundFinish)
				st.Exit(false)
			}
		case adonius:
			switch cond {
			case 2:
				htmltext = "30375-01.htm"
			case 3:
				if p.HasItems(rosary, holy) {
					htmltext = "30375-04.htm"
					st.SetCond(4)
					s.PlaySound(p, script.SoundMiddle)
					s.TakeItems(p, holy, -1)
					s.TakeItems(p, rosary, -1)
				} else {
					htmltext = "30375-03.htm"
				}
			case 4:
				htmltext = "30375-05.htm"
			}
		case levian:
			switch cond {
			case 4:
				htmltext = "30037-01.htm"
			case 5:
				htmltext = "30037-03.htm"
			case 6:
				htmltext = "30037-04.htm"
				st.SetCond(7)
				s.PlaySound(p, script.SoundMiddle)
				s.TakeItems(p, totem, -1)
			case 7:
				htmltext = "30037-05.htm"
			}
		case chest:
			if cond == 5 {
				htmltext = "30989-01.htm"
			} else {
				htmltext = "30989-03.htm"
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
	case 20008:
		if st.Cond() == 1 {
			s.DropItems(p, cargo, 1, 30, 500000)
		}
	case 20010:
		if st.Cond() == 1 {
			s.DropItems(p, cargo, 1, 30, 520000)
		}
	case 20014:
		if st.Cond() == 1 {
			s.DropItems(p, cargo, 1, 30, 550000)
		}
	case 20024, 20027, 20030:
		if st.Cond() == 3 && s.DropItems(p, holy, 1, 1, 100000) {
			s.DropItems(p, rosary, 1, 1, 100000)
		}
	case bifrons:
		s.Spawner().AddSpawn(chest, e.NPC, false, chestDespawn)
	}
}
