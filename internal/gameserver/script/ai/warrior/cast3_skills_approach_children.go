package warrior

import (
	"slices"

	"github.com/fatal10110/acis_golang/internal/gameserver/script"
	"github.com/fatal10110/acis_golang/internal/gameserver/script/ai/defaultnpc"
)

// Cast3SkillsApproachRevivalAggressive is
// WarriorCast3SkillsApproachRevivalAggressive: a
// WarriorCast3SkillsApproachAggressive that, dying, leaves its silhouette
// where it fell, told (spawn parameters 1000 and the killer) to turn on
// its killer.
//
// Its memory: i_ai4 holds the player or summon it most wants to act on
// after its last hit, 0 until one.
func Cast3SkillsApproachRevivalAggressive() script.Script {
	base := Cast3SkillsApproachAggressiveHooks()
	return script.Script{Dir: cast3ApproachDir, Behavior: true, NPCs: []int32{20830, 21067, 21062, 20831, 21070, 20858}, Hooks: base.With(script.Hooks{
		OnAttacked: func(s *script.Script, e script.Attacked) {
			base.Attacked(s, e)
			if topDesireTarget := e.NPC.TopDesireTarget(); topDesireTarget != nil && topDesireTarget.Playable() {
				e.NPC.SetInt(script.SlotIntAI4, topDesireTarget.ObjectID())
			}
		},
		OnCreated: func(s *script.Script, e script.Created) {
			e.NPC.SetInt(script.SlotIntAI4, 0)
			base.Created(s, e)
		},
		OnMyDying: func(s *script.Script, e script.MyDying) {
			n := e.NPC
			// A death with no killer names no one to turn on and leaves
			// nothing.
			if n.Int(script.SlotIntAI4) != -1 && e.Killer != nil {
				params := script.Params{P1: 1000, P2: e.Killer.ObjectID(), P3: 0}
				s.Spawner().CreateOnePrivateEx(n, n.IntParam("silhouette"), script.LocOf(n), 0, params)
			}
		},
	})}
}

// Cast3SkillsApproachStoneAggressive is
// WarriorCast3SkillsApproachStoneAggressive: a Ketra or Varka
// WarriorCast3SkillsApproach that attacks the players and summons it sees
// unless allied with its own faction, and petrifies such an ally who
// attacks it or its clan, or who helps another.
func Cast3SkillsApproachStoneAggressive() script.Script {
	base := Cast3SkillsApproachHooks()
	return script.Script{Dir: cast3ApproachDir, Behavior: true, NPCs: []int32{21335, 21324, 21350, 21361}, Hooks: base.With(script.Hooks{
		OnAttacked: func(s *script.Script, e script.Attacked) {
			if defaultnpc.MaybeCastPetrify(e.NPC, e.Attacker) {
				return
			}
			base.Attacked(s, e)
		},
		OnClanAttacked: func(s *script.Script, e script.ClanAttacked) {
			if defaultnpc.MaybeCastPetrify(e.Called, e.Attacker) {
				return
			}
			base.ClanAttacked(s, e)
		},
		OnSeeCreature: func(s *script.Script, e script.SeeCreature) {
			n := e.NPC
			p := script.ActingPlayer(e.Creature)
			if p == nil {
				return
			}
			if p.AlliedWithVarka() && slices.Contains(n.Clans(), "varka_silenos_clan") {
				return
			}
			if p.AlliedWithKetra() && slices.Contains(n.Clans(), "ketra_orc_clan") {
				return
			}
			defaultnpc.TryToAttack(s, n, e.Creature)
			base.SeeCreature(s, e)
		},
		OnSeeSpell: func(s *script.Script, e script.SeeSpell) {
			if k := s.Skill(e.Skill); k.AggroPoints() > 0 && !k.Offensive() && defaultnpc.MaybeCastPetrify(e.NPC, e.Caster) {
				return
			}
			base.SeeSpell(s, e)
		},
	})}
}
