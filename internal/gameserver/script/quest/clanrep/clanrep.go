// Package clanrep holds the quests that raise a clan's reputation: the clan
// leader takes the quest, the leader's clan brings back proofs of the hunts
// it asks for, and each proof is paid in reputation points. The quests are
// one family in one package, whose literals and calls are checked as the
// union of its quests (#4712); each quest keeps its own dialogue in its own
// file.
package clanrep

import (
	"strconv"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/questlog"
	"github.com/fatal10110/acis_golang/internal/gameserver/script"
)

// valdis gives two of the quests.
const valdis = 31331

// clanQuestCompleted is the system message of the reputation a clan quest
// earned.
const clanQuestCompleted = 1777

// payClan raises the reputation of p's clan by points and tells p so.
func payClan(p *script.Player, points int32) {
	p.AddClanReputation(points)
	p.SystemMessageNumbers(clanQuestCompleted, points)
	p.BroadcastClanInfo()
}

// raid is one raid of a raid list: its boss, the proof it drops, the
// reputation range the proof is worth and where the radar marks the boss.
type raid struct {
	boss, item, minPoints, maxPoints int32
	mark                             loc
}

// loc is a radar mark.
type loc struct{ x, y, z int32 }

// raidList is the quest whose leader picks one raid boss of a list and is
// paid for each proof of its death.
type raidList struct {
	// raids are numbered from 1 by the "raid" variable.
	raids []raid
}

// start starts the quest on the raid number names and marks the boss on the
// radar. A number off the list panics once the quest has started.
func (q raidList) start(s *script.Script, p *script.Player, st *script.QuestState, number string) {
	st.SetStatus(questlog.StatusStarted)
	st.SetCond(1)
	st.Set("raid", number)
	s.PlaySound(p, script.SoundAccept)
	n, err := strconv.Atoi(number)
	if err != nil {
		panic(err)
	}
	mark := q.raids[n-1].mark
	p.AddRadarMarker(mark.x, mark.y, mark.z)
}

// pay takes one proof of the quest's raid for a random reputation of its
// range. It returns the raid's number, and whether p held a proof.
func (q raidList) pay(s *script.Script, p *script.Player, st *script.QuestState) (int32, bool) {
	n := st.GetInt("raid")
	r := q.raids[n-1]
	if !p.HasItem(r.item) {
		return n, false
	}
	points := r.minPoints + int32(s.Rnd(int(r.maxPoints-r.minPoints+1)))
	s.TakeItems(p, r.item, 1)
	payClan(p, points)
	return n, true
}

// onMyDying drops the proof to the killer's clan leader, when the leader
// stands near with the quest started on that boss.
func (q raidList) onMyDying(s *script.Script, e script.MyDying) {
	p := script.ActingPlayer(e.Killer)
	st := s.ClanLeaderQuestState(p, e.NPC)
	if st == nil || st.Status() != questlog.StatusStarted {
		return
	}
	n := st.GetInt("raid")
	if r := q.raids[n-1]; r.boss == e.NPC.NpcID() {
		s.DropItemsAlways(st.Player(), r.item, 1, 1)
	}
}
