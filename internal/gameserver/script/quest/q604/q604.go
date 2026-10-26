// Package q604 is the quest Daimon The White-Eyed, Part 2: the Eye of Argos
// turns the unfinished summon crystal into a summon crystal, the altar of
// Daimon spends it on calling Daimon the White-Eyed, and his essence is
// traded back for a reward.
package q604

import (
	"strings"
	"time"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/questlog"
	"github.com/fatal10110/acis_golang/internal/gameserver/script"
)

const questName = "Q604_DaimonTheWhiteEyed_Part2"

// daimonTheWhiteEyed is the raid the altar calls.
const daimonTheWhiteEyed = 25290

// NPCs.
const (
	eyeOfArgos  = 31683
	daimonAltar = 31541
)

// Items.
const (
	unfinishedSummonCrystal = 7192
	summonCrystal           = 7193
	essenceOfDaimon         = 7194
)

// The NpcStrings the raid speaks: when called, and when it leaves unslain.
const (
	calledChat = 60403
	leftChat   = 60404
)

// raidLifetime is how long the called raid stays.
const raidLifetime = 1200000 * time.Millisecond

// rewards are the items the essence is traded for, one at random.
func rewards() []int32 { return []int32{4595, 4596, 4597, 4598, 4599, 4600} }

// quest holds the altar that called the raid and the raid it called, shared
// by every player: one raid at a time, until it decays.
type quest struct {
	altar, raid *script.NPCCell
}

// New returns the quest.
func New() script.Script {
	q := quest{altar: new(script.NPCCell), raid: new(script.NPCCell)}
	return script.Script{
		Title:   "Daimon The White-Eyed - Part 2",
		QuestID: 604,
		Items:   []int32{summonCrystal, essenceOfDaimon},
		Bind: script.Bindings{
			script.EventQuestStart: {eyeOfArgos},
			script.EventTalked:     {eyeOfArgos, daimonAltar},
			script.EventDecayed:    {daimonTheWhiteEyed},
			script.EventMyDying:    {daimonTheWhiteEyed},
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
	// Eye of Argos.
	case strings.EqualFold(e.Name, "31683-03.htm"):
		if p.HasItem(unfinishedSummonCrystal) {
			st.SetStatus(questlog.StatusStarted)
			st.SetCond(1)
			s.PlaySound(p, script.SoundAccept)
			s.TakeItems(p, unfinishedSummonCrystal, 1)
			s.GiveItems(p, summonCrystal, 1)
		} else {
			htmltext = "31683-04.htm"
		}
	case strings.EqualFold(e.Name, "31683-08.htm"):
		if p.HasItem(essenceOfDaimon) {
			s.TakeItems(p, essenceOfDaimon, 1)
			rs := rewards()
			s.RewardItems(p, rs[s.Rnd(len(rs))], 5)
			s.PlaySound(p, script.SoundFinish)
			st.Exit(true)
		} else {
			htmltext = "31683-09.htm"
		}
	// Daimon's altar.
	case strings.EqualFold(e.Name, "31541-02.htm"):
		switch {
		case !p.HasItem(summonCrystal):
			htmltext = "31541-03.htm"
		case !q.raid.Empty():
			htmltext = "31541-04.htm"
		default:
			if !q.call(s, e.NPC) {
				htmltext = "31541-04.htm"
				break
			}
			st.SetCond(2)
			s.PlaySound(p, script.SoundMiddle)
			s.TakeItems(p, summonCrystal, 1)
		}
	}
	return htmltext
}

// call places the raid, has it speak and removes the altar that called it.
// It reports false when the raid could not be placed or when another
// player's call placed one meanwhile: the call is then refused as one made
// while the raid stood.
func (q quest) call(s *script.Script, altar *script.NPC) bool {
	at := script.Loc{X: 186320, Y: -43904, Z: -3175, Heading: s.Rnd(65536)}
	raid := s.Spawner().AddSpawn(daimonTheWhiteEyed, at, false, raidLifetime)
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
		if e.Player.Level() < 73 {
			htmltext = "31683-02.htm"
			st.Exit(true)
		} else {
			htmltext = "31683-01.htm"
		}
	case questlog.StatusStarted:
		cond := st.Cond()
		switch e.NPC.NpcID() {
		case eyeOfArgos:
			switch cond {
			case 1:
				htmltext = "31683-05.htm"
			case 2:
				htmltext = "31683-06.htm"
			default:
				htmltext = "31683-07.htm"
			}
		case daimonAltar:
			switch cond {
			case 1:
				htmltext = "31541-01.htm"
			case 2:
				htmltext = "31541-05.htm"
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
	s.GiveItems(st.Player(), essenceOfDaimon, 1)
}
