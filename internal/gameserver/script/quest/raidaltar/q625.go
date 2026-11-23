package raidaltar

import (
	"strings"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/questlog"
	"github.com/fatal10110/acis_golang/internal/gameserver/script"
)

const q625Name = "Q625_TheFinestIngredients_Part2"

// icicleEmperorBumbalump is the raid the table calls.
const icicleEmperorBumbalump = 25296

// NPCs.
const (
	jeremy    = 31521
	yetiTable = 31542
)

// Items.
const (
	soySauceJar      = 7205
	foodForBumbalump = 7209
	specialYetiMeat  = 7210
)

// q625Rewards are the items the essence is traded for, one at random.
func q625Rewards() []int32 { return []int32{4589, 4590, 4591, 4592, 4593, 4594} }

// q625 is the quest; the raid mechanics are the site's.
type q625 struct{ site }

// Q625 is the quest The Finest Ingredients, Part 2: Jeremy turns the
// soy sauce jar into food for Bumbalump, the Yeti's table spends it on
// calling the Icicle Emperor Bumbalump, and the special Yeti meat is traded
// back for a reward.
func Q625() script.Script {
	q := q625{newSite(icicleEmperorBumbalump, script.Loc{X: 158240, Y: -121536, Z: -2222}, 62503, 62504, specialYetiMeat)}
	return script.Script{
		Title:   "The Finest Ingredients - Part 2",
		QuestID: 625,
		Items:   []int32{foodForBumbalump, specialYetiMeat},
		Bind: script.Bindings{
			script.EventQuestStart: {jeremy},
			script.EventTalked:     {jeremy, yetiTable},
			script.EventDecayed:    {icicleEmperorBumbalump},
			script.EventMyDying:    {icicleEmperorBumbalump},
		},
		Hooks: script.Hooks{OnEvent: q.onAdvEvent, OnTalk: q.onTalk, OnDecayed: q.onDecayed, OnMyDying: q.onMyDying},
	}
}

func (q q625) onAdvEvent(s *script.Script, e script.Event) string {
	htmltext := e.Name
	st := s.QuestState(e.Player, q625Name)
	if st == nil {
		return htmltext
	}
	p := e.Player
	switch {
	// Jeremy.
	case strings.EqualFold(e.Name, "31521-03.htm"):
		if p.HasItem(soySauceJar) {
			st.SetStatus(questlog.StatusStarted)
			st.SetCond(1)
			s.PlaySound(p, script.SoundAccept)
			s.TakeItems(p, soySauceJar, 1)
			s.GiveItems(p, foodForBumbalump, 1)
		} else {
			htmltext = "31521-04.htm"
		}
	case strings.EqualFold(e.Name, "31521-08.htm"):
		if p.HasItem(specialYetiMeat) {
			s.TakeItems(p, specialYetiMeat, 1)
			rs := q625Rewards()
			s.RewardItems(p, rs[s.Rnd(len(rs))], 5)
			s.PlaySound(p, script.SoundFinish)
			st.Exit(true)
		} else {
			htmltext = "31521-09.htm"
		}
	// Yeti's table.
	case strings.EqualFold(e.Name, "31542-02.htm"):
		switch {
		case !p.HasItem(foodForBumbalump):
			htmltext = "31542-03.htm"
		case !q.raid.Empty():
			htmltext = "31542-04.htm"
		default:
			if !q.call(s, e.NPC) {
				htmltext = "31542-04.htm"
				break
			}
			st.SetCond(2)
			s.PlaySound(p, script.SoundMiddle)
			s.TakeItems(p, foodForBumbalump, 1)
		}
	}
	return htmltext
}

func (q q625) onTalk(s *script.Script, e script.Talk) string {
	st := s.QuestState(e.Player, q625Name)
	htmltext := script.NoQuestMsg()
	if st == nil {
		return htmltext
	}
	switch st.Status() {
	case questlog.StatusCreated:
		if e.Player.Level() < 73 {
			htmltext = "31521-02.htm"
		} else {
			htmltext = "31521-01.htm"
		}
	case questlog.StatusStarted:
		cond := st.Cond()
		switch e.NPC.NpcID() {
		case jeremy:
			switch cond {
			case 1:
				htmltext = "31521-05.htm"
			case 2:
				htmltext = "31521-06.htm"
			default:
				htmltext = "31521-07.htm"
			}
		case yetiTable:
			switch cond {
			case 1:
				htmltext = "31542-01.htm"
			case 2:
				htmltext = "31542-05.htm"
			}
		}
	}
	return htmltext
}
