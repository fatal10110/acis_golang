// Package q023 is the quest Lidia's Heart: after the tragedy in von
// Hellmann's forest, Innocentin sends the player after Lidia's hairpin and
// diary, the ghost of von Hellmann woken at his tombstone, and the silver
// spear in the box for Violet.
package q023

import (
	"strings"
	"time"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/questlog"
	"github.com/fatal10110/acis_golang/internal/gameserver/script"
)

const questName = "Q023_LidiasHeart"

// NPCs.
const (
	innocentin         = 31328
	brokenBookshelf    = 31526
	ghostOfVonHellmann = 31524
	tombstone          = 31523
	violet             = 31386
	box                = 31530
)

// tragedyInVonHellman is the quest Lidia's Heart follows.
const tragedyInVonHellman = "Q022_TragedyInVonHellmannForest"

// Items.
const (
	forestOfDeadmanMap = 7063
	silverKey          = 7149
	lidiaHairpin       = 7148
	lidiaDiary         = 7064
	silverSpear        = 7150
)

// ghostSpawn is where the tombstone wakes the ghost.
func ghostSpawn() script.Loc { return script.Loc{X: 51432, Y: -54570, Z: -3136, Heading: 0} }

// quest holds the ghost the tombstone woke, shared by every player: one
// ghost at a time, until it decays.
type quest struct{ ghost *script.NPCCell }

// New returns the quest.
func New() script.Script {
	q := quest{ghost: new(script.NPCCell)}
	return script.Script{
		Title:   "Lidia's Heart",
		QuestID: 23,
		Items:   []int32{silverKey, lidiaDiary, silverSpear},
		Bind: script.Bindings{
			script.EventQuestStart: {innocentin},
			script.EventTalked:     {innocentin, brokenBookshelf, ghostOfVonHellmann, violet, box, tombstone},
			script.EventDecayed:    {ghostOfVonHellmann},
		},
		Hooks: script.Hooks{OnEvent: q.onAdvEvent, OnTimer: onTimer, OnTalk: q.onTalk, OnDecayed: q.onDecayed},
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
	case strings.EqualFold(e.Name, "31328-02.htm"):
		st.SetStatus(questlog.StatusStarted)
		st.SetCond(1)
		s.PlaySound(p, script.SoundAccept)
		s.GiveItems(p, forestOfDeadmanMap, 1)
		s.GiveItems(p, silverKey, 1)
	case strings.EqualFold(e.Name, "31328-06.htm"):
		st.SetCond(2)
		s.PlaySound(p, script.SoundMiddle)
	case strings.EqualFold(e.Name, "31526-05.htm"):
		if !p.HasItem(lidiaHairpin) {
			s.GiveItems(p, lidiaHairpin, 1)
			if p.HasItem(lidiaDiary) {
				st.SetCond(4)
				s.PlaySound(p, script.SoundMiddle)
			} else {
				s.PlaySound(p, script.SoundItemGet)
			}
		}
	case strings.EqualFold(e.Name, "31526-11.htm"):
		if !p.HasItem(lidiaDiary) {
			s.GiveItems(p, lidiaDiary, 1)
			if p.HasItem(lidiaHairpin) {
				st.SetCond(4)
				s.PlaySound(p, script.SoundMiddle)
			} else {
				s.PlaySound(p, script.SoundItemGet)
			}
		}
	case strings.EqualFold(e.Name, "31328-11.htm"):
		if st.Cond() < 5 {
			st.SetCond(5)
			s.PlaySound(p, script.SoundMiddle)
		}
	case strings.EqualFold(e.Name, "31328-19.htm"):
		st.SetCond(6)
		s.PlaySound(p, script.SoundMiddle)
	case strings.EqualFold(e.Name, "31524-04.htm"):
		st.SetCond(7)
		s.PlaySound(p, script.SoundMiddle)
		s.TakeItems(p, lidiaDiary, 1)
	case strings.EqualFold(e.Name, "31523-02.htm"):
		if q.ghost.Empty() {
			ghost := s.Spawner().AddSpawn(ghostOfVonHellmann, ghostSpawn(), false, 300000*time.Millisecond)
			q.ghost.Set(ghost)
			ghost.Say("Who awoke me?")
		} else {
			htmltext = "31523-03.htm"
		}
	case strings.EqualFold(e.Name, "31523-05.htm"):
		s.StartTimer("tomb_digger", nil, p, 10000*time.Millisecond)
	case strings.EqualFold(e.Name, "31530-02.htm"):
		st.SetCond(10)
		s.PlaySound(p, script.SoundMiddle)
		s.TakeItems(p, silverKey, 1)
		s.GiveItems(p, silverSpear, 1)
	}
	return htmltext
}

// onTimer ends the digging at the tombstone: the silver key is found.
func onTimer(s *script.Script, e script.Timer) string {
	if strings.EqualFold(e.Name, "tomb_digger") {
		if st := s.QuestState(e.Player, questName); st != nil {
			st.SetCond(8)
			s.PlaySound(e.Player, script.SoundMiddle)
			s.GiveItems(e.Player, silverKey, 1)
			return "31523-06.htm"
		}
	}
	return ""
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
		if st2 := s.QuestState(p, tragedyInVonHellman); st2 != nil && st2.IsCompleted() {
			if p.Level() >= 64 {
				htmltext = "31328-01.htm"
			} else {
				htmltext = "31328-00a.htm"
			}
		} else {
			htmltext = "31328-00.htm"
		}
	case questlog.StatusStarted:
		cond := st.Cond()
		switch e.NPC.NpcID() {
		case innocentin:
			switch {
			case cond == 1:
				htmltext = "31328-03.htm"
			case cond == 2:
				htmltext = "31328-07.htm"
			case cond == 4:
				htmltext = "31328-08.htm"
			case cond == 5:
				if st.GetInt("diary") == 1 {
					htmltext = "31328-14.htm"
				} else {
					htmltext = "31328-11.htm"
				}
			case cond > 5:
				htmltext = "31328-21.htm"
			}
		case brokenBookshelf:
			switch {
			case cond == 2:
				htmltext = "31526-00.htm"
				st.SetCond(3)
				s.PlaySound(p, script.SoundMiddle)
			case cond == 3:
				switch {
				case !p.HasItem(lidiaDiary) && !p.HasItem(lidiaHairpin):
					htmltext = "31526-02.htm"
				case !p.HasItem(lidiaDiary):
					htmltext = "31526-06.htm"
				case !p.HasItem(lidiaHairpin):
					htmltext = "31526-12.htm"
				}
			case cond > 3:
				htmltext = "31526-13.htm"
			}
		case ghostOfVonHellmann:
			switch cond {
			case 6:
				htmltext = "31524-01.htm"
			case 7:
				htmltext = "31524-05.htm"
			case 8:
				htmltext = "31524-06.htm"
			}
		case tombstone:
			switch cond {
			case 6:
				if q.ghost.Empty() {
					htmltext = "31523-01.htm"
				} else {
					htmltext = "31523-03.htm"
				}
			case 7:
				htmltext = "31523-04.htm"
			case 8:
				htmltext = "31523-06.htm"
			}
		case violet:
			switch cond {
			case 8:
				htmltext = "31386-01.htm"
				st.SetCond(9)
				s.PlaySound(p, script.SoundMiddle)
			case 9:
				htmltext = "31386-02.htm"
			case 10:
				if p.HasItem(silverSpear) {
					htmltext = "31386-03.htm"
					s.TakeItems(p, silverSpear, 1)
					s.RewardItems(p, 57, 100000)
					s.PlaySound(p, script.SoundFinish)
					st.Exit(false)
				} else {
					htmltext = "31386-02.htm"
					st.SetCond(9)
				}
			}
		case box:
			switch cond {
			case 9:
				htmltext = "31530-01.htm"
			case 10:
				htmltext = "31530-03.htm"
			}
		}
	case questlog.StatusCompleted:
		if e.NPC.NpcID() == violet {
			htmltext = "31386-04.htm"
		} else {
			htmltext = script.AlreadyCompletedMsg()
		}
	}
	return htmltext
}

// onDecayed forgets the woken ghost once it decays; another ghost of the
// same template stays held.
func (q quest) onDecayed(_ *script.Script, e script.Decayed) {
	q.ghost.ClearIf(e.NPC)
}
