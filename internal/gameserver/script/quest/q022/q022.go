// Package q022 is the quest Tragedy in von Hellmann Forest: Tifaren sends
// the player after an elf's lost skull for the ghost of a priest; Innocentin
// and the ghost of an adventurer then send it to the well, whose soul guards
// the sealed report box.
package q022

import (
	"strings"
	"time"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/questlog"
	"github.com/fatal10110/acis_golang/internal/gameserver/script"
)

const questName = "Q022_TragedyInVonHellmannForest"

// hiddenTruth is the quest Tragedy in von Hellmann Forest follows.
const hiddenTruth = "Q021_HiddenTruth"

// NPCs.
const (
	well              = 31527
	tifaren           = 31334
	innocentin        = 31328
	ghostOfPriest     = 31528
	ghostOfAdventurer = 31529
)

// Items.
const (
	crossOfEinhasad        = 7141
	lostSkullOfElf         = 7142
	letterOfInnocentin     = 7143
	greenJewelOfAdventurer = 7144
	redJewelOfAdventurer   = 7145
	sealedReportBox        = 7146
	reportBox              = 7147
)

// Monster.
const soulOfWell = 27217

// NpcStrings.
const (
	sayGhostCalled = 2250
	sayGhostLeaves = 2251
)

// ghostSpawn is where Tifaren calls the ghost of the priest.
func ghostSpawn() script.Loc { return script.Loc{X: 38418, Y: -49894, Z: -1104, Heading: 0} }

// soulSpawn is where the well wakes its soul.
func soulSpawn() script.Loc { return script.Loc{X: 34860, Y: -54542, Z: -2048, Heading: 0} }

// quest holds the ghost of the priest and the soul of the well, shared by
// every player: one of each at a time.
type quest struct{ ghost, soul *script.NPCCell }

// New returns the quest.
func New() script.Script {
	q := quest{ghost: new(script.NPCCell), soul: new(script.NPCCell)}
	return script.Script{
		Title:   "Tragedy in von Hellmann Forest",
		QuestID: 22,
		Items:   []int32{lostSkullOfElf, reportBox, sealedReportBox, letterOfInnocentin, redJewelOfAdventurer, greenJewelOfAdventurer},
		Bind: script.Bindings{
			script.EventQuestStart: {tifaren, innocentin},
			script.EventTalked:     {innocentin, tifaren, ghostOfPriest, ghostOfAdventurer, well},
			script.EventAttacked:   {soulOfWell},
			script.EventDecayed:    {ghostOfPriest},
			script.EventMyDying:    {soulOfWell, 21553, 21554, 21555, 21556, 21561},
		},
		Hooks: script.Hooks{
			OnEvent: q.onAdvEvent, OnTimer: q.onTimer, OnTalk: q.onTalk,
			OnAttacked: q.onAttacked, OnDecayed: q.onDecayed, OnMyDying: q.onMyDying,
		},
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
	case strings.EqualFold(e.Name, "31334-03.htm"):
		if st2 := s.QuestState(p, hiddenTruth); st2 != nil && st2.IsCompleted() && p.Level() >= 63 {
			htmltext = "31334-02.htm"
		}
	case strings.EqualFold(e.Name, "31334-04.htm"):
		st.SetStatus(questlog.StatusStarted)
		st.SetCond(1)
		s.PlaySound(p, script.SoundAccept)
	case strings.EqualFold(e.Name, "31334-07.htm"):
		if !p.HasItem(crossOfEinhasad) {
			st.SetCond(2)
		} else {
			htmltext = "31334-06.htm"
		}
	case strings.EqualFold(e.Name, "31334-08.htm"):
		if p.HasItem(crossOfEinhasad) {
			st.SetCond(4)
			s.PlaySound(p, script.SoundMiddle)
			s.TakeItems(p, crossOfEinhasad, 1)
		} else {
			st.SetCond(2)
			htmltext = "31334-07.htm"
		}
	case strings.EqualFold(e.Name, "31334-13.htm"):
		if !q.ghost.Empty() {
			st.SetCond(6)
			htmltext = "31334-14.htm"
		} else {
			st.SetCond(7)
			s.PlaySound(p, script.SoundMiddle)
			s.TakeItems(p, lostSkullOfElf, 1)
			ghost := s.Spawner().AddSpawn(ghostOfPriest, ghostSpawn(), false, 120000*time.Millisecond)
			q.ghost.Set(ghost)
			ghost.Say(script.NpcString(sayGhostCalled, p.Name()))
		}
	case strings.EqualFold(e.Name, "31528-08.htm"):
		st.SetCond(8)
		s.PlaySound(p, script.SoundMiddle)
		if ghost := q.ghost.Get(); ghost != nil {
			s.StartTimer("ghost_delete", ghost, nil, 3000*time.Millisecond)
		}
	case strings.EqualFold(e.Name, "31328-10.htm"):
		st.SetCond(9)
		s.PlaySound(p, script.SoundMiddle)
		s.GiveItems(p, letterOfInnocentin, 1)
	case strings.EqualFold(e.Name, "31529-12.htm"):
		st.SetCond(10)
		s.PlaySound(p, script.SoundMiddle)
		s.TakeItems(p, letterOfInnocentin, 1)
		s.GiveItems(p, greenJewelOfAdventurer, 1)
	case strings.EqualFold(e.Name, "31527-02.htm"):
		if q.soul.Empty() {
			soul := s.Spawner().AddSpawn(soulOfWell, soulSpawn(), false, 0)
			q.soul.Set(soul)
			soul.AddAttackDesire(p, 2000)
		}
	case strings.EqualFold(e.Name, "31328-13.htm"):
		st.SetCond(15)
		s.PlaySound(p, script.SoundMiddle)
		s.TakeItems(p, reportBox, 1)
	case strings.EqualFold(e.Name, "31328-21.htm"):
		st.SetCond(16)
		s.PlaySound(p, script.SoundMiddle)
	}
	return htmltext
}

// onTimer takes the ghost of the priest away three seconds after its last
// word, and swaps the green jewel for the red one once the player has held
// the soul of the well in a fight for twenty seconds.
func (q quest) onTimer(s *script.Script, e script.Timer) string {
	switch {
	case strings.EqualFold(e.Name, "ghost_delete"):
		q.ghost.Get().DeleteMe()
	case strings.EqualFold(e.Name, "attack_timer"):
		if st := s.QuestState(e.Player, questName); st != nil {
			st.SetCond(11)
			s.PlaySound(e.Player, script.SoundMiddle)
			s.TakeItems(e.Player, greenJewelOfAdventurer, 1)
			s.GiveItems(e.Player, redJewelOfAdventurer, 1)
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
		switch e.NPC.NpcID() {
		case innocentin:
			if st2 := s.QuestState(p, hiddenTruth); st2 != nil && st2.IsCompleted() {
				if !p.HasItem(crossOfEinhasad) {
					htmltext = "31328-01.htm"
					s.GiveItems(p, crossOfEinhasad, 1)
					s.PlaySound(p, script.SoundItemGet)
				} else {
					htmltext = "31328-01b.htm"
				}
			}
		case tifaren:
			htmltext = "31334-01.htm"
		}
	case questlog.StatusStarted:
		cond := st.Cond()
		switch e.NPC.NpcID() {
		case tifaren:
			switch {
			case cond == 1 || cond == 2 || cond == 3:
				htmltext = "31334-05.htm"
			case cond == 4:
				htmltext = "31334-09.htm"
			case cond == 5 || cond == 6:
				if p.HasItem(lostSkullOfElf) {
					if q.ghost.Empty() {
						htmltext = "31334-10.htm"
					} else {
						htmltext = "31334-11.htm"
					}
				} else {
					htmltext = "31334-09.htm"
					st.SetCond(4)
				}
			case cond == 7:
				if !q.ghost.Empty() {
					htmltext = "31334-15.htm"
				} else {
					htmltext = "31334-17.htm"
				}
			case cond > 7:
				htmltext = "31334-18.htm"
			}
		case innocentin:
			switch {
			case cond < 3:
				if !p.HasItem(crossOfEinhasad) {
					htmltext = "31328-01.htm"
					st.SetCond(3)
					s.PlaySound(p, script.SoundItemGet)
					s.GiveItems(p, crossOfEinhasad, 1)
				} else {
					htmltext = "31328-01b.htm"
				}
			case cond == 3:
				htmltext = "31328-02.htm"
			case cond == 8:
				htmltext = "31328-03.htm"
			case cond == 9:
				htmltext = "31328-11.htm"
			case cond == 14:
				if p.HasItem(reportBox) {
					htmltext = "31328-12.htm"
				} else {
					st.SetCond(13)
				}
			case cond == 15:
				htmltext = "31328-14.htm"
			case cond == 16:
				if p.Level() < 64 {
					htmltext = "31328-23.htm"
				} else {
					htmltext = "31328-22.htm"
				}
				st.Exit(false)
				s.PlaySound(p, script.SoundFinish)
			}
		case ghostOfPriest:
			switch cond {
			case 7:
				htmltext = "31528-01.htm"
			case 8:
				htmltext = "31528-08.htm"
			}
		case ghostOfAdventurer:
			switch {
			case cond == 9:
				if p.HasItem(letterOfInnocentin) {
					htmltext = "31529-01.htm"
				} else {
					htmltext = "31529-10.htm"
					st.SetCond(8)
				}
			case cond == 10:
				htmltext = "31529-16.htm"
			case cond == 11:
				if p.HasItem(redJewelOfAdventurer) {
					htmltext = "31529-17.htm"
					st.SetCond(12)
					s.PlaySound(p, script.SoundMiddle)
					s.TakeItems(p, redJewelOfAdventurer, 1)
				} else {
					htmltext = "31529-09.htm"
					st.SetCond(10)
				}
			case cond == 12:
				htmltext = "31529-17.htm"
			case cond == 13:
				if p.HasItem(sealedReportBox) {
					htmltext = "31529-18.htm"
					st.SetCond(14)
					s.PlaySound(p, script.SoundMiddle)
					s.TakeItems(p, sealedReportBox, 1)
					s.GiveItems(p, reportBox, 1)
				} else {
					htmltext = "31529-10.htm"
					st.SetCond(12)
				}
			case cond > 13:
				htmltext = "31529-19.htm"
			}
		case well:
			switch {
			case cond == 10:
				htmltext = "31527-01.htm"
			case cond == 11:
				htmltext = "31527-03.htm"
			case cond == 12:
				htmltext = "31527-04.htm"
				st.SetCond(13)
				s.PlaySound(p, script.SoundMiddle)
				s.GiveItems(p, sealedReportBox, 1)
			case cond > 12:
				htmltext = "31527-05.htm"
			}
		}
	case questlog.StatusCompleted:
		htmltext = script.AlreadyCompletedMsg()
	}
	return htmltext
}

// onAttacked starts the twenty seconds the player, not its summon, must
// hold the soul of the well in a fight at cond 10.
func (q quest) onAttacked(s *script.Script, e script.Attacked) {
	p := script.ActingPlayer(e.Attacker)
	if p == nil {
		return
	}
	st := s.QuestState(p, questName)
	if st == nil || st.Status() != questlog.StatusStarted {
		return
	}
	if e.Attacker.Summon() || !script.Same(e.NPC, q.soul.Get()) {
		return
	}
	if st.Cond() == 10 {
		s.StartTimer("attack_timer", e.NPC, p, 20000*time.Millisecond)
	}
}

// onDecayed sends the ghost of the priest away with its last word and its
// pending departure.
func (q quest) onDecayed(s *script.Script, e script.Decayed) {
	if !q.ghost.ClearIf(e.NPC) {
		return
	}
	e.NPC.Say(script.NpcString(sayGhostLeaves))
	s.CancelTimers(script.TimerNPC(e.NPC))
}

// onMyDying drops an elf's skull at cond 4; any soul of the well the player
// kills frees the well and stops the fight's twenty seconds.
func (q quest) onMyDying(s *script.Script, e script.MyDying) {
	p := script.ActingPlayer(e.Killer)
	st := s.CheckPlayerState(p, e.NPC, questlog.StatusStarted)
	if st == nil {
		return
	}
	if e.NPC.NpcID() != soulOfWell {
		if st.Cond() == 4 && s.DropItems(p, lostSkullOfElf, 1, 1, 100000) {
			st.SetCond(5)
		}
	} else {
		s.CancelTimers(script.TimerName("attack_timer"), script.TimerNPC(q.soul.Get()))
		q.soul.Clear()
	}
}
