// Package q371 is the quest Shriek of Ghosts: Reva buys the urns of the Hallate's
// ghosts and Patrin appraises the porcelain.
package q371

import (
	"strings"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/questlog"
	"github.com/fatal10110/acis_golang/internal/gameserver/script"
)

const questName = "Q371_ShriekOfGhosts"

// NPCs.
const (
	reva   = 30867
	patrin = 30929
)

// Quest items.
const (
	urn       = 5903
	porcelain = 6002
)

// New returns the quest.
func New() script.Script {
	return script.Script{
		Title:   "Shriek of Ghosts",
		QuestID: 371,
		Items:   []int32{urn, porcelain},
		Bind: script.Bindings{
			script.EventQuestStart: {reva},
			script.EventTalked:     {reva, patrin},
			script.EventMyDying:    {20818, 20820, 20824},
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
	case strings.EqualFold(e.Name, "30867-03.htm"):
		st.SetStatus(questlog.StatusStarted)
		st.SetCond(1)
		s.PlaySound(p, script.SoundAccept)
	case strings.EqualFold(e.Name, "30867-07.htm"):
		urns := p.ItemCount(urn)
		if urns > 0 {
			s.TakeItems(p, urn, urns)
			if urns >= 100 {
				urns += 13
				htmltext = "30867-08.htm"
			} else {
				urns += 7
			}
			s.RewardItems(p, 57, urns*1000)
		}
	case strings.EqualFold(e.Name, "30867-10.htm"):
		s.PlaySound(p, script.SoundGiveUp)
		st.Exit(true)
	case strings.EqualFold(e.Name, "APPR"):
		if p.HasItem(porcelain) {
			chance := s.Rnd(100)
			s.TakeItems(p, porcelain, 1)
			switch {
			case chance < 2:
				s.GiveItems(p, 6003, 1)
				htmltext = "30929-03.htm"
			case chance < 32:
				s.GiveItems(p, 6004, 1)
				htmltext = "30929-04.htm"
			case chance < 62:
				s.GiveItems(p, 6005, 1)
				htmltext = "30929-05.htm"
			case chance < 77:
				s.GiveItems(p, 6006, 1)
				htmltext = "30929-06.htm"
			default:
				htmltext = "30929-07.htm"
			}
		} else {
			htmltext = "30929-02.htm"
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
		if p.Level() < 59 {
			htmltext = "30867-01.htm"
		} else {
			htmltext = "30867-02.htm"
		}
	case questlog.StatusStarted:
		switch e.NPC.NpcID() {
		case reva:
			switch {
			case !p.HasItem(urn):
				htmltext = "30867-06.htm"
			case p.HasItem(porcelain):
				htmltext = "30867-05.htm"
			default:
				htmltext = "30867-04.htm"
			}
		case patrin:
			htmltext = "30929-01.htm"
		}
	}
	return htmltext
}

func onMyDying(s *script.Script, e script.MyDying) {
	st := s.RandomPartyMemberState(script.ActingPlayer(e.Killer), e.NPC, questlog.StatusStarted)
	if st == nil {
		return
	}
	urnBelow, porcelainBelow := chances(e.NPC.NpcID())
	random := s.Rnd(100)
	if random < porcelainBelow {
		item := int32(porcelain)
		if random < urnBelow {
			item = urn
		}
		s.DropItemsAlways(st.Player(), item, 1, 0)
	}
}

// chances is the upper bound, out of 100, of the roll that drops an urn and of
// the roll that drops an urn or porcelain, for a monster.
func chances(npcID int32) (urnBelow, porcelainBelow int) {
	switch npcID {
	case 20818:
		return 38, 43
	case 20820:
		return 48, 56
	default:
		return 50, 58
	}
}
