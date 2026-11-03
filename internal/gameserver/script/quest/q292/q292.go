// Package q292 is the quest Brigands Sweep: Spiron pays a dwarf for the
// necklaces and pendants of the goblin brigands, and Balanki or Spiron for
// the suspicious contract pieced together from their memos.
package q292

import (
	"strings"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/player"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/questlog"
	"github.com/fatal10110/acis_golang/internal/gameserver/script"
)

const questName = "Q292_BrigandsSweep"

// NPCs.
const (
	spiron  = 30532
	balanki = 30533
)

// Items.
const (
	goblinNecklace     = 1483
	goblinPendant      = 1484
	goblinLordPendant  = 1485
	suspiciousMemo     = 1486
	suspiciousContract = 1487
)

// Monsters.
const (
	goblinBrigand           = 20322
	goblinBrigandLeader     = 20323
	goblinBrigandLieutenant = 20324
	goblinSnooper           = 20327
	goblinLord              = 20528
)

// New returns the quest.
func New() script.Script {
	return script.Script{
		Title:   "Brigands Sweep",
		QuestID: 292,
		Items:   []int32{goblinNecklace, goblinPendant, goblinLordPendant, suspiciousMemo, suspiciousContract},
		Bind: script.Bindings{
			script.EventQuestStart: {spiron},
			script.EventTalked:     {spiron, balanki},
			script.EventMyDying:    {goblinBrigand, goblinBrigandLeader, goblinBrigandLieutenant, goblinSnooper, goblinLord},
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
	case strings.EqualFold(e.Name, "30532-03.htm"):
		st.SetStatus(questlog.StatusStarted)
		st.SetCond(1)
		s.PlaySound(e.Player, script.SoundAccept)
	case strings.EqualFold(e.Name, "30532-06.htm"):
		s.PlaySound(e.Player, script.SoundFinish)
		st.Exit(true)
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
		switch {
		case p.Race() != player.RaceDwarf:
			htmltext = "30532-00.htm"
		case p.Level() < 5:
			htmltext = "30532-01.htm"
		default:
			htmltext = "30532-02.htm"
		}
	case questlog.StatusStarted:
		switch e.NPC.NpcID() {
		case spiron:
			necklaces := p.ItemCount(goblinNecklace)
			pendants := p.ItemCount(goblinPendant)
			lordPendants := p.ItemCount(goblinLordPendant)
			memos := p.ItemCount(suspiciousMemo)
			countAll := necklaces + pendants + lordPendants
			hasContract := p.HasItem(suspiciousContract)
			if countAll == 0 {
				htmltext = "30532-04.htm"
				break
			}
			switch {
			case hasContract:
				htmltext = "30532-10.htm"
			case memos > 0:
				if memos > 1 {
					htmltext = "30532-09.htm"
				} else {
					htmltext = "30532-08.htm"
				}
			default:
				htmltext = "30532-05.htm"
			}
			s.TakeItems(p, goblinNecklace, -1)
			s.TakeItems(p, goblinPendant, -1)
			s.TakeItems(p, goblinLordPendant, -1)
			if hasContract {
				st.SetCond(1)
				s.TakeItems(p, suspiciousContract, -1)
			}
			reward := 12*necklaces + 36*pendants + 33*lordPendants
			if countAll >= 10 {
				reward += 1000
			}
			if hasContract {
				reward += 1120
			}
			s.RewardItems(p, 57, reward)
		case balanki:
			if !p.HasItem(suspiciousContract) {
				htmltext = "30533-01.htm"
			} else {
				htmltext = "30533-02.htm"
				st.SetCond(1)
				s.TakeItems(p, suspiciousContract, -1)
				s.RewardItems(p, 57, 1500)
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
	chance := s.Rnd(10)
	if chance > 5 {
		switch e.NPC.NpcID() {
		case goblinBrigand, goblinSnooper, goblinBrigandLieutenant:
			s.DropItemsAlways(p, goblinNecklace, 1, 0)
		case goblinBrigandLeader:
			s.DropItemsAlways(p, goblinPendant, 1, 0)
		case goblinLord:
			s.DropItemsAlways(p, goblinLordPendant, 1, 0)
		}
	} else if chance > 4 && st.Cond() == 1 && s.DropItemsAlways(p, suspiciousMemo, 1, 3) {
		st.SetCond(2)
		s.TakeItems(p, suspiciousMemo, -1)
		s.GiveItems(p, suspiciousContract, 1)
	}
}
