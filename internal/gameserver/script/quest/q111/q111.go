// Package q111 is the quest Elrokian Hunter's Proof: Marquez's expedition
// gathers the fragments of a journal, learns the Elroki hunters' song from
// Kirikashin and makes Asamah's practice trap from claws, bones and skins,
// for an Elrokian trap and its stones.
package q111

import (
	"strings"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/questlog"
	"github.com/fatal10110/acis_golang/internal/gameserver/script"
)

const questName = "Q111_ElrokianHuntersProof"

// NPCs.
const (
	marquez    = 32113
	mushika    = 32114
	asamah     = 32115
	kirikashin = 32116
)

// Items.
const (
	fragment         = 8768
	expeditionLetter = 8769
	claw             = 8770
	bone             = 8771
	skin             = 8772
	practiceTrap     = 8773
)

// New returns the quest.
func New() script.Script {
	return script.Script{
		Title:   "Elrokian Hunter's Proof",
		QuestID: 111,
		Items:   []int32{fragment, expeditionLetter, claw, bone, skin, practiceTrap},
		Bind: script.Bindings{
			script.EventQuestStart: {marquez},
			script.EventTalked:     {marquez, mushika, asamah, kirikashin},
			script.EventMyDying: {
				22196, 22197, 22198, 22218, 22200, 22201, 22202, 22219,
				22208, 22209, 22210, 22221, 22203, 22204, 22205, 22220,
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
	case strings.EqualFold(e.Name, "32113-002.htm"):
		st.SetStatus(questlog.StatusStarted)
		st.SetCond(1)
		s.PlaySound(p, script.SoundAccept)
	case strings.EqualFold(e.Name, "32115-002.htm"):
		st.SetCond(3)
		s.PlaySound(p, script.SoundMiddle)
	case strings.EqualFold(e.Name, "32113-009.htm"):
		st.SetCond(4)
		s.PlaySound(p, script.SoundMiddle)
	case strings.EqualFold(e.Name, "32113-018.htm"):
		st.SetCond(6)
		s.PlaySound(p, script.SoundMiddle)
		s.TakeItems(p, fragment, -1)
		s.GiveItems(p, expeditionLetter, 1)
	case strings.EqualFold(e.Name, "32116-003.htm"):
		st.SetCond(7)
		s.PlaySound(p, "EtcSound.elcroki_song_full")
	case strings.EqualFold(e.Name, "32116-005.htm"):
		st.SetCond(8)
		s.PlaySound(p, script.SoundMiddle)
	case strings.EqualFold(e.Name, "32115-004.htm"):
		st.SetCond(9)
		s.PlaySound(p, script.SoundMiddle)
	case strings.EqualFold(e.Name, "32115-006.htm"):
		st.SetCond(10)
		s.PlaySound(p, script.SoundMiddle)
	case strings.EqualFold(e.Name, "32116-007.htm"):
		s.TakeItems(p, practiceTrap, 1)
		s.GiveItems(p, 8763, 1)
		s.GiveItems(p, 8764, 100)
		s.RewardItems(p, 57, 1022636)
		s.PlaySound(p, script.SoundFinish)
		st.Exit(false)
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
		if p.Level() < 75 {
			htmltext = "32113-000.htm"
		} else {
			htmltext = "32113-001.htm"
		}
	case questlog.StatusStarted:
		cond := st.Cond()
		switch e.NPC.NpcID() {
		case marquez:
			switch cond {
			case 1, 2:
				htmltext = "32113-002.htm"
			case 3:
				htmltext = "32113-003.htm"
			case 4:
				htmltext = "32113-009.htm"
			case 5:
				htmltext = "32113-010.htm"
			}
		case mushika:
			if cond == 1 {
				st.SetCond(2)
				s.PlaySound(p, script.SoundMiddle)
			}
			htmltext = "32114-001.htm"
		case asamah:
			switch cond {
			case 2:
				htmltext = "32115-001.htm"
			case 3:
				htmltext = "32115-002.htm"
			case 8:
				htmltext = "32115-003.htm"
			case 9:
				htmltext = "32115-004.htm"
			case 10:
				htmltext = "32115-006.htm"
			case 11:
				htmltext = "32115-007.htm"
				st.SetCond(12)
				s.PlaySound(p, script.SoundMiddle)
				s.TakeItems(p, bone, -1)
				s.TakeItems(p, claw, -1)
				s.TakeItems(p, skin, -1)
				s.GiveItems(p, practiceTrap, 1)
			}
		case kirikashin:
			switch {
			case cond < 6:
				htmltext = "32116-008.htm"
			case cond == 6:
				htmltext = "32116-001.htm"
				s.TakeItems(p, expeditionLetter, 1)
			case cond == 7:
				htmltext = "32116-004.htm"
			case cond == 12:
				htmltext = "32116-006.htm"
			}
		}
	case questlog.StatusCompleted:
		htmltext = script.AlreadyCompletedMsg()
	}
	return htmltext
}

func onMyDying(s *script.Script, e script.MyDying) {
	p := script.ActingPlayer(e.Killer)
	st := s.RandomPartyMemberState(p, e.NPC, questlog.StatusStarted)
	if st == nil {
		return
	}
	p = st.Player()
	switch e.NPC.NpcID() {
	case 22196, 22197, 22198, 22218:
		if st.Cond() == 4 && s.DropItems(p, fragment, 1, 50, 250000) {
			st.SetCond(5)
		}
	case 22200, 22201, 22202, 22219:
		if st.Cond() == 10 && s.DropItems(p, claw, 1, 10, 650000) {
			if p.ItemCount(bone) >= 10 && p.ItemCount(skin) >= 10 {
				st.SetCond(11)
			}
		}
	case 22208, 22209, 22210, 22221:
		if st.Cond() == 10 && s.DropItems(p, skin, 1, 10, 650000) {
			if p.ItemCount(claw) >= 10 && p.ItemCount(bone) >= 10 {
				st.SetCond(11)
			}
		}
	case 22203, 22204, 22205, 22220:
		if st.Cond() == 10 && s.DropItems(p, bone, 1, 10, 650000) {
			if p.ItemCount(claw) >= 10 && p.ItemCount(skin) >= 10 {
				st.SetCond(11)
			}
		}
	}
}
