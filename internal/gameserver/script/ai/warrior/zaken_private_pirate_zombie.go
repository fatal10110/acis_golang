package warrior

import (
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/npc"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/script"
	"github.com/fatal10110/acis_golang/internal/gameserver/script/ai/defaultnpc"
)

// ZakenPrivatePirateZombie is ZakenPrivatePirateZombie: Zaken's pirate
// zombie and its captain, a WarriorPhysicalSpecialAggressive that keeps
// where it was created, only notices the players and summons within 100
// of its height, attacking them in its territory, and curses an attacker,
// and mutes a caster, more than 8 levels above it.
//
// Its memory: WeightPoint, RespawnTime and Flag hold the x, y and z it was
// created at.
func ZakenPrivatePirateZombie() script.Script {
	base := PhysicalSpecialAggressiveHooks()
	return script.Script{Dir: physicalSpecialDir, Behavior: true, NPCs: []int32{29026, 29027}, Hooks: base.With(script.Hooks{
		OnCreated: func(_ *script.Script, e script.Created) {
			at := script.LocOf(e.NPC)
			e.NPC.SetInt(script.SlotWeightPoint, int32(at.X))
			e.NPC.SetInt(npc.IntRespawnTime, int32(at.Y))
			e.NPC.SetInt(script.SlotFlag, int32(at.Z))
		},
		OnSeeCreature: func(s *script.Script, e script.SeeCreature) {
			n, c := e.NPC, e.Creature
			z, own := script.LocOf(c).Z, script.LocOf(n).Z
			if z > own-100 && z < own+100 {
				if !c.Playable() {
					return
				}
				if n.InMyTerritory() {
					n.AddAttackDesire(c, 200)
				}
				defaultnpc.MakeAttackEvent(s, n, c, 9, true)
			}
		},
		OnAttacked: func(s *script.Script, e script.Attacked) {
			if e.Attacker.Level() > e.NPC.Level()+8 {
				e.NPC.AddCastDesire(e.Attacker, skill.Ref{ID: 4515, Level: 1}, 1000000)
			}
			base.Attacked(s, e)
		},
		OnSeeSpell: func(s *script.Script, e script.SeeSpell) {
			if e.Caster.Level() > e.NPC.Level()+8 {
				e.NPC.AddCastDesire(e.Caster, skill.Ref{ID: 4215, Level: 1}, 1000000)
				return
			}
			base.SeeSpell(s, e)
		},
	})}
}
