package script

import (
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/attackable"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/npc"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/questlog"
	"github.com/fatal10110/acis_golang/internal/gameserver/world"
)

// NewQuestState adds to p's journal a state of the script, created, and
// returns it. It writes nothing.
func (s *Script) NewQuestState(p *Player) *QuestState {
	return s.env.Quests.NewState(p.character(), s)
}

// IsCompleted reports whether the state's status is completed.
func (qs *QuestState) IsCompleted() bool { return qs.Status() == questlog.StatusCompleted }

// WorldNPC returns an NPC of npcID in the world, the first one met; nil
// when there is none. The scan copies nothing and wraps only the NPC it
// returns.
func (s *Script) WorldNPC(npcID int32) *NPC {
	if s.env.World == nil {
		return nil
	}
	found := s.env.World.FindObject(func(obj world.Tracked) bool {
		switch o := obj.(type) {
		case *npc.Hostile:
			return int32(o.NpcID()) == npcID
		case *npc.Folk:
			return int32(o.NpcID()) == npcID
		}
		return false
	})
	if found == nil {
		return nil
	}
	return NPCOf(found.(attackable.Combatant))
}
