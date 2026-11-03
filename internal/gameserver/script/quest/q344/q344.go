// Package q344 is the quest 1000 Years, the End of Lamentation: Gilmore buys
// the articles of dead heroes and sends relics to their old owners.
package q344

import (
	"strconv"
	"strings"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/questlog"
	"github.com/fatal10110/acis_golang/internal/gameserver/script"
)

const questName = "Q344_1000YearsTheEndOfLamentation"

// NPCs.
const (
	gilmore    = 30754
	rodemai    = 30756
	orven      = 30857
	kaien      = 30623
	garvarentz = 30704
)

// Items.
const (
	articleDeadHero = 4269
	oldKey          = 4270
	oldHilt         = 4271
	oldTotem        = 4272
	crucifix        = 4273
)

// chances returns the article drop chance of each monster, by NPC id.
func chances() map[int32]int32 {
	return map[int32]int32{
		20236: 380000,
		20237: 490000,
		20238: 460000,
		20239: 490000,
		20240: 530000,
		20272: 380000,
		20273: 490000,
		20274: 460000,
		20275: 490000,
		20276: 530000,
	}
}

// New returns the quest.
func New() script.Script {
	return script.Script{
		Title:   "1000 Years, the End of Lamentation",
		QuestID: 344,
		Items:   []int32{articleDeadHero, oldKey, oldHilt, oldTotem, crucifix},
		Bind: script.Bindings{
			script.EventQuestStart: {gilmore},
			script.EventTalked:     {gilmore, rodemai, orven, garvarentz, kaien},
			script.EventMyDying:    {20236, 20237, 20238, 20239, 20240, 20272, 20273, 20274, 20275, 20276},
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
	case strings.EqualFold(e.Name, "30754-04.htm"):
		st.SetStatus(questlog.StatusStarted)
		st.SetCond(1)
		s.PlaySound(p, script.SoundAccept)
	case strings.EqualFold(e.Name, "30754-07.htm"):
		if st.GetInt("success") > 0 {
			st.SetCond(1)
			st.Unset("success")
			s.PlaySound(p, script.SoundMiddle)
		}
	case strings.EqualFold(e.Name, "30754-08.htm"):
		s.PlaySound(p, script.SoundFinish)
		st.Exit(true)
	case strings.EqualFold(e.Name, "30754-06.htm"):
		if !p.HasItem(articleDeadHero) {
			htmltext = "30754-06a.htm"
		} else {
			amount := p.ItemCount(articleDeadHero)
			s.TakeItems(p, articleDeadHero, -1)
			s.GiveItems(p, 57, amount*60)
			// The special item: the chance grows with the articles handed in.
			if int32(s.Rnd(1000)) < min(10, max(1, amount/10)) {
				htmltext = "30754-10.htm"
			}
		}
	case strings.EqualFold(e.Name, "30754-11.htm"):
		random := s.Rnd(4)
		switch {
		case random < 1:
			htmltext = "30754-12.htm"
			s.GiveItems(p, oldKey, 1)
		case random < 2:
			htmltext = "30754-13.htm"
			s.GiveItems(p, oldHilt, 1)
		case random < 3:
			htmltext = "30754-14.htm"
			s.GiveItems(p, oldTotem, 1)
		default:
			s.GiveItems(p, crucifix, 1)
		}
		st.SetCond(2)
		s.PlaySound(p, script.SoundMiddle)
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
		if p.Level() < 48 {
			htmltext = "30754-01.htm"
		} else {
			htmltext = "30754-02.htm"
		}
	case questlog.StatusStarted:
		cond := st.Cond()
		npcID := e.NPC.NpcID()
		switch npcID {
		case gilmore:
			if cond == 1 {
				if p.HasItem(articleDeadHero) {
					htmltext = "30754-05.htm"
				} else {
					htmltext = "30754-09.htm"
				}
			} else if cond == 2 {
				if st.GetInt("success") > 0 {
					htmltext = "30754-16.htm"
				} else {
					htmltext = "30754-15.htm"
				}
			}
		default:
			if cond == 2 {
				if st.GetInt("success") > 0 {
					htmltext = strconv.Itoa(int(npcID)) + "-02.htm"
				} else {
					rewards(s, p, st, npcID)
					htmltext = strconv.Itoa(int(npcID)) + "-01.htm"
				}
			}
		}
	}
	return htmltext
}

// rewards trades the relic its old owner npcID recognizes for his reward.
func rewards(s *script.Script, p *script.Player, st *script.QuestState, npcID int32) {
	switch npcID {
	case orven:
		if p.HasItem(crucifix) {
			st.Set("success", itoa(1))
			s.TakeItems(p, crucifix, -1)
			chance := s.Rnd(100)
			switch {
			case chance < 80:
				s.GiveItems(p, 1875, 19)
			case chance < 95:
				s.GiveItems(p, 952, 5)
			default:
				s.GiveItems(p, 2437, 1)
			}
		}
	case garvarentz:
		if p.HasItem(oldTotem) {
			st.Set("success", itoa(1))
			s.TakeItems(p, oldTotem, -1)
			chance := s.Rnd(100)
			switch {
			case chance < 55:
				s.GiveItems(p, 1882, 70)
			case chance < 99:
				s.GiveItems(p, 1881, 50)
			default:
				s.GiveItems(p, 191, 1)
			}
		}
	case kaien:
		if p.HasItem(oldHilt) {
			st.Set("success", itoa(1))
			s.TakeItems(p, oldHilt, -1)
			chance := s.Rnd(100)
			switch {
			case chance < 60:
				s.GiveItems(p, 1874, 25)
			case chance < 85:
				s.GiveItems(p, 1887, 10)
			case chance < 99:
				s.GiveItems(p, 951, 1)
			default:
				s.GiveItems(p, 133, 1)
			}
		}
	case rodemai:
		if p.HasItem(oldKey) {
			st.Set("success", itoa(1))
			s.TakeItems(p, oldKey, -1)
			chance := s.Rnd(100)
			switch {
			case chance < 80:
				s.GiveItems(p, 1879, 55)
			case chance < 95:
				s.GiveItems(p, 951, 1)
			default:
				s.GiveItems(p, 885, 1)
			}
		}
	}
}

func onMyDying(s *script.Script, e script.MyDying) {
	p := script.ActingPlayer(e.Killer)
	if s.CheckPlayerCondition(p, e.NPC, 1) == nil {
		return
	}
	s.DropItems(p, articleDeadHero, 1, 0, chances()[e.NPC.NpcID()])
}

func itoa(n int32) string { return strconv.Itoa(int(n)) }
