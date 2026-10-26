// Package q615 is the quest Magical Power of Fire, part 1: Naran Ashanuk
// sends a Varka ally to steal the Red Totem from the Asefa Box. A Ketra
// warrior that sees the thief at the box calls Asefa's Eye, and the theft
// then fails until Udan lets the thief try again.
package q615

import (
	"strconv"
	"strings"
	"time"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/questlog"
	"github.com/fatal10110/acis_golang/internal/gameserver/script"
)

const questName = "Q615_MagicalPowerOfFire_Part1"

// NPCs.
const (
	naran    = 31378
	udan     = 31379
	asefaBox = 31559
	asefaEye = 31685
)

// Items.
const (
	thiefKey       = 1661
	stolenRedTotem = 7242
	redTotem       = 7243
	divineStone    = 7081
)

// asefaEyeLifetime is how long Asefa's Eye stays once called.
const asefaEyeLifetime = 10000 * time.Millisecond

// The lines Asefa's Eye says: on being called, and on dying while a thief
// of the quest stands by.
const (
	sayCalled = 61503
	saySeen   = 61504
)

// spawnedKey is the quest variable that holds whether Asefa's Eye has been
// called.
const spawnedKey = "spawned"

// minAlliance is the Varka alliance level the quest asks for.
const minAlliance = 2

// New returns the quest.
func New() script.Script {
	return script.Script{
		Title:   "Magical Power of Fire - Part 1",
		QuestID: 615,
		Items:   []int32{stolenRedTotem},
		Bind: script.Bindings{
			script.EventQuestStart: {naran},
			script.EventTalked:     {naran, udan, asefaBox},
			script.EventMyDying:    {asefaEye},
			script.EventSeeCreature: {
				21350, 21351, 21353, 21354, 21355, 21357, 21358, 21360, 21361, 21362, 21369, 21370,
				21364, 21365, 21366, 21368, 21371, 21372, 21373, 21374, 21375,
			},
		},
		Hooks: script.Hooks{OnEvent: onAdvEvent, OnTalk: onTalk, OnMyDying: onMyDying, OnSeeCreature: onSeeCreature},
	}
}

func setSpawned(st *script.QuestState, v int) { st.Set(spawnedKey, strconv.Itoa(v)) }

// onAdvEvent starts the quest, or steals the totem: refused once Asefa's Eye
// has been called, and without a Thief's Key.
func onAdvEvent(s *script.Script, e script.Event) string {
	htmltext := e.Name
	st := s.QuestState(e.Player, questName)
	if st == nil {
		return htmltext
	}
	switch {
	case strings.EqualFold(e.Name, "31378-03.htm"):
		st.SetStatus(questlog.StatusStarted)
		st.SetCond(1)
		setSpawned(st, 0)
		s.PlaySound(e.Player, script.SoundAccept)
	case strings.EqualFold(e.Name, "31559-03.htm"):
		switch {
		case st.GetInt(spawnedKey) == 1:
			htmltext = "31559-04.htm"
		case !e.Player.HasItem(thiefKey):
			htmltext = "31559-02.htm"
		default:
			st.SetCond(3)
			s.PlaySound(e.Player, script.SoundItemGet)
			s.TakeItems(e.Player, thiefKey, 1)
			s.GiveItems(e.Player, stolenRedTotem, 1)
		}
	}
	return htmltext
}

// onTalk offers the quest to a level 74 Varka ally of level 2 or more.
func onTalk(s *script.Script, e script.Talk) string {
	st := s.QuestState(e.Player, questName)
	htmltext := script.NoQuestMsg()
	if st == nil {
		return htmltext
	}
	switch st.Status() {
	case questlog.StatusCreated:
		if e.Player.Level() >= 74 && e.Player.VarkaKetraAlliance() <= -minAlliance {
			htmltext = "31378-01.htm"
		} else {
			htmltext = "31378-02.htm"
		}
	case questlog.StatusStarted:
		cond := st.Cond()
		switch e.NPC.NpcID() {
		case naran:
			htmltext = "31378-04.htm"
		case udan:
			switch {
			case cond == 1:
				htmltext = "31379-01.htm"
				st.SetCond(2)
				s.PlaySound(e.Player, script.SoundMiddle)
			case cond == 2:
				if st.GetInt(spawnedKey) == 0 {
					htmltext = "31379-02.htm"
				} else {
					htmltext = "31379-03.htm"
					setSpawned(st, 0)
					s.PlaySound(e.Player, script.SoundMiddle)
				}
			case cond == 3 && e.Player.HasItem(stolenRedTotem):
				htmltext = "31379-04.htm"
				s.TakeItems(e.Player, stolenRedTotem, 1)
				s.GiveItems(e.Player, redTotem, 1)
				s.GiveItems(e.Player, divineStone, 1)
				st.Unset(spawnedKey)
				s.PlaySound(e.Player, script.SoundFinish)
				st.Exit(true)
			}
		case asefaBox:
			switch cond {
			case 2:
				htmltext = "31559-01.htm"
			case 3:
				htmltext = "31559-05.htm"
			}
		}
	}
	return htmltext
}

// onMyDying has Asefa's Eye say a started quest taker's face is seen when it
// dies to the killer's party.
func onMyDying(s *script.Script, e script.MyDying) {
	p := script.ActingPlayer(e.Killer)
	if s.RandomPartyMemberState(p, e.NPC, questlog.StatusStarted) == nil {
		return
	}
	e.NPC.Say(script.NpcString(saySeen))
}

// onSeeCreature calls Asefa's Eye beside a thief at cond 2 once, and flags
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
		if eye := s.Spawner().AddSpawn(asefaEye, p, true, asefaEyeLifetime); eye != nil {
			eye.Say(script.NpcString(sayCalled))
			s.PlaySound(p, script.SoundGiveUp)
		}
	}
}
