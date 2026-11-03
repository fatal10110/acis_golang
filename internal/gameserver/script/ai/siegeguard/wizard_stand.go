package siegeguard

import (
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/npc"
	"github.com/fatal10110/acis_golang/internal/gameserver/script"
	"github.com/fatal10110/acis_golang/internal/gameserver/script/ai/defaultnpc"
)

// WizardStand is GludioWizardStand: a wizard that keeps its post and casts
// its DD magic, from where it stands, at the players and summons it sees,
// and sometimes at one attacking a clan mate; it spares its castle's
// defenders.
func WizardStand() script.Script {
	base := defaultnpc.New().Hooks
	return script.Script{Dir: dir, Behavior: true, NPCs: wizardStandIDs(), Hooks: base.With(script.Hooks{
		OnSeeCreature: func(s *script.Script, e script.SeeCreature) {
			if !spared(s, e.NPC, e.Creature) && e.Creature.Playable() {
				if dd, ok := e.NPC.SkillByType(npc.SkillTypeDDMagic); ok {
					e.NPC.AddCastDesireHold(e.Creature, dd, 1000000)
				}
			}
		},
		OnClanAttacked: func(s *script.Script, e script.ClanAttacked) {
			if !spared(s, e.Called, e.Attacker) && e.Attacker.Playable() && s.Rnd(100) < 10 {
				if dd, ok := e.Called.SkillByType(npc.SkillTypeDDMagic); ok {
					e.Called.AddCastDesireHold(e.Attacker, dd, 1000000)
				}
			}
		},
	})}
}

// wizardStandIDs are GludioWizardStand's own NPCs.
func wizardStandIDs() []int32 {
	return []int32{35014, 35024, 35034, 35044, 35054, 35080, 35122, 35164, 35206, 35249, 35296, 35340, 35485, 35532}
}
