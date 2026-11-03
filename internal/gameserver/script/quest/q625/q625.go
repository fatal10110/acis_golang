// Package q625 is the quest The Finest Ingredients, Part 2: Jeremy turns the
// soy sauce jar into food for Bumbalump, the Yeti's table spends it on
// calling the Icicle Emperor Bumbalump, and the special Yeti meat is traded
// back for a reward.
package q625

import (
	"strings"
	"time"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/questlog"
	"github.com/fatal10110/acis_golang/internal/gameserver/script"
)

const questName = "Q625_TheFinestIngredients_Part2"

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

// The NpcStrings the raid speaks: when called, and when it leaves unslain.
const (
	calledChat = 62503
	leftChat   = 62504
)

// raidLifetime is how long the called raid stays.
const raidLifetime = 1200000 * time.Millisecond

// rewards are the items the essence is traded for, one at random.
func rewards() []int32 { return []int32{4589, 4590, 4591, 4592, 4593, 4594} }

// quest holds the table that called the raid and the raid it called, shared
// by every player: one raid at a time, until it decays.
type quest struct {
	table, raid *script.NPCCell
}

// New returns the quest.
func New() script.Script {
	q := quest{table: new(script.NPCCell), raid: new(script.NPCCell)}
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
		Hooks: script.Hooks{OnEvent: q.onAdvEvent, OnTalk: onTalk, OnDecayed: q.onDecayed, OnMyDying: onMyDying},
	}
}

func (q quest) onAdvEvent(s *script.Script, e script.Event) string {
	htmltext := e.Name
	st := s.QuestState(e.Player, questName)
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
			rs := rewards()
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

// call places the raid, has it speak and removes the table that called it.
// It reports false when the raid could not be placed or when another
// player's call placed one meanwhile: the call is then refused as one made
// while the raid stood.
func (q quest) call(s *script.Script, table *script.NPC) bool {
	at := script.Loc{X: 158240, Y: -121536, Z: -2222, Heading: s.Rnd(65536)}
	raid := s.Spawner().AddSpawn(icicleEmperorBumbalump, at, false, raidLifetime)
	if raid == nil {
		return false
	}
	if !q.raid.SetIfEmpty(raid) {
		raid.DeleteMe()
		return false
	}
	raid.Say(script.NpcString(calledChat))
	q.table.Set(table)
	table.DeleteMe()
	return true
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

// onDecayed lets the table call another raid once the raid has left. A raid
// is dead already when its decayed hooks run, so the branch that brings the
// table back and has the raid speak never runs: the table comes back with its
// own respawn.
func (q quest) onDecayed(s *script.Script, e script.Decayed) {
	if !q.raid.ClearIf(e.NPC) {
		return
	}
	table := q.table.Get()
	if !e.NPC.Dead() {
		if table != nil {
			s.Spawner().DoRespawn(table)
		}
		e.NPC.Say(script.NpcString(leftChat))
	}
	q.table.ClearIf(table)
}

func onMyDying(s *script.Script, e script.MyDying) {
	st := s.RandomPartyMember(script.ActingPlayer(e.Killer), e.NPC, script.ByCond(2))
	if st == nil {
		return
	}
	st.SetCond(3)
	s.PlaySound(st.Player(), script.SoundMiddle)
	s.GiveItems(st.Player(), specialYetiMeat, 1)
}
