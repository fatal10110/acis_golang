package siegeguard

import (
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/npc"
	"github.com/fatal10110/acis_golang/internal/gameserver/script"
	"github.com/fatal10110/acis_golang/internal/gameserver/script/ai/defaultnpc"
)

// Cleric is GludioCleric: when a clan mate below 60% of its hit points is
// attacked, it may walk over and heal it.
func Cleric() script.Script {
	base := defaultnpc.New().Hooks
	return script.Script{Dir: dir, Behavior: true, NPCs: clericIDs(), Hooks: base.With(script.Hooks{
		OnClanAttacked: func(s *script.Script, e script.ClanAttacked) {
			if e.Caller.HPRatio() < 0.6 && s.Rnd(100) < 20 {
				if heal, ok := e.Called.SkillByType(npc.SkillTypeMagicHeal); ok {
					e.Called.AddCastDesire(e.Caller, heal, 1000000)
				}
			}
		},
	})}
}

// clericIDs are GludioCleric's own NPCs.
func clericIDs() []int32 {
	return []int32{35018, 35028, 35038, 35048, 35058, 35068, 35110, 35152, 35194, 35237, 35284, 35328, 35473, 35520}
}

// ClericStand is GludioClericStand: a cleric that keeps its post, healing
// a hurt clan mate only from where it stands.
func ClericStand() script.Script {
	base := defaultnpc.New().Hooks
	return script.Script{Dir: dir, Behavior: true, NPCs: clericStandIDs(), Hooks: base.With(script.Hooks{
		OnClanAttacked: func(s *script.Script, e script.ClanAttacked) {
			if e.Caller.HPRatio() < 0.6 && s.Rnd(100) < 20 {
				if heal, ok := e.Called.SkillByType(npc.SkillTypeMagicHeal); ok {
					e.Called.AddCastDesireHold(e.Caller, heal, 1000000)
				}
			}
		},
	})}
}

// clericStandIDs are GludioClericStand's own NPCs.
func clericStandIDs() []int32 {
	return []int32{35013, 35023, 35033, 35043, 35053, 35081, 35123, 35165, 35207, 35250, 35297, 35341, 35486, 35533}
}
