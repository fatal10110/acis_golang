package script

import (
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/attackable"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/questlog"
)

// NewQuestState adds to p's journal a state of the script, created, and
// returns it. It writes nothing.
func (s *Script) NewQuestState(p *Player) *QuestState {
	return s.env.Quests.NewState(p.character(), s)
}

// IsCompleted reports whether the state's status is completed.
func (qs *QuestState) IsCompleted() bool { return qs.Status() == questlog.StatusCompleted }

// WorldNPC returns an NPC of npcID in the world, the first one met; nil
// when there is none.
func (s *Script) WorldNPC(npcID int32) *NPC {
	if s.env.World == nil {
		return nil
	}
	for _, obj := range s.env.World.Objects() {
		c, ok := obj.(attackable.Combatant)
		if !ok {
			continue
		}
		if n := NPCOf(c); n != nil && n.NpcID() == npcID {
			return n
		}
	}
	return nil
}
