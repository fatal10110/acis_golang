// Package q271 is the quest Proof of Valor: Rukain trades fifty Kasha wolf
// fangs from an orc for a necklace of courage, or rarely of valor.
package q271

import (
	"strings"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/player"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/questlog"
	"github.com/fatal10110/acis_golang/internal/gameserver/script"
)

const questName = "Q271_ProofOfValor"

// Item.
const kashaWolfFang = 1473

// Rewards.
const (
	necklaceOfValor   = 1507
	necklaceOfCourage = 1506
)

// New returns the quest.
func New() script.Script {
	return script.Script{
		Title:   "Proof of Valor",
		QuestID: 271,
		Items:   []int32{kashaWolfFang},
		Bind: script.Bindings{
			script.EventQuestStart: {30577}, // Rukain
			script.EventTalked:     {30577},
			script.EventMyDying:    {20475}, // Kasha Wolf
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
	if strings.EqualFold(e.Name, "30577-03.htm") {
		st.SetStatus(questlog.StatusStarted)
		st.SetCond(1)
		s.PlaySound(e.Player, script.SoundAccept)
		if e.Player.HasAtLeastOneItem(necklaceOfCourage, necklaceOfValor) {
			htmltext = "30577-07.htm"
		}
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
		switch {
		case p.Race() != player.RaceOrc:
			htmltext = "30577-00.htm"
		case p.Level() < 4:
			htmltext = "30577-01.htm"
		case p.HasAtLeastOneItem(necklaceOfCourage, necklaceOfValor):
			htmltext = "30577-06.htm"
		default:
			htmltext = "30577-02.htm"
		}
	case questlog.StatusStarted:
		if st.Cond() == 1 {
			if p.HasAtLeastOneItem(necklaceOfCourage, necklaceOfValor) {
				htmltext = "30577-07.htm"
			} else {
				htmltext = "30577-04.htm"
			}
		} else {
			htmltext = "30577-05.htm"
			s.TakeItems(p, kashaWolfFang, -1)
			necklace := int32(necklaceOfCourage)
			if s.Rnd(100) < 10 {
				necklace = necklaceOfValor
			}
			s.GiveItems(p, necklace, 1)
			s.PlaySound(p, script.SoundFinish)
			st.Exit(true)
		}
	}
	return htmltext
}

func onMyDying(s *script.Script, e script.MyDying) {
	p := script.ActingPlayer(e.Killer)
	st := s.CheckPlayerCondition(p, e.NPC, 1)
	if st == nil {
		return
	}
	count := int32(1)
	if s.Rnd(4) == 0 {
		count = 2
	}
	if s.DropItemsAlways(p, kashaWolfFang, count, 50) {
		st.SetCond(2)
	}
}
