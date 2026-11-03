// Package q374 is the quest Whisper of Dreams, Part 1: Manakia trades the
// teeth and lights of the cave monsters for armor pieces and a sealed stone
// that Torai opens.
package q374

import (
	"strconv"
	"strings"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/questlog"
	"github.com/fatal10110/acis_golang/internal/gameserver/script"
)

const questName = "Q374_WhisperOfDreams_Part1"

// Npcs.
const (
	manakia = 30515
	torai   = 30557
)

// Monsters.
const (
	caveBeast = 20620
	deathWave = 20621
)

// Quest items.
const (
	caveBeastTooth        = 5884
	deathWaveLight        = 5885
	sealedMysteriousStone = 5886
	mysteriousStone       = 5887
)

// reward is what Manakia gives for a full set: an item, its count and adena.
type reward struct {
	item, count, adena int32
}

// rewards lists the choices of the reward page, indexed by the number the
// event carries.
func rewards() []reward {
	return []reward{
		{5486, 3, 2950},
		{5487, 2, 18050},
		{5488, 2, 18050},
		{5485, 4, 10450},
		{5489, 6, 15550},
	}
}

// New returns the quest.
func New() script.Script {
	return script.Script{
		Title:   "Whisper of Dreams, Part 1",
		QuestID: 374,
		Items:   []int32{deathWaveLight, caveBeastTooth, sealedMysteriousStone, mysteriousStone},
		Bind: script.Bindings{
			script.EventQuestStart: {manakia},
			script.EventTalked:     {manakia, torai},
			script.EventMyDying:    {caveBeast, deathWave},
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
	case strings.EqualFold(e.Name, "30515-03.htm"):
		st.SetStatus(questlog.StatusStarted)
		st.SetCond(1)
		st.Set("condStone", strconv.Itoa(1))
		s.PlaySound(p, script.SoundAccept)
	case strings.HasPrefix(e.Name, "30515-06-"):
		if p.ItemCount(caveBeastTooth) >= 65 && p.ItemCount(deathWaveLight) >= 65 {
			htmltext = "30515-06.htm"
			s.PlaySound(p, script.SoundMiddle)
			r := rewards()[number(e.Name, 9, 10)]
			s.TakeItems(p, caveBeastTooth, -1)
			s.TakeItems(p, deathWaveLight, -1)
			s.RewardItems(p, 57, r.adena)
			s.GiveItems(p, r.item, r.count)
		} else {
			htmltext = "30515-07.htm"
		}
	case strings.EqualFold(e.Name, "30515-08.htm"):
		s.PlaySound(p, script.SoundFinish)
		st.Exit(true)
	case strings.EqualFold(e.Name, "30557-02.htm"):
		if st.Cond() == 2 && p.HasItem(sealedMysteriousStone) {
			st.SetCond(3)
			s.TakeItems(p, sealedMysteriousStone, -1)
			s.GiveItems(p, mysteriousStone, 1)
			s.PlaySound(p, script.SoundMiddle)
		} else {
			htmltext = "30557-03.htm"
		}
	}
	return htmltext
}

// number reads event[from:to] as a decimal integer; a missing or other
// character panics, so the invocation aborts.
func number(event string, from, to int) int {
	n, err := strconv.ParseInt(event[from:to], 10, 32)
	if err != nil {
		panic(err)
	}
	return int(n)
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
		if p.Level() < 56 {
			htmltext = "30515-01.htm"
		} else {
			htmltext = "30515-02.htm"
		}
	case questlog.StatusStarted:
		cond := st.Cond()
		switch e.NPC.NpcID() {
		case manakia:
			if !p.HasItem(sealedMysteriousStone) {
				if p.ItemCount(caveBeastTooth) >= 65 && p.ItemCount(deathWaveLight) >= 65 {
					htmltext = "30515-05.htm"
				} else {
					htmltext = "30515-04.htm"
				}
			} else if cond == 1 {
				htmltext = "30515-09.htm"
				st.SetCond(2)
				s.PlaySound(p, script.SoundMiddle)
			} else {
				htmltext = "30515-10.htm"
			}
		case torai:
			if cond == 2 && p.HasItem(sealedMysteriousStone) {
				htmltext = "30557-01.htm"
			}
		}
	}
	return htmltext
}

func onMyDying(s *script.Script, e script.MyDying) {
	player := script.ActingPlayer(e.Killer)
	st := s.RandomPartyMemberState(player, e.NPC, questlog.StatusStarted)
	if st == nil {
		return
	}
	item := int32(deathWaveLight)
	if e.NPC.NpcID() == caveBeast {
		item = caveBeastTooth
	}
	s.DropItems(st.Player(), item, 1, 65, 500000)

	st = s.RandomPartyMember(player, e.NPC, script.ByVar("condStone", "1"))
	if st != nil && s.DropItems(st.Player(), sealedMysteriousStone, 1, 1, 1000) {
		st.Unset("condStone")
	}
}
