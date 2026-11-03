// Package q610 is the quest Magical Power of Water, Part 2: Asefa takes the
// Green Totem to the altar of the Varka, which spends it on calling Soul of
// Water Ashutar, and the Ice Heart of Ashutar is traded back for a reward.
package q610

import (
	"strings"
	"time"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/questlog"
	"github.com/fatal10110/acis_golang/internal/gameserver/script"
)

const questName = "Q610_MagicalPowerOfWater_Part2"

// soulOfWaterAshutar is the raid the altar calls.
const soulOfWaterAshutar = 25316

// NPCs.
const (
	asefa           = 31372
	varkasHolyAltar = 31560
)

// Items.
const (
	greenTotem        = 7238
	iceHeartOfAshutar = 7239
)

// The NpcStrings the raid speaks: when called, and when it leaves unslain.
const (
	calledChat = 61050
	leftChat   = 61051
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
		Title:   "Magical Power of Water - Part 2",
		QuestID: 610,
		Items:   []int32{iceHeartOfAshutar},
		Bind: script.Bindings{
			script.EventQuestStart: {asefa},
			script.EventTalked:     {asefa, varkasHolyAltar},
			script.EventDecayed:    {soulOfWaterAshutar},
			script.EventMyDying:    {soulOfWaterAshutar},
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
	// Asefa.
	case strings.EqualFold(e.Name, "31372-04.htm"):
		if p.HasItem(greenTotem) {
			st.SetStatus(questlog.StatusStarted)
			st.SetCond(1)
			s.PlaySound(p, script.SoundAccept)
		} else {
			htmltext = "31372-02.htm"
		}
	case strings.EqualFold(e.Name, "31372-07.htm"):
		if p.HasItem(iceHeartOfAshutar) {
			s.TakeItems(p, iceHeartOfAshutar, 1)
			s.RewardExpAndSp(p, 10000, 0)
			s.PlaySound(p, script.SoundFinish)
			st.Exit(true)
		} else {
			htmltext = "31372-08.htm"
		}
	// Varka's Holy Altar.
	case strings.EqualFold(e.Name, "31560-02.htm"):
		switch {
		case !p.HasItem(greenTotem):
			htmltext = "31560-03.htm"
		case !q.raid.Empty():
			htmltext = "31560-04.htm"
		default:
			if !q.call(s, e.NPC) {
				htmltext = "31560-04.htm"
				break
			}
			st.SetCond(2)
			s.PlaySound(p, script.SoundMiddle)
			s.TakeItems(p, greenTotem, 1)
		}
	}
	return htmltext
}

// call places the raid, has it speak and removes the altar that called it.
// It reports false when the raid could not be placed or when another
// player's call placed one meanwhile: the call is then refused as one made
// while the raid stood.
func (q quest) call(s *script.Script, altar *script.NPC) bool {
	at := script.Loc{X: 104825, Y: -36926, Z: -1136, Heading: s.Rnd(65536)}
	raid := s.Spawner().AddSpawn(soulOfWaterAshutar, at, false, raidLifetime)
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
		case !e.Player.HasItem(greenTotem):
			htmltext = "31372-02.htm"
		case e.Player.Level() < 75 && e.Player.VarkaKetraAlliance() < 2:
			htmltext = "31372-03.htm"
		default:
			htmltext = "31372-01.htm"
		}
	case questlog.StatusStarted:
		cond := st.Cond()
		switch e.NPC.NpcID() {
		case asefa:
			if cond < 3 {
				htmltext = "31372-05.htm"
			} else {
				htmltext = "31372-06.htm"
			}
		case varkasHolyAltar:
			switch cond {
			case 1:
				htmltext = "31560-01.htm"
			case 2:
				htmltext = "31560-05.htm"
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
	s.GiveItems(st.Player(), iceHeartOfAshutar, 1)
}
