// Package q419 is the quest Get a Pet: a young adventurer hunts the animals
// a list names, learns about pets from three townsfolk, passes Martin's quiz
// and is given a wolf collar.
package q419

import (
	"strconv"
	"strings"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/questlog"
	"github.com/fatal10110/acis_golang/internal/gameserver/script"
)

const questName = "Q419_GetAPet"

// Quest items.
const (
	animalLoverList     = 3417
	animalSlayerList1   = 3418
	animalSlayerList2   = 3419
	animalSlayerList3   = 3420
	animalSlayerList4   = 3421
	animalSlayerList5   = 3422
	bloodyFang          = 3423
	bloodyClaw          = 3424
	bloodyNail          = 3425
	bloodyKashaFang     = 3426
	bloodyTarantulaNail = 3427
)

// Reward.
const wolfCollar = 2375

// Npcs.
const (
	martin = 30731
	bella  = 30256
	metty  = 30072
	ellie  = 30091
)

// New returns the quest.
func New() script.Script {
	return script.Script{
		Title:   "Get a Pet",
		QuestID: 419,
		Items: []int32{
			animalLoverList, animalSlayerList1, animalSlayerList2, animalSlayerList3, animalSlayerList4,
			animalSlayerList5, bloodyFang, bloodyClaw, bloodyNail, bloodyKashaFang, bloodyTarantulaNail,
		},
		Bind: script.Bindings{
			script.EventQuestStart: {martin},
			script.EventTalked:     {martin, bella, ellie, metty},
			script.EventMyDying: {
				20103, 20106, 20108, 20460, 20308, 20466, 20025, 20105, 20034, 20474, 20476, 20478, 20403, 20508,
			},
		},
		Hooks: script.Hooks{OnEvent: onAdvEvent, OnTalk: onTalk, OnMyDying: onMyDying},
	}
}

// drop returns the proof a monster drops and the chance of it.
func drop(npcID int32) (itemID, chance int32) {
	switch npcID {
	case 20103:
		return bloodyFang, 600000
	case 20106:
		return bloodyFang, 750000
	case 20108:
		return bloodyFang, 1000000
	case 20460:
		return bloodyClaw, 600000
	case 20308:
		return bloodyClaw, 750000
	case 20466:
		return bloodyClaw, 1000000
	case 20025:
		return bloodyNail, 600000
	case 20105:
		return bloodyNail, 750000
	case 20034:
		return bloodyNail, 1000000
	case 20474:
		return bloodyKashaFang, 600000
	case 20476:
		return bloodyKashaFang, 750000
	case 20478:
		return bloodyKashaFang, 1000000
	case 20403:
		return bloodyTarantulaNail, 750000
	case 20508:
		return bloodyTarantulaNail, 1000000
	}
	return 0, 0
}

func onAdvEvent(s *script.Script, e script.Event) string {
	htmltext := e.Name
	st := s.QuestState(e.Player, questName)
	if st == nil {
		return htmltext
	}
	p := e.Player
	switch {
	case strings.EqualFold(e.Name, "task"):
		race := int32(p.Race())
		htmltext = "30731-0" + strconv.Itoa(int(race)+4) + ".htm"
		st.SetStatus(questlog.StatusStarted)
		st.SetCond(1)
		s.PlaySound(p, script.SoundAccept)
		s.GiveItems(p, animalSlayerList1+race, 1)
	case strings.EqualFold(e.Name, "30731-12.htm"):
		s.PlaySound(p, script.SoundMiddle)
		s.TakeItems(p, animalSlayerList1, 1)
		s.TakeItems(p, animalSlayerList2, 1)
		s.TakeItems(p, animalSlayerList3, 1)
		s.TakeItems(p, animalSlayerList4, 1)
		s.TakeItems(p, animalSlayerList5, 1)
		s.TakeItems(p, bloodyFang, -1)
		s.TakeItems(p, bloodyClaw, -1)
		s.TakeItems(p, bloodyNail, -1)
		s.TakeItems(p, bloodyKashaFang, -1)
		s.TakeItems(p, bloodyTarantulaNail, -1)
		s.GiveItems(p, animalLoverList, 1)
	case strings.EqualFold(e.Name, "30256-03.htm"):
		learn(s, st, p, 1)
	case strings.EqualFold(e.Name, "30072-02.htm"):
		learn(s, st, p, 2)
	case strings.EqualFold(e.Name, "30091-02.htm"):
		learn(s, st, p, 4)
	case strings.EqualFold(e.Name, "test"):
		st.Set("answers", "0")
		st.Set("quiz", "20 21 22 23 24 25 26 27 28 29 30 31 32 33")
		return checkQuestions(s, st, p)
	case strings.EqualFold(e.Name, "wrong"):
		st.Set("wrong", strconv.Itoa(int(st.GetInt("wrong"))+1))
		return checkQuestions(s, st, p)
	case strings.EqualFold(e.Name, "right"):
		st.Set("correct", strconv.Itoa(int(st.GetInt("correct"))+1))
		return checkQuestions(s, st, p)
	}
	return htmltext
}

// learn records that one of the three townsfolk has been heard, with the
// bit of that person, and plays the middle sound once all three have.
func learn(s *script.Script, st *script.QuestState, p *script.Player, bit int32) {
	progress := st.GetInt("progress") | bit
	st.Set("progress", strconv.Itoa(int(progress)))
	if progress == 7 {
		s.PlaySound(p, script.SoundMiddle)
	}
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
		if p.Level() < 15 {
			htmltext = "30731-01.htm"
		} else {
			htmltext = "30731-02.htm"
		}
	case questlog.StatusStarted:
		switch e.NPC.NpcID() {
		case martin:
			switch {
			case p.HasAtLeastOneItem(animalSlayerList1, animalSlayerList2, animalSlayerList3, animalSlayerList4, animalSlayerList5):
				proofs := p.ItemCount(bloodyFang) + p.ItemCount(bloodyClaw) + p.ItemCount(bloodyNail) +
					p.ItemCount(bloodyKashaFang) + p.ItemCount(bloodyTarantulaNail)
				switch {
				case proofs == 0:
					htmltext = "30731-09.htm"
				case proofs < 50:
					htmltext = "30731-10.htm"
				default:
					htmltext = "30731-11.htm"
				}
			case st.GetInt("progress") == 7:
				htmltext = "30731-13.htm"
			default:
				htmltext = "30731-16.htm"
			}
		case bella:
			htmltext = "30256-01.htm"
		case metty:
			htmltext = "30072-01.htm"
		case ellie:
			htmltext = "30091-01.htm"
		}
	}
	return htmltext
}

func onMyDying(s *script.Script, e script.MyDying) {
	p := script.ActingPlayer(e.Killer)
	if s.CheckPlayerState(p, e.NPC, questlog.StatusStarted) == nil {
		return
	}
	proof, chance := drop(e.NPC.NpcID())
	if p.HasItem(proof - 5) {
		s.DropItems(p, proof, 1, 50, chance)
	}
}

// checkQuestions picks the next quiz page, or ends the quiz after ten
// answers: any wrong answer restarts it, none earns the wolf collar.
func checkQuestions(s *script.Script, st *script.QuestState, p *script.Player) string {
	answers := st.GetInt("correct") + st.GetInt("wrong")
	if answers < 10 {
		quiz, ok := st.Get("quiz")
		if !ok {
			// An answer sent before the quiz was started has no question to
			// pick: it gets no page.
			return ""
		}
		questions := strings.Split(quiz, " ")
		index := 0
		if len(questions) > 1 {
			index = s.Rnd(len(questions) - 1)
		}
		question := questions[index]

		if int32(len(questions)) > 10-answers {
			questions[index] = questions[len(questions)-1]
			st.Set("quiz", strings.Join(questions[:len(questions)-1], " "))
		}
		return "30731-" + question + ".htm"
	}

	if st.GetInt("wrong") > 0 {
		st.Unset("progress")
		st.Unset("answers")
		st.Unset("quiz")
		st.Unset("wrong")
		st.Unset("correct")
		return "30731-14.htm"
	}

	s.TakeItems(p, animalLoverList, 1)
	s.GiveItems(p, wolfCollar, 1)
	s.PlaySound(p, script.SoundFinish)
	st.Exit(true)
	return "30731-15.htm"
}
