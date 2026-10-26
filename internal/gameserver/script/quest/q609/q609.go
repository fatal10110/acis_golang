// Package q609 is the quest Magical Power of Water, part 1: Wahkan sends a
// Ketra ally to steal the Green Totem from the Udan Box. A Varka warrior
// that sees the thief at the box calls Udan's Eye, and the theft then
// fails until Asefa lets the thief try again.
package q609

import (
	"strconv"
	"strings"
	"time"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/questlog"
	"github.com/fatal10110/acis_golang/internal/gameserver/script"
)

const questName = "Q609_MagicalPowerOfWater_Part1"

// NPCs.
const (
	wahkan  = 31371
	asefa   = 31372
	udanBox = 31561
	udanEye = 31684
)

// Items.
const (
	thiefKey         = 1661
	stolenGreenTotem = 7237
	greenTotem       = 7238
	divineStone      = 7081
)

// udanEyeLifetime is how long Udan's Eye stays once called.
const udanEyeLifetime = 10000 * time.Millisecond

// The lines Udan's Eye says: on being called, and on dying while a thief
// of the quest stands by.
const (
	sayCalled = 60903
	saySeen   = 60904
)

// spawnedKey is the quest variable that holds whether Udan's Eye has been
// called.
const spawnedKey = "spawned"

// minAlliance is the Ketra alliance level the quest asks for.
const minAlliance = 2

// New returns the quest.
func New() script.Script {
	return script.Script{
		Title:   "Magical Power of Water - Part 1",
		QuestID: 609,
		Items:   []int32{stolenGreenTotem},
		Bind: script.Bindings{
			script.EventQuestStart: {wahkan},
			script.EventTalked:     {wahkan, asefa, udanBox},
			script.EventMyDying:    {udanEye},
			script.EventSeeCreature: {
				21350, 21351, 21353, 21354, 21355, 21357, 21358, 21360, 21361, 21362, 21369, 21370,
				21364, 21365, 21366, 21368, 21371, 21372, 21373, 21374, 21375,
			},
		},
		Hooks: script.Hooks{OnEvent: onAdvEvent, OnTalk: onTalk, OnMyDying: onMyDying, OnSeeCreature: onSeeCreature},
	}
}

func setSpawned(st *script.QuestState, v int) { st.Set(spawnedKey, strconv.Itoa(v)) }

// onAdvEvent starts the quest, or steals the totem: refused once Udan's Eye
// has been called, and without a Thief's Key.
func onAdvEvent(s *script.Script, e script.Event) string {
	htmltext := e.Name
	st := s.QuestState(e.Player, questName)
	if st == nil {
		return htmltext
	}
	switch {
	case strings.EqualFold(e.Name, "31371-03.htm"):
		st.SetStatus(questlog.StatusStarted)
		st.SetCond(1)
		setSpawned(st, 0)
		s.PlaySound(e.Player, script.SoundAccept)
	case strings.EqualFold(e.Name, "31561-03.htm"):
		switch {
		case st.GetInt(spawnedKey) == 1:
			htmltext = "31561-04.htm"
		case !e.Player.HasItem(thiefKey):
			htmltext = "31561-02.htm"
		default:
			st.SetCond(3)
			s.PlaySound(e.Player, script.SoundItemGet)
			s.TakeItems(e.Player, thiefKey, 1)
			s.GiveItems(e.Player, stolenGreenTotem, 1)
		}
	}
	return htmltext
}

// onTalk offers the quest to a level 74 Ketra ally of level 2 or more.
func onTalk(s *script.Script, e script.Talk) string {
	st := s.QuestState(e.Player, questName)
	htmltext := script.NoQuestMsg()
	if st == nil {
		return htmltext
	}
	switch st.Status() {
	case questlog.StatusCreated:
		if e.Player.Level() >= 74 && e.Player.VarkaKetraAlliance() >= minAlliance {
			htmltext = "31371-01.htm"
		} else {
			htmltext = "31371-02.htm"
		}
	case questlog.StatusStarted:
		cond := st.Cond()
		switch e.NPC.NpcID() {
		case wahkan:
			htmltext = "31371-04.htm"
		case asefa:
			switch {
			case cond == 1:
				htmltext = "31372-01.htm"
				st.SetCond(2)
				s.PlaySound(e.Player, script.SoundMiddle)
			case cond == 2:
				if st.GetInt(spawnedKey) == 0 {
					htmltext = "31372-02.htm"
				} else {
					htmltext = "31372-03.htm"
					setSpawned(st, 0)
					s.PlaySound(e.Player, script.SoundMiddle)
				}
			case cond == 3 && e.Player.HasItem(stolenGreenTotem):
				htmltext = "31372-04.htm"
				s.TakeItems(e.Player, stolenGreenTotem, 1)
				s.GiveItems(e.Player, greenTotem, 1)
				s.GiveItems(e.Player, divineStone, 1)
				st.Unset(spawnedKey)
				s.PlaySound(e.Player, script.SoundFinish)
				st.Exit(true)
			}
		case udanBox:
			switch cond {
			case 2:
				htmltext = "31561-01.htm"
			case 3:
				htmltext = "31561-05.htm"
			}
		}
	}
	return htmltext
}

// onMyDying has Udan's Eye say a started quest taker's face is seen when it
// dies to the killer's party.
func onMyDying(s *script.Script, e script.MyDying) {
	p := script.ActingPlayer(e.Killer)
	if s.RandomPartyMemberState(p, e.NPC, questlog.StatusStarted) == nil {
		return
	}
	e.NPC.Say(script.NpcString(saySeen))
}

// onSeeCreature calls Udan's Eye beside a thief at cond 2 once, and flags
// the quest as discovered.
func onSeeCreature(s *script.Script, e script.SeeCreature) {
	p, ok := e.Creature.(*script.Player)
	if !ok {
		return
	}
	st := s.QuestState(p, questName)
	if st == nil {
		return
	}
	if st.GetInt(spawnedKey) == 0 && st.Cond() == 2 {
		setSpawned(st, 1)
		if eye := s.Spawner().AddSpawn(udanEye, p, true, udanEyeLifetime); eye != nil {
			eye.Say(script.NpcString(sayCalled))
			s.PlaySound(p, script.SoundGiveUp)
		}
	}
}
