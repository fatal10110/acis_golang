// Package raidaltar holds the quests whose altar calls a raid: a player spends
// the quest's token on an altar, the altar is removed and the raid takes its
// place for a while, and the one who slays it earns the trophy. The quests are
// one family in one package, checked against the union of their reference
// classes (#4712); each quest keeps its own dialogue in its own file.
package raidaltar

import (
	"time"

	"github.com/fatal10110/acis_golang/internal/gameserver/script"
)

// raidLifetime is how long the called raid stays.
const raidLifetime = 1200000 * time.Millisecond

// site is the altar of one quest and the raid it calls, shared by every
// player: one raid at a time, until it decays.
type site struct {
	raidNPC int32
	at      script.Loc
	// called and left are the NpcStrings the raid speaks: when called, and
	// when it leaves unslain.
	called, left int32
	trophy       int32 // the item the raid's death gives its slayer

	altar, raid *script.NPCCell
}

func newSite(raidNPC int32, at script.Loc, called, left, trophy int32) site {
	return site{
		raidNPC: raidNPC, at: at, called: called, left: left, trophy: trophy,
		altar: new(script.NPCCell), raid: new(script.NPCCell),
	}
}

// call places the raid, has it speak and removes the altar that called it.
// It reports false when the raid could not be placed or when another
// player's call placed one meanwhile: the call is then refused as one made
// while the raid stood.
func (q site) call(s *script.Script, altar *script.NPC) bool {
	at := q.at
	at.Heading = s.Rnd(65536)
	raid := s.Spawner().AddSpawn(q.raidNPC, at, false, raidLifetime)
	if raid == nil {
		return false
	}
	if !q.raid.SetIfEmpty(raid) {
		raid.DeleteMe()
		return false
	}
	raid.Say(script.NpcString(q.called))
	q.altar.Set(altar)
	altar.DeleteMe()
	return true
}

// onDecayed lets the altar call another raid once the raid has left. A raid
// is dead already when its decayed hooks run, so the branch that brings the
// altar back and has the raid speak never runs: the altar comes back with its
// own respawn.
func (q site) onDecayed(s *script.Script, e script.Decayed) {
	if !q.raid.ClearIf(e.NPC) {
		return
	}
	altar := q.altar.Get()
	if !e.NPC.Dead() {
		if altar != nil {
			s.Spawner().DoRespawn(altar)
		}
		e.NPC.Say(script.NpcString(q.left))
	}
	q.altar.ClearIf(altar)
}

func (q site) onMyDying(s *script.Script, e script.MyDying) {
	st := s.RandomPartyMember(script.ActingPlayer(e.Killer), e.NPC, script.ByCond(2))
	if st == nil {
		return
	}
	st.SetCond(3)
	s.PlaySound(st.Player(), script.SoundMiddle)
	s.GiveItems(st.Player(), q.trophy, 1)
}
