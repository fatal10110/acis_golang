// Package q616 is the quest Magical Power of Fire, Part 2: Udan takes the
// Red Totem to the altar of the Ketra, which spends it on calling Soul of
// Fire Nastron, and the Fire Heart of Nastron is traded back for a reward.
package q616

import (
	"strings"
	"time"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/questlog"
	"github.com/fatal10110/acis_golang/internal/gameserver/script"
)

const questName = "Q616_MagicalPowerOfFire_Part2"

// soulOfFireNastron is the raid the altar calls.
const soulOfFireNastron = 25306

// NPCs.
const (
	udanMardui      = 31379
	ketrasHolyAltar = 31558
)

// Items.
const (
	redTotem           = 7243
	fireHeartOfNastron = 7244
)

// The NpcStrings the raid speaks: when called, and when it leaves unslain.
const (
	calledChat = 61650
	leftChat   = 61651
)

// raidLifetime is how long the called raid stays.
const raidLifetime = 1200000 * time.Millisecond

// quest holds the altar that called the raid and the raid it called, shared
// by every player: one raid at a time, until it decays.
type quest struct {
	altar, raid *script.NPCCell
}

// New returns the quest.
func New() script.Script {
	q := quest{altar: new(script.NPCCell), raid: new(script.NPCCell)}
	return script.Script{
		Title:   "Magical Power of Fire - Part 2",
		QuestID: 616,
		Items:   []int32{fireHeartOfNastron},
		Bind: script.Bindings{
			script.EventQuestStart: {udanMardui},
			script.EventTalked:     {udanMardui, ketrasHolyAltar},
			script.EventDecayed:    {soulOfFireNastron},
			script.EventMyDying:    {soulOfFireNastron},
		},
		Hooks: script.Hooks{OnEvent: q.onAdvEvent, OnTalk: onTalk, OnDecayed: q.onDecayed, OnMyDying: onMyDying},
	}
}

func (q quest) onAdvEvent(s *script.Script, e script.Event) string {
	htmltext := e.Name
	st := s.QuestState(e.Player, questName)
	if st == nil {
		return htmltext
	}
	p := e.Player
	switch {
	// Udan Mardui.
	case strings.EqualFold(e.Name, "31379-04.htm"):
		if p.HasItem(redTotem) {
			st.SetStatus(questlog.StatusStarted)
			st.SetCond(1)
			s.PlaySound(p, script.SoundAccept)
		} else {
			htmltext = "31379-02.htm"
		}
	case strings.EqualFold(e.Name, "31379-08.htm"):
		if p.HasItem(fireHeartOfNastron) {
			s.TakeItems(p, fireHeartOfNastron, 1)
			s.RewardExpAndSp(p, 10000, 0)
			s.PlaySound(p, script.SoundFinish)
			st.Exit(true)
		} else {
			htmltext = "31379-09.htm"
		}
	// Ketra's Holy Altar.
	case strings.EqualFold(e.Name, "31558-02.htm"):
		switch {
		case !p.HasItem(redTotem):
			htmltext = "31558-03.htm"
		case !q.raid.Empty():
			htmltext = "31558-04.htm"
		default:
			if !q.call(s, e.NPC) {
				htmltext = "31558-04.htm"
				break
			}
			st.SetCond(2)
			s.PlaySound(p, script.SoundMiddle)
			s.TakeItems(p, redTotem, 1)
		}
	}
	return htmltext
}

// call places the raid, has it speak and removes the altar that called it.
// It reports false when the raid could not be placed or when another
// player's call placed one meanwhile: the call is then refused as one made
// while the raid stood.
func (q quest) call(s *script.Script, altar *script.NPC) bool {
	at := script.Loc{X: 142528, Y: -82528, Z: -6496, Heading: s.Rnd(65536)}
	raid := s.Spawner().AddSpawn(soulOfFireNastron, at, false, raidLifetime)
	if raid == nil {
		return false
	}
	if !q.raid.SetIfEmpty(raid) {
		raid.DeleteMe()
		return false
	}
	raid.Say(script.NpcString(calledChat))
	q.altar.Set(altar)
	altar.DeleteMe()
	return true
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
		case !e.Player.HasItem(redTotem):
			htmltext = "31379-02.htm"
		case e.Player.Level() < 75 && e.Player.VarkaKetraAlliance() > -2:
			htmltext = "31379-03.htm"
		default:
			htmltext = "31379-01.htm"
		}
	case questlog.StatusStarted:
		cond := st.Cond()
		switch e.NPC.NpcID() {
		case udanMardui:
			switch cond {
			case 1:
				htmltext = "31379-05.htm"
			case 2:
				htmltext = "31379-06.htm"
			default:
				htmltext = "31379-07.htm"
			}
		case ketrasHolyAltar:
			switch cond {
			case 1:
				htmltext = "31558-01.htm"
			case 2:
				htmltext = "31558-05.htm"
			}
		}
	}
	return htmltext
}

// onDecayed lets the altar call another raid once the raid has left. A raid
// is dead already when its decayed hooks run, so the branch that brings the
// altar back and has the raid speak never runs: the altar comes back with its
// own respawn.
func (q quest) onDecayed(s *script.Script, e script.Decayed) {
	if !q.raid.ClearIf(e.NPC) {
		return
	}
	altar := q.altar.Get()
	if !e.NPC.Dead() {
		if altar != nil {
			s.Spawner().DoRespawn(altar)
		}
		e.NPC.Say(script.NpcString(leftChat))
	}
	q.altar.ClearIf(altar)
}

func onMyDying(s *script.Script, e script.MyDying) {
	st := s.RandomPartyMember(script.ActingPlayer(e.Killer), e.NPC, script.ByCond(2))
	if st == nil {
		return
	}
	st.SetCond(3)
	s.PlaySound(st.Player(), script.SoundMiddle)
	s.GiveItems(st.Player(), fireHeartOfNastron, 1)
}
