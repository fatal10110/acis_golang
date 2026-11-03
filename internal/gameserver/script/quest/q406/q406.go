// Package q406 is the quest Path to an Elven Knight: an elven fighter earns
// the Elven Knight Brooch by collecting topaz pieces from the skeletons for
// Sorius and emerald pieces from the Ol Mahum novices for Kluto.
package q406

import (
	"strings"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/questlog"
	"github.com/fatal10110/acis_golang/internal/gameserver/script"
)

const questName = "Q406_PathToAnElvenKnight"

// Quest items.
const (
	soriusLetter      = 1202
	klutoBox          = 1203
	elvenKnightBrooch = 1204
	topazPiece        = 1205
	emeraldPiece      = 1206
	klutoMemo         = 1276
)

// Npcs.
const (
	sorius = 30327
	kluto  = 30317
)

// Monsters.
const (
	trackerSkeleton       = 20035
	trackerSkeletonLeader = 20042
	skeletonScout         = 20045
	skeletonBowman        = 20051
	ruinSpartoi           = 20054
	ragingSpartoi         = 20060
	olMahumNovice         = 20782
)

// The classes the quest checks.
const (
	classElvenFighter = 18
	classElvenKnight  = 19
)

// New returns the quest.
func New() script.Script {
	return script.Script{
		Title:   "Path to an Elven Knight",
		QuestID: 406,
		Items:   []int32{soriusLetter, klutoBox, topazPiece, emeraldPiece, klutoMemo},
		Bind: script.Bindings{
			script.EventQuestStart: {sorius},
			script.EventTalked:     {sorius, kluto},
			script.EventMyDying: {
				trackerSkeleton, trackerSkeletonLeader, skeletonScout, skeletonBowman, ruinSpartoi, ragingSpartoi, olMahumNovice,
			},
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
	case strings.EqualFold(e.Name, "30327-05.htm"):
		switch {
		case p.ClassID() != classElvenFighter:
			if p.ClassID() == classElvenKnight {
				htmltext = "30327-02a.htm"
			} else {
				htmltext = "30327-02.htm"
			}
		case p.Level() < 19:
			htmltext = "30327-03.htm"
		case p.HasItem(elvenKnightBrooch):
			htmltext = "30327-04.htm"
		}
	case strings.EqualFold(e.Name, "30327-06.htm"):
		st.SetStatus(questlog.StatusStarted)
		st.SetCond(1)
		s.PlaySound(p, script.SoundAccept)
	case strings.EqualFold(e.Name, "30317-02.htm"):
		st.SetCond(4)
		s.PlaySound(p, script.SoundMiddle)
		s.TakeItems(p, soriusLetter, 1)
		s.GiveItems(p, klutoMemo, 1)
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
		htmltext = "30327-01.htm"
	case questlog.StatusStarted:
		cond := st.Cond()
		switch e.NPC.NpcID() {
		case sorius:
			switch {
			case cond == 1:
				if !p.HasItem(topazPiece) {
					htmltext = "30327-07.htm"
				} else {
					htmltext = "30327-08.htm"
				}
			case cond == 2:
				htmltext = "30327-09.htm"
				st.SetCond(3)
				s.PlaySound(p, script.SoundMiddle)
				s.GiveItems(p, soriusLetter, 1)
			case cond > 2 && cond < 6:
				htmltext = "30327-11.htm"
			case cond == 6:
				htmltext = "30327-10.htm"
				s.TakeItems(p, klutoBox, 1)
				s.TakeItems(p, klutoMemo, 1)
				s.GiveItems(p, elvenKnightBrooch, 1)
				s.RewardExpAndSp(p, 3200, 2280)
				p.SocialAction(3)
				s.PlaySound(p, script.SoundFinish)
				st.Exit(true)
			}
		case kluto:
			switch {
			case cond == 3:
				htmltext = "30317-01.htm"
			case cond == 4:
				if !p.HasItem(emeraldPiece) {
					htmltext = "30317-03.htm"
				} else {
					htmltext = "30317-04.htm"
				}
			case cond == 5:
				htmltext = "30317-05.htm"
				st.SetCond(6)
				s.PlaySound(p, script.SoundMiddle)
				s.TakeItems(p, emeraldPiece, -1)
				s.TakeItems(p, topazPiece, -1)
				s.GiveItems(p, klutoBox, 1)
			case cond == 6:
				htmltext = "30317-06.htm"
			}
		}
	}
	return htmltext
}

func onMyDying(s *script.Script, e script.MyDying) {
	p := script.ActingPlayer(e.Killer)
	st := s.CheckPlayerState(p, e.NPC, questlog.StatusStarted)
	if st == nil {
		return
	}
	switch e.NPC.NpcID() {
	case trackerSkeleton, trackerSkeletonLeader, skeletonScout, skeletonBowman, ruinSpartoi, ragingSpartoi:
		if st.Cond() == 1 && s.DropItems(p, topazPiece, 1, 20, 700000) {
			st.SetCond(2)
		}
	case olMahumNovice:
		if st.Cond() == 4 && s.DropItems(p, emeraldPiece, 1, 20, 500000) {
			st.SetCond(5)
		}
	}
}
