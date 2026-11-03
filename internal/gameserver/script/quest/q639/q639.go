// Package q639 is the quest Guardians of the Holy Grail: Dominic, for a
// character of level 73 or more, buys scriptures the guardians drop, and
// Gremory and the Holy Grail turn them into scrolls for a bottle of holy
// water.
package q639

import (
	"strings"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/questlog"
	"github.com/fatal10110/acis_golang/internal/gameserver/script"
)

const questName = "Q639_GuardiansOfTheHolyGrail"

// NPCs.
const (
	dominic   = 31350
	gremory   = 32008
	holyGrail = 32028
)

// Items.
const (
	scripture       = 8069
	waterBottle     = 8070
	holyWaterBottle = 8071
)

// monsters returns the monsters that drop scriptures.
func monsters() []int32 {
	return []int32{22122, 22123, 22124, 22125, 22126, 22127, 22128, 22129, 22130, 22131, 22132, 22133, 22134, 22135}
}

// chance returns the monster npcID's drop chance of a scripture.
func chance(npcID int32) int32 {
	switch npcID {
	case 22122:
		return 760000
	case 22123:
		return 750000
	case 22124:
		return 590000
	case 22125:
		return 580000
	case 22126:
		return 590000
	case 22127:
		return 580000
	case 22128:
		return 170000
	case 22129:
		return 590000
	case 22130:
		return 850000
	case 22131:
		return 920000
	case 22132:
		return 580000
	case 22133:
		return 930000
	case 22134:
		return 230000
	case 22135:
		return 580000
	}
	return 0
}

// New returns the quest.
func New() script.Script {
	return script.Script{
		Title:   "Guardians of the Holy Grail",
		QuestID: 639,
		Items:   []int32{scripture, waterBottle, holyWaterBottle},
		Bind: script.Bindings{
			script.EventQuestStart: {dominic},
			script.EventTalked:     {dominic, gremory, holyGrail},
			script.EventMyDying:    monsters(),
		},
		Hooks: script.Hooks{OnEvent: onAdvEvent, OnTalk: onTalk, OnMyDying: onMyDying},
	}
}

// onAdvEvent starts the quest, pays adena for the scriptures, trades them for
// a scroll, hands over the water bottle, or ends the quest.
func onAdvEvent(s *script.Script, e script.Event) string {
	htmltext := e.Name
	st := s.QuestState(e.Player, questName)
	if st == nil {
		return htmltext
	}
	switch {
	// Dominic.
	case strings.EqualFold(e.Name, "31350-04.htm"):
		st.SetStatus(questlog.StatusStarted)
		st.SetCond(1)
		s.PlaySound(e.Player, script.SoundAccept)
	case strings.EqualFold(e.Name, "31350-08.htm"):
		count := e.Player.ItemCount(scripture)

		s.TakeItems(e.Player, scripture, -1)
		bonus := int32(0)
		if count >= 10 {
			bonus = 33940
		}
		s.RewardItems(e.Player, 57, 1625*count+bonus)
	case strings.EqualFold(e.Name, "31350-09.htm"):
		s.PlaySound(e.Player, script.SoundGiveUp)
		st.Exit(true)
	// Gremory.
	case strings.EqualFold(e.Name, "32008-05.htm"):
		st.SetCond(2)
		s.PlaySound(e.Player, script.SoundMiddle)
		s.GiveItems(e.Player, waterBottle, 1)
	case strings.EqualFold(e.Name, "32008-09.htm"):
		st.SetCond(4)
		s.PlaySound(e.Player, script.SoundMiddle)
		s.TakeItems(e.Player, holyWaterBottle, 1)
	case strings.EqualFold(e.Name, "32008-12.htm"):
		if e.Player.ItemCount(scripture) >= 4000 {
			htmltext = "32008-11.htm"
			s.TakeItems(e.Player, scripture, 4000)
			s.RewardItems(e.Player, 959, 1)
		}
	case strings.EqualFold(e.Name, "32008-14.htm"):
		if e.Player.ItemCount(scripture) >= 400 {
			htmltext = "32008-13.htm"
			s.TakeItems(e.Player, scripture, 400)
			s.RewardItems(e.Player, 960, 1)
		}
	// Holy Grail.
	case strings.EqualFold(e.Name, "32028-02.htm"):
		st.SetCond(3)
		s.PlaySound(e.Player, script.SoundMiddle)
		s.TakeItems(e.Player, waterBottle, 1)
		s.GiveItems(e.Player, holyWaterBottle, 1)
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
		if e.Player.Level() < 73 {
			htmltext = "31350-02.htm"
		} else {
			htmltext = "31350-01.htm"
		}
	case questlog.StatusStarted:
		cond := st.Cond()
		switch e.NPC.NpcID() {
		case dominic:
			if e.Player.HasItem(scripture) {
				htmltext = "31350-05.htm"
			} else {
				htmltext = "31350-06.htm"
			}
		case gremory:
			switch cond {
			case 1:
				htmltext = "32008-01.htm"
			case 2:
				htmltext = "32008-06.htm"
			case 3:
				htmltext = "32008-08.htm"
			case 4:
				htmltext = "32008-10.htm"
			}
		case holyGrail:
			if cond == 2 {
				htmltext = "32028-01.htm"
			} else if cond > 2 {
				htmltext = "32028-03.htm"
			}
		}
	}
	return htmltext
}

// onMyDying drops a scripture to a party member who has the quest.
func onMyDying(s *script.Script, e script.MyDying) {
	p := script.ActingPlayer(e.Killer)
	st := s.RandomPartyMemberState(p, e.NPC, questlog.StatusStarted)
	if st == nil {
		return
	}
	s.DropItems(st.Player(), scripture, 1, 0, chance(e.NPC.NpcID()))
}
