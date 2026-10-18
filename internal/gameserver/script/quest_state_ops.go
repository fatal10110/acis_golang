package script

import (
	"fmt"
	"strconv"
	"strings"
	"time"
)

// The quest-state, party and academy reads and the memo states the freeze
// of the script-facing API adds (#3748).

// GetBool reports whether the variable key reads true, ignoring case; an
// unset variable, or any other value, reads false.
func (qs *QuestState) GetBool(key string) bool {
	v, ok := qs.state.Get(key)
	return ok && strings.EqualFold(v, "true")
}

// memoStateKey is the variable a quest's memo state slot is kept in.
func memoStateKey(slot int32) string { return "memoState-" + strconv.Itoa(int(slot)) }

// MemoState is MemoStateEx of slot 0.
func (s *Script) MemoState(p *Player, questID int32) int32 { return s.MemoStateEx(p, questID, 0) }

// MemoStateEx returns memo state slot of p's state in the quest questID, -1
// when it is unset. A player with no state in that quest panics, and so
// does a slot value that is not an int.
func (s *Script) MemoStateEx(p *Player, questID, slot int32) int32 {
	qs := s.questStateByID(p, questID)
	if qs == nil {
		panic(fmt.Sprintf("script: the memo state of quest %d the player has no state in", questID))
	}
	key := memoStateKey(slot)
	if _, ok := qs.Get(key); !ok {
		return -1
	}
	return qs.GetInt(key)
}

// SetMemoState is SetMemoStateEx of slot 0.
func (s *Script) SetMemoState(p *Player, questID, value int32) {
	s.SetMemoStateEx(p, questID, 0, value)
}

// SetMemoStateEx sets memo state slot of p's state in the quest questID to
// value. A player with no state in that quest is left as is.
func (s *Script) SetMemoStateEx(p *Player, questID, slot, value int32) {
	if qs := s.questStateByID(p, questID); qs != nil {
		qs.Set(memoStateKey(slot), strconv.Itoa(int(value)))
	}
}

// questStateByID returns p's first state whose quest has questID, nil when
// there is none.
func (s *Script) questStateByID(p *Player, questID int32) *QuestState {
	c := p.character()
	st := c.Quests().StateByID(questID)
	if st == nil {
		return nil
	}
	return (&QuestState{quests: s.env.Quests, player: c, state: st}).foundBy(s)
}

// ElapsedTicks returns the seconds the server has been up past tick, a
// count of seconds since it started.
func (s *Script) ElapsedTicks(tick int32) int32 {
	return int32(s.env.Uptime()/time.Second) - tick
}

// ItemStackable reports whether the item itemID stacks; an item with no
// template panics.
func (s *Script) ItemStackable(itemID int32) bool {
	if s.env != nil && s.env.Items != nil {
		if tmpl, ok := s.env.Items.Get(itemID); ok && tmpl != nil {
			return tmpl.Stackable
		}
	}
	panic(fmt.Sprintf("script: no item %d", itemID))
}

// FirstItemCount returns the count of the player's first instance of
// itemID, in inventory order. A player who holds none panics.
func (p *Player) FirstItemCount(itemID int32) int32 {
	if inv := p.character().Inventory(); inv != nil {
		if inst := inv.ItemByTemplateID(itemID); inst != nil {
			return int32(inst.CountValue())
		}
	}
	panic(fmt.Sprintf("script: the count of item %d the player does not hold", itemID))
}

// PartyLeader returns the leader of the player's party, nil when it is in
// none.
func (p *Player) PartyLeader() *Player {
	ref, ok := p.character().Party()
	if !ok {
		return nil
	}
	return leaderOf(ref)
}

// IsPartyLeader reports whether the player leads its party. A player in no
// party panics.
func (p *Player) IsPartyLeader() bool {
	leader := p.PartyLeader()
	if leader == nil {
		panic("script: the party leader of a player in no party")
	}
	return leader.ObjectID() == p.ObjectID()
}

// Sponsor returns the object id of the player's academy sponsor, 0 for
// none.
func (p *Player) Sponsor() int32 { return p.character().Sponsor() }

// Apprentice returns the object id of the player's academy apprentice, 0
// for none.
func (p *Player) Apprentice() int32 { return p.character().Apprentice() }

// mentorRange is how near an academy sponsor or apprentice must stand.
const mentorRange = 1500

// SponsorInRange reports whether the player's academy sponsor is a member of
// its clan in the world, strictly within 1500 of it.
func (s *Script) SponsorInRange(p *Player) bool {
	return s.clanMateInRange(p, p.Sponsor()) != nil
}

// ApprenticeInRange returns the player's academy apprentice when it is a
// member of its clan in the world, strictly within 1500 of it; nil
// otherwise.
func (s *Script) ApprenticeInRange(p *Player) *Player {
	return s.clanMateInRange(p, p.Apprentice())
}

// clanMateInRange returns the player objectID when it is in the world, in
// p's clan and strictly within mentorRange of p; nil otherwise.
func (s *Script) clanMateInRange(p *Player, objectID int32) *Player {
	clanID := p.ClanID()
	if objectID == 0 || clanID == 0 {
		return nil
	}
	mate := s.WorldPlayer(objectID)
	if mate == nil || mate.ClanID() != clanID || !InRadius3D(p, mate, mentorRange) {
		return nil
	}
	return mate
}
