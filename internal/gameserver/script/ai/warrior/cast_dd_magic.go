package warrior

import (
	"strings"
	"time"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/npc"
	"github.com/fatal10110/acis_golang/internal/gameserver/script"
	"github.com/fatal10110/acis_golang/internal/gameserver/script/ai/defaultnpc"
)

const castDDMagicDir = "ai/individual/Monster/WarriorBase/Warrior/WarriorCastDDMagic"

// CastDDMagic is WarriorCastDDMagic: a Warrior that may cast its DD magic
// on its most hated attacker and on a clan member's attacker standing
// away, and once hit, every 5 s while it fights, on the creature it hates
// most.
func CastDDMagic() script.Script {
	return script.Script{Dir: castDDMagicDir, Behavior: true, NPCs: castDDMagicIDs(), Hooks: CastDDMagicHooks()}
}

// castDDMagicIDs are WarriorCastDDMagic's own NPCs.
func castDDMagicIDs() []int32 {
	return []int32{
		20037, 20172, 20177, 21167, 21170, 21173, 21188, 21191, 21194, 21622, 21797, 20277, 20589, 20595,
		27126, 20590, 20786, 21644, 20153, 20244, 20245, 20842, 21620, 21621, 20648, 20678, 20997, 20785,
		20501, 20599,
	}
}

// CastDDMagicHooks are WarriorCastDDMagic's hooks, which its children
// derive theirs from.
func CastDDMagicHooks() script.Hooks {
	base := WarriorHooks()
	return base.With(script.Hooks{
		OnCreated: func(s *script.Script, e script.Created) {
			e.NPC.SetInt(script.SlotIntAI1, 0)
			e.NPC.SetInt(script.SlotIntAI2, 0)
			base.Created(s, e)
		},
		OnAttacked: func(s *script.Script, e script.Attacked) {
			n, attacker := e.NPC, e.Attacker
			mostHated := n.MostHated()
			armFightTimer(s, n, attacker, mostHated)
			if attacker.Playable() && script.Same(mostHated, attacker) {
				maybeDebuff(s, n, attacker, npc.SkillTypeDDMagic)
			}
			base.Attacked(s, e)
		},
		OnClanAttacked: func(s *script.Script, e script.ClanAttacked) {
			called, attacker := e.Called, e.Attacker
			if attacker.Playable() && called.LifeTime() > 7 && called.Intention() != script.IntentionAttack {
				if s.Rnd(100) < 33 && called.Distance2D(attacker) > 100 {
					addCastByType(called, attacker, npc.SkillTypeDDMagic, 1000000)
				}
			}
			base.ClanAttacked(s, e)
		},
		OnTimer: func(s *script.Script, e script.Timer) string {
			if strings.EqualFold(e.Name, "2001") {
				n := e.NPC
				if !fighting(n.Intention()) {
					n.SetInt(script.SlotIntAI1, 0)
					n.SetInt(script.SlotIntAI2, 0)
					return base.Timer(s, e)
				}
				fightTick(s, n, npc.SkillTypeDDMagic)
			}
			return base.Timer(s, e)
		},
	}).Named("WarriorCastDDMagic")
}

// armFightTimer is the hit half of the 5 s fight timer "2001": the first
// hit arms it (i_ai1); a later hit by the creature n hates most marks the
// next tick to cast nothing (i_ai2).
func armFightTimer(s *script.Script, n *script.NPC, attacker, mostHated script.Creature) {
	if n.Int(script.SlotIntAI1) == 1 {
		if script.Same(mostHated, attacker) {
			n.SetInt(script.SlotIntAI2, 1)
		}
		return
	}
	s.StartTimer("2001", n, nil, 5000*time.Millisecond)
	n.SetInt(script.SlotIntAI1, 1)
}

// fightTick is a tick of the fight timer "2001" while n fights: unless the
// tick is marked, n may want, on a draw under 50, to cast its template's
// skill of type t on the creature it hates most when that creature holds
// no effect of the skill's stack, with weight 1000000. The timer then
// re-arms and the mark clears.
func fightTick(s *script.Script, n *script.NPC, t npc.SkillType) {
	if n.Int(script.SlotIntAI2) == 0 {
		if mostHated := n.MostHated(); mostHated != nil && s.Rnd(100) < 50 {
			ref, ok := n.SkillByType(t)
			if s.AbnormalLevel(mostHated, ref) <= 0 && ok {
				n.AddCastDesire(mostHated, ref, 1000000)
			}
		}
	}
	s.StartTimer("2001", n, nil, 5000*time.Millisecond)
	n.SetInt(script.SlotIntAI2, 0)
}

// CastDDMagicAggressive is WarriorCastDDMagicAggressive: a
// WarriorCastDDMagic that attacks the players and summons it sees, and,
// settled in its territory and not yet attacking, may first cast its DD
// magic on one standing away.
func CastDDMagicAggressive() script.Script {
	return script.Script{Dir: castDDMagicDir, Behavior: true, NPCs: castDDMagicAggressiveIDs(), Hooks: CastDDMagicAggressiveHooks()}
}

// castDDMagicAggressiveIDs are WarriorCastDDMagicAggressive's own NPCs.
func castDDMagicAggressiveIDs() []int32 {
	return []int32{
		21421, 20635, 20670, 21569, 20664, 20137, 20668, 21239, 21243, 20146, 21417, 20656, 20596, 20680,
		20227, 21223, 21231, 21227, 20235, 20279, 21247, 21255, 21251, 20110, 20113, 20115, 21215, 21219,
		21211, 20935, 20605, 20176, 20647, 20194, 20347, 20354, 20431, 20453, 20513, 20546, 21000, 21143,
		21146, 21155, 21162, 21235, 20412,
	}
}

// CastDDMagicAggressiveHooks are WarriorCastDDMagicAggressive's hooks,
// which its children derive theirs from.
func CastDDMagicAggressiveHooks() script.Hooks {
	base := CastDDMagicHooks()
	return base.With(script.Hooks{
		OnSeeCreature: func(s *script.Script, e script.SeeCreature) {
			n, c := e.NPC, e.Creature
			if !c.Playable() {
				return
			}
			if n.LifeTime() > 7 && n.Intention() != script.IntentionAttack && n.InMyTerritory() {
				if s.Rnd(100) < 33 && n.Distance2D(c) > 100 {
					addCastByType(n, c, npc.SkillTypeDDMagic, 1000000)
				}
			}
			defaultnpc.TryToAttack(s, n, c)
			base.SeeCreature(s, e)
		},
	}).Named("WarriorCastDDMagicAggressive")
}

// CastDDMagicSelfBuff is WarriorCastDDMagicSelfBuff: a WarriorCastDDMagic
// that buffs itself as it spawns, and again when a player or summon hits
// it while it neither attacks nor casts and the buff has worn off.
func CastDDMagicSelfBuff() script.Script {
	return script.Script{Dir: castDDMagicDir, Behavior: true, NPCs: []int32{21405}, Hooks: CastDDMagicSelfBuffHooks()}
}

// CastDDMagicSelfBuffHooks are WarriorCastDDMagicSelfBuff's hooks, which
// its children derive theirs from.
func CastDDMagicSelfBuffHooks() script.Hooks {
	base := CastDDMagicHooks()
	return base.With(script.Hooks{
		OnCreated: func(s *script.Script, e script.Created) {
			addCastByType(e.NPC, e.NPC, npc.SkillTypeSelfBuff, 1000000)
			base.Created(s, e)
		},
		OnAttacked: func(s *script.Script, e script.Attacked) {
			n := e.NPC
			if e.Attacker.Playable() {
				if !fighting(n.Intention()) {
					selfBuff, ok := n.SkillByType(npc.SkillTypeSelfBuff)
					if s.AbnormalLevel(n, selfBuff) <= 0 && ok {
						n.AddCastDesire(n, selfBuff, 1000000)
					}
				}
			}
			base.Attacked(s, e)
		},
	}).Named("WarriorCastDDMagicSelfBuff")
}

// CastDDMagicSelfBuffAggressive is WarriorCastDDMagicSelfBuffAggressive: a
// WarriorCastDDMagicSelfBuff that attacks the players and summons it sees.
// Seeing any creature while it neither attacks nor casts, it buffs itself
// when the creature it sees, not itself, holds no effect of the buff's
// stack. It binds no NPC of its own.
func CastDDMagicSelfBuffAggressive() script.Script {
	base := CastDDMagicSelfBuffHooks()
	return script.Script{Dir: castDDMagicDir, Behavior: true, Hooks: base.With(script.Hooks{
		OnSeeCreature: func(s *script.Script, e script.SeeCreature) {
			n, c := e.NPC, e.Creature
			if !fighting(n.Intention()) {
				selfBuff, ok := n.SkillByType(npc.SkillTypeSelfBuff)
				if s.AbnormalLevel(c, selfBuff) <= 0 && ok {
					n.AddCastDesire(n, selfBuff, 1000000)
				}
			}
			if !c.Playable() {
				return
			}
			defaultnpc.TryToAttack(s, n, c)
			base.SeeCreature(s, e)
		},
	})}
}
