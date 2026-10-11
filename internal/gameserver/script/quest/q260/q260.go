// Package q260 is the quest Hunt the Orcs: Rayen pays an elf for the
// amulets and necklaces of the Kaboo orcs, with beginner shots on the first
// payment.
package q260

import (
	"strings"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/player"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/questlog"
	"github.com/fatal10110/acis_golang/internal/gameserver/script"
)

const questName = "Q260_HuntTheOrcs"

// rayen is the quest giver.
const rayen = 30221

// Items.
const (
	orcAmulet   = 1114
	orcNecklace = 1115
)

// Monsters.
const (
	kabooOrc                  = 20468
	kabooOrcArcher            = 20469
	kabooOrcGrunt             = 20470
	kabooOrcFighter           = 20471
	kabooOrcFighterLeader     = 20472
	kabooOrcFighterLieutenant = 20473
)

// New returns the quest.
func New() script.Script {
	return script.Script{
		Title:   "Hunt the Orcs",
		QuestID: 260,
		Items:   []int32{orcAmulet, orcNecklace},
		Bind: script.Bindings{
			script.EventQuestStart: {rayen},
			script.EventTalked:     {rayen},
			script.EventMyDying: {
				kabooOrc, kabooOrcArcher, kabooOrcGrunt, kabooOrcFighter,
				kabooOrcFighterLeader, kabooOrcFighterLieutenant,
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
	switch {
	case strings.EqualFold(e.Name, "30221-03.htm"):
		st.SetStatus(questlog.StatusStarted)
		st.SetCond(1)
		s.PlaySound(e.Player, script.SoundAccept)
	case strings.EqualFold(e.Name, "30221-06.htm"):
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
	switch st.Status() {
	case questlog.StatusCreated:
		switch {
		case e.Player.Race() != player.RaceElf:
			htmltext = "30221-00.htm"
		case e.Player.Level() < 6:
			htmltext = "30221-01.htm"
		default:
			htmltext = "30221-02.htm"
		}
	case questlog.StatusStarted:
		amulets := e.Player.ItemCount(orcAmulet)
		necklaces := e.Player.ItemCount(orcNecklace)
		if amulets == 0 && necklaces == 0 {
			htmltext = "30221-04.htm"
		} else {
			htmltext = "30221-05.htm"
			s.TakeItems(e.Player, orcAmulet, -1)
			s.TakeItems(e.Player, orcNecklace, -1)
			reward := amulets*12 + necklaces*30
			if amulets+necklaces >= 10 {
				reward += 1000
			}
			s.RewardItems(e.Player, 57, reward)
			s.RewardNewbieShots(e.Player, 6000, 3000)
		}
	}
	return htmltext
}

func onMyDying(s *script.Script, e script.MyDying) {
	p := script.ActingPlayer(e.Killer)
	if s.CheckPlayerState(p, e.NPC, questlog.StatusStarted) == nil {
		return
	}
	switch e.NPC.NpcID() {
	case kabooOrc, kabooOrcGrunt, kabooOrcArcher:
		s.DropItems(p, orcAmulet, 1, 0, 500000)
	case kabooOrcFighter, kabooOrcFighterLeader, kabooOrcFighterLieutenant:
		s.DropItems(p, orcNecklace, 1, 0, 500000)
	}
}
