// Package q246 is the quest Possessor of a Precious Soul - 3: on a
// subclass of level 65 or more, with Caradine's first letter, the player
// gathers the waterbinder and the evergreen, and the rain song from
// Barakiel, for Ossian's relic box, which Ladd trades for Caradine's second
// letter.
package q246

import (
	"strings"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/questlog"
	"github.com/fatal10110/acis_golang/internal/gameserver/script"
)

const questName = "Q246_PossessorOfAPreciousSoul"

// NPCs.
const (
	caradine = 31740
	ossian   = 31741
	ladd     = 30721
)

// Items.
const (
	waterbinder     = 7591
	evergreen       = 7592
	rainSong        = 7593
	relicBox        = 7594
	caradineLetter1 = 7678
	caradineLetter2 = 7679
)

// Monsters.
const (
	pilgrimOfSplendor = 21541
	judgeOfSplendor   = 21544
	barakiel          = 25325
)

// New returns the quest.
func New() script.Script {
	return script.Script{
		Title:   "Possessor of a Precious Soul - 3",
		QuestID: 246,
		Items:   []int32{waterbinder, evergreen, rainSong, relicBox},
		Bind: script.Bindings{
			script.EventQuestStart: {caradine},
			script.EventTalked:     {caradine, ossian, ladd},
			script.EventMyDying:    {pilgrimOfSplendor, judgeOfSplendor, barakiel},
		},
		Hooks: script.Hooks{OnEvent: onAdvEvent, OnTalk: onTalk, OnMyDying: onMyDying},
	}
}

func onAdvEvent(s *script.Script, e script.Event) string {
	htmltext := e.Name
	p := e.Player
	st := s.QuestState(p, questName)
	if st == nil {
		return htmltext
	}
	switch {
	// Caradine.
	case strings.EqualFold(e.Name, "31740-04.htm"):
		st.SetStatus(questlog.StatusStarted)
		st.SetCond(1)
		s.PlaySound(p, script.SoundAccept)
		s.TakeItems(p, caradineLetter1, 1)
	// Ossian.
	case strings.EqualFold(e.Name, "31741-02.htm"):
		st.SetCond(2)
		s.PlaySound(p, script.SoundMiddle)
	case strings.EqualFold(e.Name, "31741-05.htm"):
		if p.HasItems(waterbinder, evergreen) {
			st.SetCond(4)
			s.PlaySound(p, script.SoundMiddle)
			s.TakeItems(p, waterbinder, 1)
			s.TakeItems(p, evergreen, 1)
		} else {
			htmltext = ""
		}
	case strings.EqualFold(e.Name, "31741-08.htm"):
		if p.HasItem(rainSong) {
			st.SetCond(6)
			s.PlaySound(p, script.SoundMiddle)
			s.TakeItems(p, rainSong, 1)
			s.GiveItems(p, relicBox, 1)
		} else {
			htmltext = ""
		}
	// Ladd.
	case strings.EqualFold(e.Name, "30721-02.htm"):
		if p.HasItem(relicBox) {
			s.TakeItems(p, relicBox, 1)
			s.GiveItems(p, caradineLetter2, 1)
			s.RewardExpAndSp(p, 719843, 0)
			p.SocialAction(3)
			s.PlaySound(p, script.SoundFinish)
			st.Exit(false)
		} else {
			htmltext = ""
		}
	}
	return htmltext
}

func onTalk(s *script.Script, e script.Talk) string {
	htmltext := script.NoQuestMsg()
	p := e.Player
	st := s.QuestState(p, questName)
	if st == nil {
		return htmltext
	}
	switch st.Status() {
	case questlog.StatusCreated:
		if p.HasItem(caradineLetter1) {
			if !p.SubClassActive() || p.Level() < 65 {
				htmltext = "31740-02.htm"
			} else {
				htmltext = "31740-01.htm"
			}
		}
	case questlog.StatusStarted:
		if !p.SubClassActive() {
			break
		}
		cond := st.Cond()
		switch e.NPC.NpcID() {
		case caradine:
			if cond == 1 {
				htmltext = "31740-05.htm"
			}
		case ossian:
			switch {
			case cond == 1:
				htmltext = "31741-01.htm"
			case cond == 2:
				htmltext = "31741-03.htm"
			case cond == 3:
				if p.HasItems(waterbinder, evergreen) {
					htmltext = "31741-04.htm"
				}
			case cond == 4:
				htmltext = "31741-06.htm"
			case cond == 5:
				if p.HasItem(rainSong) {
					htmltext = "31741-07.htm"
				}
			case cond == 6:
				htmltext = "31741-09.htm"
			}
		case ladd:
			if cond == 6 && p.HasItem(relicBox) {
				htmltext = "30721-01.htm"
			}
		}
	case questlog.StatusCompleted:
		htmltext = script.AlreadyCompletedMsg()
	}
	return htmltext
}

func onMyDying(s *script.Script, e script.MyDying) {
	p := script.ActingPlayer(e.Killer)
	npcID := e.NPC.NpcID()

	if npcID == barakiel {
		for _, st := range s.PartyMembers(p, e.NPC, script.ByCond(4)) {
			pm := st.Player()
			if !pm.SubClassActive() {
				continue
			}
			if !pm.HasItem(rainSong) {
				st.SetCond(5)
				s.PlaySound(pm, script.SoundMiddle)
				s.GiveItems(pm, rainSong, 1)
			}
		}
		return
	}

	if p == nil || !p.SubClassActive() {
		return
	}
	st := s.CheckPlayerCondition(p, e.NPC, 2)
	if st == nil {
		return
	}
	if s.Rnd(10) < 2 {
		necklaceOrRing, other := int32(evergreen), int32(waterbinder)
		if npcID == pilgrimOfSplendor {
			necklaceOrRing, other = waterbinder, evergreen
		}
		if !p.HasItem(necklaceOrRing) {
			s.GiveItems(p, necklaceOrRing, 1)
			if !p.HasItem(other) {
				s.PlaySound(p, script.SoundItemGet)
			} else {
				st.SetCond(3)
				s.PlaySound(p, script.SoundMiddle)
			}
		}
	}
}
