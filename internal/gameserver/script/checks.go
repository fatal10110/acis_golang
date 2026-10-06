package script

import (
	"strings"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/player"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/questlog"
)

// The quest-state checks find a player's state in the script's quest and
// keep it only when it matches and the player stands strictly within the
// party range of the NPC, centre to centre in 3D. A nil player or NPC
// matches nothing.

// CheckPlayerCondition returns p's state when its condition is cond.
func (s *Script) CheckPlayerCondition(p *Player, npc *NPC, cond int32) *QuestState {
	if p == nil {
		return nil
	}
	return s.checkCondition(p.character(), npc, cond)
}

// CheckPlayerVariable returns p's state when its variable key equals
// value, ignoring case.
func (s *Script) CheckPlayerVariable(p *Player, npc *NPC, key, value string) *QuestState {
	if p == nil {
		return nil
	}
	return s.checkVariable(p.character(), npc, key, value)
}

// CheckPlayerState returns p's state when its status is status.
func (s *Script) CheckPlayerState(p *Player, npc *NPC, status questlog.Status) *QuestState {
	if p == nil {
		return nil
	}
	return s.checkStatus(p.character(), npc, status)
}

// Match is what a party lookup asks of each member's quest state: a
// condition (ByCond) or a variable's value (ByVar).
type Match struct {
	cond       int32
	key, value string
	byVar      bool
}

// ByCond matches a state whose condition is cond.
func ByCond(cond int32) Match { return Match{cond: cond} }

// ByVar matches a state whose variable key equals value, ignoring case.
func ByVar(key, value string) Match { return Match{key: key, value: value, byVar: true} }

// PartyMembers returns the states of p's party members, in party order,
// that match m as CheckPlayerCondition or CheckPlayerVariable do; p's own
// alone when p is in no party.
func (s *Script) PartyMembers(p *Player, npc *NPC, m Match) []*QuestState {
	return s.partyStates(p, func(c *player.Character) *QuestState {
		if m.byVar {
			return s.checkVariable(c, npc, m.key, m.value)
		}
		return s.checkCondition(c, npc, m.cond)
	})
}

// PartyMembersState is PartyMembers checked as CheckPlayerState.
func (s *Script) PartyMembersState(p *Player, npc *NPC, status questlog.Status) []*QuestState {
	return s.partyStates(p, func(c *player.Character) *QuestState { return s.checkStatus(c, npc, status) })
}

// RandomPartyMember returns one of PartyMembers' states at random, nil
// when there is none.
func (s *Script) RandomPartyMember(p *Player, npc *NPC, m Match) *QuestState {
	return s.pick(s.PartyMembers(p, npc, m))
}

// RandomPartyMemberState is RandomPartyMember over PartyMembersState.
func (s *Script) RandomPartyMemberState(p *Player, npc *NPC, status questlog.Status) *QuestState {
	return s.pick(s.PartyMembersState(p, npc, status))
}

// ClanLeaderQuestState returns the state of p's clan leader in the
// script's quest: p's own when p leads its clan, else the leader's when the
// leader is in the world. With an NPC, p and then the leader must stand
// strictly within the party range of it.
func (s *Script) ClanLeaderQuestState(p *Player, npc *NPC) *QuestState {
	if p == nil {
		return nil
	}
	c := p.character()
	if npc != nil && !s.inRange(c, npc) {
		return nil
	}
	if c.IsClanLeader() {
		return s.env.Quests.State(c, s)
	}
	leader, ok := c.OnlineClanLeader()
	if !ok {
		return nil
	}
	if npc != nil && !s.inRange(leader, npc) {
		return nil
	}
	return s.env.Quests.State(leader, s)
}

// CheckClanLeaderCondition returns ClanLeaderQuestState when its
// condition is cond.
func (s *Script) CheckClanLeaderCondition(p *Player, npc *NPC, cond int32) *QuestState {
	st := s.ClanLeaderQuestState(p, npc)
	if st == nil || st.Cond() != cond {
		return nil
	}
	return st
}

// CheckClanLeaderVariable returns ClanLeaderQuestState when its variable
// key equals value, ignoring case.
func (s *Script) CheckClanLeaderVariable(p *Player, npc *NPC, key, value string) *QuestState {
	st := s.ClanLeaderQuestState(p, npc)
	if st == nil || !st.varIs(key, value) {
		return nil
	}
	return st
}

// CheckClanLeaderState returns ClanLeaderQuestState when its status is
// status.
func (s *Script) CheckClanLeaderState(p *Player, npc *NPC, status questlog.Status) *QuestState {
	st := s.ClanLeaderQuestState(p, npc)
	if st == nil || st.Status() != status {
		return nil
	}
	return st
}

func (s *Script) checkCondition(c *player.Character, npc *NPC, cond int32) *QuestState {
	return s.check(c, npc, func(st *QuestState) bool { return st.Cond() == cond })
}

func (s *Script) checkVariable(c *player.Character, npc *NPC, key, value string) *QuestState {
	return s.check(c, npc, func(st *QuestState) bool { return st.varIs(key, value) })
}

func (s *Script) checkStatus(c *player.Character, npc *NPC, status questlog.Status) *QuestState {
	return s.check(c, npc, func(st *QuestState) bool { return st.Status() == status })
}

// check returns c's state when match accepts it and c stands within the
// party range of npc.
func (s *Script) check(c *player.Character, npc *NPC, match func(*QuestState) bool) *QuestState {
	if npc == nil {
		return nil
	}
	st := s.env.Quests.State(c, s)
	if st == nil || !match(st) || !s.inRange(c, npc) {
		return nil
	}
	return st
}

// partyStates returns the states check keeps of p's party members, in
// party order, or of p alone when p is in no party.
func (s *Script) partyStates(p *Player, check func(*player.Character) *QuestState) []*QuestState {
	if p == nil {
		return nil
	}
	c := p.character()
	members := c.PartyMembers()
	if members == nil {
		members = []*player.Character{c}
	}
	var out []*QuestState
	for _, m := range members {
		if st := check(m); st != nil {
			out = append(out, st)
		}
	}
	return out
}

// pick returns one of states at random, nil with no draw when there is
// none.
func (s *Script) pick(states []*QuestState) *QuestState {
	if len(states) == 0 {
		return nil
	}
	return states[s.env.Rand(len(states))]
}

// inRange reports whether c stands strictly within the party range of npc.
// A handle on nothing panics.
func (s *Script) inRange(c *player.Character, npc *NPC) bool {
	at := npc.combatant()
	if at == nil {
		panic("script: the place of a handle on nothing")
	}
	nx, ny, nz := at.Position()
	x, y, z := c.Position()
	return location.In3DRadius(x, y, z, nx, ny, nz, s.env.PartyRange)
}

// varIs reports whether the state's variable key is set and equals value,
// ignoring case.
func (qs *QuestState) varIs(key, value string) bool {
	v, ok := qs.Get(key)
	return ok && strings.EqualFold(value, v)
}
