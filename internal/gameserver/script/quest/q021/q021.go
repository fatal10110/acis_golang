// Package q021 is the quest Hidden Truth: the mysterious wizard sends the
// player to von Hellmann's tombstone, whose duke's ghost calls his page;
// the page leads the player to the broken bookshelf and its truth, and the
// cross of Einhasad it gives goes round the three priests to Innocentin.
package q021

import (
	"strconv"
	"strings"
	"time"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/questlog"
	"github.com/fatal10110/acis_golang/internal/gameserver/script"
)

const questName = "Q021_HiddenTruth"

// NPCs.
const (
	mysteriousWizard = 31522
	tombstone        = 31523
	vonHellmanDuke   = 31524
	vonHellmanPage   = 31525
	brokenBookshelf  = 31526
	agripel          = 31348
	dominic          = 31350
	benedict         = 31349
	innocentin       = 31328
)

// Items.
const (
	crossOfEinhasad          = 7140
	crossOfEinhasadNextQuest = 7141
)

// Sounds.
const (
	soundHorror   = "SkillSound5.horror_02"
	soundItemDrop = "ItemSound.item_drop_equip_armor_cloth"
)

// NpcStrings.
const (
	sayDukeWakes   = 2150
	sayPageGuides  = 2151
	sayPageArrives = 2152
)

// dukeSpawn is where the tombstone wakes the duke's ghost.
func dukeSpawn() script.Loc { return script.Loc{X: 51432, Y: -54570, Z: -3136, Heading: 0} }

// pageSpawn is where the duke calls his page.
func pageSpawn() script.Loc { return script.Loc{X: 51608, Y: -54520, Z: -3168, Heading: 0} }

// quest holds the duke and the page, shared by every player: one of each
// at a time, until it decays.
type quest struct{ duke, page *script.NPCCell }

// New returns the quest.
func New() script.Script {
	q := quest{duke: new(script.NPCCell), page: new(script.NPCCell)}
	return script.Script{
		Title:   "Hidden Truth",
		QuestID: 21,
		Items:   []int32{crossOfEinhasad},
		Bind: script.Bindings{
			script.EventQuestStart: {mysteriousWizard},
			script.EventTalked:     {mysteriousWizard, tombstone, vonHellmanDuke, vonHellmanPage, brokenBookshelf, agripel, dominic, benedict, innocentin},
			script.EventDecayed:    {vonHellmanDuke, vonHellmanPage},
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
	case strings.EqualFold(e.Name, "31522-02.htm"):
		st.SetStatus(questlog.StatusStarted)
		st.SetCond(1)
		s.PlaySound(p, script.SoundAccept)
	case strings.EqualFold(e.Name, "31523-03.htm"):
		if q.duke.Empty() {
			if st.Cond() == 1 {
				st.SetCond(2)
			}
			duke := s.Spawner().AddSpawn(vonHellmanDuke, dukeSpawn(), false, 300000*time.Millisecond)
			q.duke.Set(duke)
			duke.Say(script.NpcString(sayDukeWakes))
		} else {
			htmltext = "31523-04.htm"
		}
		s.PlaySound(p, soundHorror)
	case strings.EqualFold(e.Name, "31524-06.htm"):
		if q.spawnThePage(s, p) {
			st.SetCond(3)
			s.PlaySound(p, script.SoundMiddle)
		} else {
			htmltext = "31524-06a.htm"
		}
	case strings.EqualFold(e.Name, "31526-03.htm"):
		s.PlaySound(p, soundItemDrop)
	case strings.EqualFold(e.Name, "31526-07.htm"):
		st.Set("truth", strconv.Itoa(1))
	case strings.EqualFold(e.Name, "31526-08.htm"):
		st.SetCond(5)
		st.Unset("end_walk")
		st.Unset("truth")
		q.page.Get().DeleteMe()
		s.PlaySound(p, script.SoundMiddle)
	case strings.EqualFold(e.Name, "31526-14.htm"):
		st.SetCond(6)
		s.PlaySound(p, script.SoundMiddle)
		s.GiveItems(p, crossOfEinhasad, 1)
	case strings.EqualFold(e.Name, "31328-05.htm"):
		if p.HasItem(crossOfEinhasad) {
			s.TakeItems(p, crossOfEinhasad, 1)
			s.GiveItems(p, crossOfEinhasadNextQuest, 1)
			s.PlaySound(p, script.SoundFinish)
			st.Exit(false)
		}
	}
	return htmltext
}

// onTimer is the page's walk: half a second after it appears it offers to
// guide the player, and nine seconds later it has led the player to the
// bookcase.
func onTimer(s *script.Script, e script.Timer) string {
	switch {
	case strings.EqualFold(e.Name, "2102"):
		e.NPC.Say(script.NpcString(sayPageGuides, e.Player.Name()))
		s.StartTimer("2103", e.NPC, e.Player, 9000*time.Millisecond)
	case strings.EqualFold(e.Name, "2103"):
		e.NPC.Say(script.NpcString(sayPageArrives, e.Player.Name()))
		if st := s.QuestState(e.Player, questName); st != nil {
			st.Set("end_walk", strconv.Itoa(1))
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
		if p.Level() < 63 {
			htmltext = "31522-03.htm"
		} else {
			htmltext = "31522-01.htm"
		}
	case questlog.StatusStarted:
		cond := st.Cond()
		switch npcID := e.NPC.NpcID(); npcID {
		case mysteriousWizard:
			htmltext = "31522-05.htm"
		case tombstone:
			if cond == 1 || cond == 2 || cond == 3 {
				htmltext = "31523-01.htm"
			}
		case vonHellmanDuke:
			switch {
			case cond == 2:
				htmltext = "31524-01.htm"
			case cond == 3:
				if st.GetInt("end_walk") == 0 {
					if q.spawnThePage(s, p) {
						st.Set("end_walk", strconv.Itoa(0))
						htmltext = "31524-07.htm"
					} else {
						htmltext = "31524-07a.htm"
					}
				} else {
					htmltext = "31524-07b.htm"
					st.SetCond(4)
					s.PlaySound(p, script.SoundMiddle)
				}
			case cond == 4:
				htmltext = "31524-07b.htm"
			case cond > 4:
				htmltext = "31524-07c.htm"
			}
		case vonHellmanPage:
			switch cond {
			case 3:
				if st.GetInt("end_walk") == 1 {
					htmltext = "31525-02.htm"
					st.SetCond(4)
					s.PlaySound(p, script.SoundMiddle)
				} else {
					htmltext = "31525-01.htm"
				}
			case 4:
				htmltext = "31525-02.htm"
			}
		case brokenBookshelf:
			switch {
			case cond == 3 && st.GetInt("end_walk") == 1:
				htmltext = "31526-01.htm"
			case cond == 4:
				if st.GetInt("truth") == 0 {
					htmltext = "31526-01.htm"
				} else {
					htmltext = "31526-10.htm"
					st.SetCond(5)
					st.Unset("end_walk")
					st.Unset("truth")
					q.page.Get().DeleteMe()
					s.PlaySound(p, script.SoundMiddle)
				}
			case cond == 5:
				htmltext = "31526-11.htm"
			case cond == 6:
				htmltext = "31526-15.htm"
			}
		case agripel, benedict, dominic:
			if (cond == 6 || cond == 7) && p.HasItem(crossOfEinhasad) {
				// At cond 6, each priest is marked until the third one moves the
				// quest to cond 7.
				if cond == 6 {
					var npcID1, npcID2 int32
					switch npcID {
					case agripel:
						npcID1, npcID2 = benedict, dominic
					case benedict:
						npcID1, npcID2 = agripel, dominic
					case dominic:
						npcID1, npcID2 = agripel, benedict
					}
					if st.GetInt(strconv.Itoa(int(npcID1))) == 1 && st.GetInt(strconv.Itoa(int(npcID2))) == 1 {
						st.SetCond(7)
						s.PlaySound(p, script.SoundMiddle)
					} else {
						st.Set(strconv.Itoa(int(npcID)), strconv.Itoa(1))
					}
				}
				htmltext = strconv.Itoa(int(npcID)) + "-01.htm"
			}
		case innocentin:
			if cond == 7 && p.HasItem(crossOfEinhasad) {
				htmltext = "31328-01.htm"
			}
		}
	case questlog.StatusCompleted:
		if e.NPC.NpcID() == innocentin {
			htmltext = "31328-06.htm"
		} else {
			htmltext = script.AlreadyCompletedMsg()
		}
	}
	return htmltext
}

// onDecayed forgets the duke or the page once it decays, the page with its
// pending walk; another NPC of the same template stays held.
func (q quest) onDecayed(s *script.Script, e script.Decayed) {
	if q.duke.ClearIf(e.NPC) {
		return
	}
	if q.page.ClearIf(e.NPC) {
		s.CancelTimers(script.TimerNPC(e.NPC))
	}
}

// spawnThePage calls the page when none stands, starting its walk with p,
// and reports whether it did.
func (q quest) spawnThePage(s *script.Script, p *script.Player) bool {
	if !q.page.Empty() {
		return false
	}
	page := s.Spawner().AddSpawn(vonHellmanPage, pageSpawn(), false, 90000*time.Millisecond)
	q.page.Set(page)
	s.StartTimer("2102", page, p, 500*time.Millisecond)
	return true
}
