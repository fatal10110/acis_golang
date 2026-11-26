package warrior

import (
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/npc"
	"github.com/fatal10110/acis_golang/internal/gameserver/script"
	"github.com/fatal10110/acis_golang/internal/gameserver/script/ai/defaultnpc"
)

// ZakenPrivateDollBlader is ZakenPrivateDollBlader: Zaken's Doll Blader, a
// WarriorCastingEnchantClanAggressive that keeps where it was created, only
// notices the players and summons within 100 of its height, attacking
// them in its territory, and curses an attacker, and mutes a caster, more
// than 8 levels above it.
//
// Its memory: WeightPoint, RespawnTime and Flag hold the x, y and z it was
// created at.
func ZakenPrivateDollBlader() script.Script {
	base := CastingEnchantClanAggressiveHooks()
	return script.Script{Dir: castingEnchantDir, Behavior: true, NPCs: []int32{29023}, Hooks: base.With(script.Hooks{
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
			raidCurse(e.NPC, e.Attacker)
			base.Attacked(s, e)
		},
		OnSeeSpell: func(s *script.Script, e script.SeeSpell) {
			if raidMute(e.NPC, e.Caster) {
				return
			}
			base.SeeSpell(s, e)
		},
	})}
}
