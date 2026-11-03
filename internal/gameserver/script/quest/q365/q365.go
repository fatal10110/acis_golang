// Package q365 is the quest Devil's Legacy: Randolf buys the treasure chests
// the pirate zombies carry, and Collob opens one for a fee, at the risk of a
// curse.
package q365

import (
	"strings"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/questlog"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/script"
)

const questName = "Q365_DevilsLegacy"

// Npcs.
const (
	randolf = 30095
	collob  = 30092
)

// pirateTreasureChest is the quest item.
const pirateTreasureChest = 5873

// Pirate Zombie and Pirate Zombie Captain.
const pirateZombie = 20836

// adena is the currency item.
const adena = 57

// poisonOfDeath is the curse a rare find brings.
const poisonOfDeath skill.ID = 4082

// New returns the quest.
func New() script.Script {
	return script.Script{
		Title:   "Devil's Legacy",
		QuestID: 365,
		Items:   []int32{pirateTreasureChest},
		Bind: script.Bindings{
			script.EventQuestStart: {randolf},
			script.EventTalked:     {randolf, collob},
			script.EventMyDying:    {pirateZombie, 20845, 21629, 21630},
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
	case strings.EqualFold(e.Name, "30095-02.htm"):
		st.SetStatus(questlog.StatusStarted)
		st.SetCond(1)
		s.PlaySound(p, script.SoundAccept)
	case strings.EqualFold(e.Name, "30095-06.htm"):
		s.PlaySound(p, script.SoundGiveUp)
		st.Exit(true)
	case strings.EqualFold(e.Name, "30092-05.htm"):
		switch {
		case !p.HasItem(pirateTreasureChest):
			htmltext = "30092-02.htm"
		case p.Adena() < 600:
			htmltext = "30092-03.htm"
		default:
			s.TakeItems(p, pirateTreasureChest, 1)
			s.TakeItems(p, adena, 600)

			if s.Rnd(100) < 80 {
				i0 := s.Rnd(100)
				switch {
				case i0 < 1:
					s.GiveItems(p, 955, 1)
				case i0 < 4:
					s.GiveItems(p, 956, 1)
				case i0 < 36:
					s.GiveItems(p, 1868, 1)
				case i0 < 68:
					s.GiveItems(p, 1884, 1)
				default:
					s.GiveItems(p, 1872, 1)
				}
				htmltext = "30092-05.htm"
			} else {
				i0 := s.Rnd(1000)
				switch {
				case i0 < 10:
					s.GiveItems(p, 951, 1)
				case i0 < 40:
					s.GiveItems(p, 952, 1)
				case i0 < 60:
					s.GiveItems(p, 955, 1)
				case i0 < 260:
					s.GiveItems(p, 956, 1)
				case i0 < 445:
					s.GiveItems(p, 1879, 1)
				case i0 < 630:
					s.GiveItems(p, 1880, 1)
				case i0 < 815:
					s.GiveItems(p, 1882, 1)
				default:
					s.GiveItems(p, 1881, 1)
				}
				htmltext = "30092-06.htm"

				// Curse effect!
				if !p.HasEffect(int32(poisonOfDeath)) {
					s.ApplyEffects(skill.Ref{ID: poisonOfDeath, Level: 1}, e.NPC, p)
				}
			}
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
		if p.Level() < 39 {
			htmltext = "30095-00.htm"
		} else {
			htmltext = "30095-01.htm"
		}
	case questlog.StatusStarted:
		switch e.NPC.NpcID() {
		case randolf:
			if !p.HasItem(pirateTreasureChest) {
				htmltext = "30095-03.htm"
			} else {
				htmltext = "30095-05.htm"

				reward := p.ItemCount(pirateTreasureChest) * 400

				s.TakeItems(p, pirateTreasureChest, -1)
				s.RewardItems(p, adena, reward+19800)
			}
		case collob:
			htmltext = "30092-01.htm"
		}
	}
	return htmltext
}

func onMyDying(s *script.Script, e script.MyDying) {
	st := s.RandomPartyMemberState(script.ActingPlayer(e.Killer), e.NPC, questlog.StatusStarted)
	if st == nil {
		return
	}
	chance := int32(520000)
	if e.NPC.NpcID() == pirateZombie {
		chance = 360000
	}
	s.DropItems(st.Player(), pirateTreasureChest, 1, 0, chance)
}
