// Package q005 is the quest Miner's Favor: Bolter's list of things to
// fetch from Shari, Garita, Reed and Brunon, for a necklace.
package q005

import (
	"strings"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/questlog"
	"github.com/fatal10110/acis_golang/internal/gameserver/script"
)

const questName = "Q005_MinersFavor"

// NPCs.
const (
	bolter = 30554
	shari  = 30517
	garita = 30518
	reed   = 30520
	brunon = 30526
)

// Items.
const (
	boltersList        = 1547
	miningBoots        = 1548
	minersPick         = 1549
	boomboomPowder     = 1550
	redstoneBeer       = 1551
	boltersSmellySocks = 1552
)

// Reward.
const necklace = 906

// New returns the quest.
func New() script.Script {
	return script.Script{
		Title:   "Miner's Favor",
		QuestID: 5,
		Items:   []int32{boltersList, miningBoots, minersPick, boomboomPowder, redstoneBeer, boltersSmellySocks},
		Bind: script.Bindings{
			script.EventQuestStart: {bolter},
			script.EventTalked:     {bolter, shari, garita, reed, brunon},
		},
		Hooks: script.Hooks{OnEvent: onAdvEvent, OnTalk: onTalk},
	}
}

func onAdvEvent(s *script.Script, e script.Event) string {
	htmltext := e.Name
	st := s.QuestState(e.Player, questName)
	if st == nil {
		return htmltext
	}
	switch {
	case strings.EqualFold(e.Name, "30554-03.htm"):
		st.SetStatus(questlog.StatusStarted)
		st.SetCond(1)
		s.PlaySound(e.Player, script.SoundAccept)
		s.GiveItems(e.Player, boltersList, 1)
		s.GiveItems(e.Player, boltersSmellySocks, 1)
	case strings.EqualFold(e.Name, "30526-02.htm"):
		s.TakeItems(e.Player, boltersSmellySocks, 1)
		s.GiveItems(e.Player, minersPick, 1)
		if e.Player.HasItems(miningBoots, boomboomPowder, redstoneBeer) {
			st.SetCond(2)
			s.PlaySound(e.Player, script.SoundMiddle)
		} else {
			s.PlaySound(e.Player, script.SoundItemGet)
		}
	}
	return htmltext
}

func onTalk(s *script.Script, e script.Talk) string {
	st := s.QuestState(e.Player, questName)
	htmltext := script.NoQuestMsg()
	if st == nil {
		return htmltext
	}
	switch st.Status() {
	case questlog.StatusCreated:
		if e.Player.Level() < 2 {
			htmltext = "30554-01.htm"
		} else {
			htmltext = "30554-02.htm"
		}
	case questlog.StatusStarted:
		cond := st.Cond()
		switch e.NPC.NpcID() {
		case bolter:
			switch cond {
			case 1:
				htmltext = "30554-04.htm"
			case 2:
				htmltext = "30554-06.htm"
				s.TakeItems(e.Player, boltersList, 1)
				s.TakeItems(e.Player, boomboomPowder, 1)
				s.TakeItems(e.Player, minersPick, 1)
				s.TakeItems(e.Player, miningBoots, 1)
				s.TakeItems(e.Player, redstoneBeer, 1)
				s.GiveItems(e.Player, necklace, 1)
				s.PlaySound(e.Player, script.SoundFinish)
				st.Exit(false)
			}
		case shari:
			if cond == 1 && !e.Player.HasItem(boomboomPowder) {
				htmltext = "30517-01.htm"
				s.GiveItems(e.Player, boomboomPowder, 1)
				if e.Player.HasItems(miningBoots, minersPick, redstoneBeer) {
					st.SetCond(2)
					s.PlaySound(e.Player, script.SoundMiddle)
				} else {
					s.PlaySound(e.Player, script.SoundItemGet)
				}
			} else {
				htmltext = "30517-02.htm"
			}
		case garita:
			if cond == 1 && !e.Player.HasItem(miningBoots) {
				htmltext = "30518-01.htm"
				s.GiveItems(e.Player, miningBoots, 1)
				if e.Player.HasItems(minersPick, boomboomPowder, redstoneBeer) {
					st.SetCond(2)
					s.PlaySound(e.Player, script.SoundMiddle)
				} else {
					s.PlaySound(e.Player, script.SoundItemGet)
				}
			} else {
				htmltext = "30518-02.htm"
			}
		case reed:
			if cond == 1 && !e.Player.HasItem(redstoneBeer) {
				htmltext = "30520-01.htm"
				s.GiveItems(e.Player, redstoneBeer, 1)
				if e.Player.HasItems(miningBoots, minersPick, boomboomPowder) {
					st.SetCond(2)
					s.PlaySound(e.Player, script.SoundMiddle)
				} else {
					s.PlaySound(e.Player, script.SoundItemGet)
				}
			} else {
				htmltext = "30520-02.htm"
			}
		case brunon:
			if cond == 1 && !e.Player.HasItem(minersPick) {
				htmltext = "30526-01.htm"
			} else {
				htmltext = "30526-03.htm"
			}
		}
	case questlog.StatusCompleted:
		htmltext = script.AlreadyCompletedMsg()
	}
	return htmltext
}
