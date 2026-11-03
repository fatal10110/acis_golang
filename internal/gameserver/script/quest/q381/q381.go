// Package q381 is the quest Let's Become a Royal Member: Sorint wants a
// coin of Kail and a coin album, which Sandra trades for a golden clover
// coin.
package q381

import (
	"strconv"
	"strings"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/questlog"
	"github.com/fatal10110/acis_golang/internal/gameserver/script"
)

const questName = "Q381_LetsBecomeARoyalMember"

// Npcs.
const (
	sorint = 30232
	sandra = 30090
)

// Quest items.
const (
	kailCoin                = 5899
	coinAlbum               = 5900
	goldenCloverCoin        = 7569
	coinCollectorMembership = 3813
)

// Reward.
const royalMembership = 5898

// New returns the quest.
func New() script.Script {
	return script.Script{
		Title:   "Lets Become a Royal Member!",
		QuestID: 381,
		Items:   []int32{kailCoin, goldenCloverCoin},
		Bind: script.Bindings{
			script.EventQuestStart: {sorint},
			script.EventTalked:     {sorint, sandra},
			script.EventMyDying:    {21018, 27316},
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
	switch {
	case strings.EqualFold(e.Name, "30090-02.htm"):
		st.Set("aCond", strconv.Itoa(1)) // Alternative cond used for Sandra.
	case strings.EqualFold(e.Name, "30232-03.htm"):
		st.SetStatus(questlog.StatusStarted)
		st.SetCond(1)
		s.PlaySound(e.Player, script.SoundAccept)
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
		if p.Level() < 55 || !p.HasItem(coinCollectorMembership) {
			htmltext = "30232-02.htm"
		} else {
			htmltext = "30232-01.htm"
		}
	case questlog.StatusStarted:
		switch e.NPC.NpcID() {
		case sorint:
			switch {
			case !p.HasItem(kailCoin):
				htmltext = "30232-04.htm"
			case !p.HasItem(coinAlbum):
				htmltext = "30232-05.htm"
			default:
				htmltext = "30232-06.htm"
				s.TakeItems(p, kailCoin, -1)
				s.TakeItems(p, coinAlbum, -1)
				s.GiveItems(p, royalMembership, 1)
				s.PlaySound(p, script.SoundFinish)
				st.Exit(true)
			}
		case sandra:
			if !p.HasItem(coinAlbum) {
				if st.GetInt("aCond") == 0 {
					htmltext = "30090-01.htm"
				} else if !p.HasItem(goldenCloverCoin) {
					htmltext = "30090-03.htm"
				} else {
					htmltext = "30090-04.htm"
					s.TakeItems(p, goldenCloverCoin, -1)
					s.GiveItems(p, coinAlbum, 1)
				}
			} else {
				htmltext = "30090-05.htm"
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
	if e.NPC.NpcID() == 21018 {
		s.DropItems(p, kailCoin, 1, 1, 50000)
	} else if st.GetInt("aCond") == 1 {
		s.DropItemsAlways(p, goldenCloverCoin, 1, 1)
	}
}
