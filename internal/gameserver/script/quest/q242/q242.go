// Package q242 is the quest Possessor of a Precious Soul - 2: on a
// subclass of level 60 or more, with Virgil's letter, the player searches
// the angel corpses for the golden hair, has Kalis bind it with Matild's
// sorcery ingredient, binds the four cornerstones with the restrainers'
// orbs and wakes the pure white unicorn, and Kassandra closes it with a
// letter for Caradine.
package q242

import (
	"strconv"
	"strings"
	"time"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/questlog"
	"github.com/fatal10110/acis_golang/internal/gameserver/script"
)

const questName = "Q242_PossessorOfAPreciousSoul"

// NPCs.
const (
	virgil           = 31742
	kassandra        = 31743
	ogmar            = 31744
	mysteriousKnight = 31751
	angelCorpse      = 31752
	kalis            = 30759
	matild           = 30738
	cornerstone      = 31748
	fallenUnicorn    = 31746
	pureUnicorn      = 31747
)

// Monsters.
const restrainerOfGlory = 27317

// Items.
const (
	virgilLetter      = 7677
	goldenHair        = 7590
	sorceryIngredient = 7596
	orbOfBinding      = 7595
	caradineLetter    = 7678
)

// quest holds the pure white unicorn the fallen unicorn's timer spawned,
// shared by every player: one at a time. A unicorn that left on its own
// 30 second despawn stays held, so no fallen unicorn spawns another.
type quest struct{ pureUnicorn *script.NPCCell }

// New returns the quest.
func New() script.Script {
	q := quest{pureUnicorn: new(script.NPCCell)}
	return script.Script{
		Title:   "Possessor of a Precious Soul - 2",
		QuestID: 242,
		Items:   []int32{goldenHair, sorceryIngredient, orbOfBinding},
		Bind: script.Bindings{
			script.EventQuestStart: {virgil},
			script.EventTalked: {
				virgil, kassandra, ogmar, mysteriousKnight, angelCorpse, kalis, matild, cornerstone,
				fallenUnicorn, pureUnicorn,
			},
			script.EventMyDying: {restrainerOfGlory},
		},
		Hooks: script.Hooks{OnEvent: onAdvEvent, OnTimer: q.onTimer, OnTalk: onTalk, OnMyDying: onMyDying},
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
	// Kassandra.
	case strings.EqualFold(e.Name, "31743-05.htm"):
		st.SetCond(2)
		s.PlaySound(p, script.SoundMiddle)
	// Ogmar.
	case strings.EqualFold(e.Name, "31744-02.htm"):
		st.SetCond(3)
		s.PlaySound(p, script.SoundMiddle)
	// Mysterious Knight.
	case strings.EqualFold(e.Name, "31751-02.htm"):
		st.SetCond(4)
		s.PlaySound(p, script.SoundMiddle)
	// Kalis.
	case strings.EqualFold(e.Name, "30759-02.htm"):
		st.SetCond(7)
		s.PlaySound(p, script.SoundMiddle)
	case strings.EqualFold(e.Name, "30759-05.htm"):
		if p.HasItem(sorceryIngredient) {
			st.SetCond(9)
			s.PlaySound(p, script.SoundMiddle)
			s.TakeItems(p, goldenHair, 1)
			s.TakeItems(p, sorceryIngredient, 1)
		} else {
			st.SetCond(7)
			htmltext = "30759-02.htm"
		}
	// Matild.
	case strings.EqualFold(e.Name, "30738-02.htm"):
		st.SetCond(8)
		s.PlaySound(p, script.SoundMiddle)
		s.GiveItems(p, sorceryIngredient, 1)
	// Cornerstone.
	case strings.EqualFold(e.Name, "31748-03.htm"):
		if p.HasItem(orbOfBinding) {
			e.NPC.DeleteMe()
			s.TakeItems(p, orbOfBinding, 1)

			cornerstones := st.GetInt("cornerstone") + 1
			if cornerstones == 4 {
				st.Unset("orb")
				st.Unset("cornerstone")
				st.SetCond(10)
				s.PlaySound(p, script.SoundMiddle)
			} else {
				st.Set("cornerstone", strconv.Itoa(int(cornerstones)))
			}
		} else {
			htmltext = ""
		}
	}
	return htmltext
}

// onTimer swaps the fallen unicorn for the pure white one, which stays 30
// seconds, and takes the pure white one away once it has been talked to.
func (q quest) onTimer(s *script.Script, e script.Timer) string {
	switch {
	case strings.EqualFold(e.Name, "fallen"):
		if q.pureUnicorn.Empty() {
			e.NPC.DeleteMe()

			// Spawn Pure Unicorn (spawn for 30 seconds).
			q.pureUnicorn.Set(s.Spawner().AddSpawn(pureUnicorn, e.NPC, false, 30000*time.Millisecond))
		}
	case strings.EqualFold(e.Name, "pure"):
		if unicorn := q.pureUnicorn.Get(); unicorn != nil {
			// Despawn Pure Unicorn.
			unicorn.DeleteMe()
			q.pureUnicorn.Clear()
		}
	}
	return ""
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
		if p.HasItem(virgilLetter) {
			if !p.SubClassActive() || p.Level() < 60 {
				htmltext = "31742-02.htm"
			} else {
				htmltext = "31742-03.htm"
				st.SetStatus(questlog.StatusStarted)
				st.SetCond(1)
				s.PlaySound(p, script.SoundAccept)
				s.TakeItems(p, virgilLetter, 1)
			}
		}
	case questlog.StatusStarted:
		if !p.SubClassActive() {
			break
		}
		if page := startedTalk(s, e, st); page != "" {
			htmltext = page
		}
	case questlog.StatusCompleted:
		htmltext = script.AlreadyCompletedMsg()
	}
	return htmltext
}

// startedTalk is the talk of a started subclass player: the page the NPC
// gives, empty when it has none to give.
func startedTalk(s *script.Script, e script.Talk, st *script.QuestState) string {
	p := e.Player
	cond := st.Cond()
	switch e.NPC.NpcID() {
	case virgil:
		switch cond {
		case 1:
			return "31742-04.htm"
		case 2:
			return "31742-05.htm"
		}
	case kassandra:
		switch cond {
		case 1:
			return "31743-01.htm"
		case 2:
			return "31743-06.htm"
		case 11:
			s.GiveItems(p, caradineLetter, 1)
			s.RewardExpAndSp(p, 455764, 0)
			p.SocialAction(3)
			s.PlaySound(p, script.SoundFinish)
			st.Exit(false)
			return "31743-07.htm"
		}
	case ogmar:
		switch cond {
		case 2:
			return "31744-01.htm"
		case 3:
			return "31744-03.htm"
		}
	case mysteriousKnight:
		switch cond {
		case 3:
			return "31751-01.htm"
		case 4:
			return "31751-03.htm"
		case 5:
			if p.HasItem(goldenHair) {
				st.SetCond(6)
				s.PlaySound(p, script.SoundMiddle)
				return "31751-04.htm"
			}
			st.SetCond(4)
			return "31751-03.htm"
		case 6:
			return "31751-05.htm"
		}
	case angelCorpse:
		switch cond {
		case 4:
			e.NPC.DeleteMe()

			angel := st.GetInt("angel") + 1
			if angel == 4 {
				st.Unset("angel")
				st.SetCond(5)
				s.PlaySound(p, script.SoundMiddle)
				s.GiveItems(p, goldenHair, 1)
				return "31752-02.htm"
			}
			st.Set("angel", strconv.Itoa(int(angel)))
			return "31752-01.htm"
		case 5:
			return "31752-01.htm"
		}
	case kalis:
		switch cond {
		case 6:
			return "30759-01.htm"
		case 7:
			return "30759-03.htm"
		case 8:
			if p.HasItem(sorceryIngredient) {
				return "30759-04.htm"
			}
			st.SetCond(7)
			return "30759-03.htm"
		case 9:
			return "30759-06.htm"
		}
	case matild:
		switch cond {
		case 7:
			return "30738-01.htm"
		case 8:
			return "30738-03.htm"
		}
	case cornerstone:
		if cond == 9 {
			if p.HasItem(orbOfBinding) {
				return "31748-02.htm"
			}
			return "31748-01.htm"
		}
	case fallenUnicorn:
		switch cond {
		case 9:
			return "31746-01.htm"
		case 10:
			s.StartTimer("fallen", e.NPC, nil, 3000*time.Millisecond)
			return "31746-02.htm"
		}
	case pureUnicorn:
		switch cond {
		case 10:
			st.SetCond(11)
			s.PlaySound(p, script.SoundMiddle)
			s.StartTimer("pure", e.NPC, nil, 3000*time.Millisecond)
			return "31747-01.htm"
		case 11:
			return "31747-02.htm"
		}
	}
	return ""
}

func onMyDying(s *script.Script, e script.MyDying) {
	p := script.ActingPlayer(e.Killer)
	st := s.CheckPlayerCondition(p, e.NPC, 9)
	if st == nil || !p.SubClassActive() {
		return
	}

	// Check orbs internally, because player can use them before he gets them all.
	orbs := st.GetInt("orb")
	if orbs < 4 {
		st.Set("orb", strconv.Itoa(int(orbs+1)))
		s.PlaySound(p, script.SoundItemGet)
		s.GiveItems(p, orbOfBinding, 1)
	}
}
