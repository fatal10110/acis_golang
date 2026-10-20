package warrior

import (
	"strings"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/npc"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/script"
	"github.com/fatal10110/acis_golang/internal/gameserver/script/ai/defaultnpc"
)

const castingEnchantDir = "ai/individual/Monster/WarriorBase/Warrior/WarriorCastingEnchant"

// CastingEnchant is WarriorCastingEnchant: a Warrior that, still above
// half its hit points, may once buff itself when a player or summon first
// hits it.
func CastingEnchant() script.Script {
	return script.Script{Dir: castingEnchantDir, Behavior: true, NPCs: castingEnchantIDs(), Hooks: CastingEnchantHooks()}
}

// castingEnchantIDs are WarriorCastingEnchant's own NPCs.
func castingEnchantIDs() []int32 {
	return []int32{20681, 21138, 21168, 21171, 21174, 21189, 21192, 21195, 20569, 20552, 20550, 20675, 20586, 22004, 22085}
}

// CastingEnchantHooks are WarriorCastingEnchant's hooks, which its
// children derive theirs from.
func CastingEnchantHooks() script.Hooks {
	base := WarriorHooks()
	return base.With(script.Hooks{
		OnCreated: func(s *script.Script, e script.Created) {
			e.NPC.SetInt(script.SlotIntAI0, 0)
			e.NPC.SetInt(script.SlotIntAI1, 0)
			base.Created(s, e)
		},
		OnAttacked: func(s *script.Script, e script.Attacked) {
			n := e.NPC
			if e.Attacker.Playable() && n.Int(script.SlotIntAI1) == 0 && s.Rnd(100) < 33 && n.HPRatio() > 0.5 {
				addCastByType(n, n, npc.SkillTypeBuff, 1000000)
				n.SetInt(script.SlotIntAI1, 1)
			}
			base.Attacked(s, e)
		},
	}).Named("WarriorCastingEnchant")
}

// CastingEnchantClan is WarriorCastingEnchantClan: a WarriorCastingEnchant
// that, settled, not attacking and above half its hit points, may once
// buff a clan member a player or summon hits, unless the buff already
// holds on the member.
func CastingEnchantClan() script.Script {
	return script.Script{Dir: castingEnchantDir, Behavior: true, NPCs: castingEnchantClanIDs(), Hooks: CastingEnchantClanHooks()}
}

// castingEnchantClanIDs are WarriorCastingEnchantClan's own NPCs.
func castingEnchantClanIDs() []int32 {
	return []int32{21616, 21634, 20081, 20794, 20840, 21614, 21615, 20846, 21632, 21633, 21010, 20643, 20593, 20682}
}

// CastingEnchantClanHooks are WarriorCastingEnchantClan's hooks, which its
// children derive theirs from.
func CastingEnchantClanHooks() script.Hooks {
	base := CastingEnchantHooks()
	return base.With(script.Hooks{
		OnClanAttacked: func(s *script.Script, e script.ClanAttacked) {
			caller, called := e.Caller, e.Called
			if e.Attacker.Playable() && called.LifeTime() > 7 && called.Intention() != script.IntentionAttack &&
				called.Int(script.SlotIntAI1) == 0 && s.Rnd(100) < 50 && called.HPRatio() > 0.5 {
				buff, ok := called.SkillByType(npc.SkillTypeBuff)
				if s.AbnormalLevel(caller, buff) <= 0 && ok {
					called.AddCastDesire(caller, buff, 1000000)
				}
			}
			// Any clan call spends the buff, whether or not it was cast.
			called.SetInt(script.SlotIntAI1, 1)
			base.ClanAttacked(s, e)
		},
	}).Named("WarriorCastingEnchantClan")
}

// CastingEnchantClanAggressive is WarriorCastingEnchantClanAggressive: a
// WarriorCastingEnchantClan that, settled in its territory, may once buff
// itself on sight of any creature, and attacks the players and summons it
// sees.
func CastingEnchantClanAggressive() script.Script {
	return script.Script{Dir: castingEnchantDir, Behavior: true, NPCs: castingEnchantClanAggressiveIDs(), Hooks: CastingEnchantClanAggressiveHooks()}
}

// castingEnchantClanAggressiveIDs are WarriorCastingEnchantClanAggressive's
// own NPCs.
func castingEnchantClanAggressiveIDs() []int32 {
	return []int32{
		21237, 21241, 20789, 20804, 20806, 20791, 21245, 21253, 21249, 21004, 20803, 21002, 21005, 20834,
		21213, 21217, 21209, 21221, 21229, 21225, 20793, 20808, 20066, 20571, 20076, 21233,
	}
}

// CastingEnchantClanAggressiveHooks are
// WarriorCastingEnchantClanAggressive's hooks, which its children derive
// theirs from.
func CastingEnchantClanAggressiveHooks() script.Hooks {
	base := CastingEnchantClanHooks()
	return base.With(script.Hooks{
		OnSeeCreature: func(s *script.Script, e script.SeeCreature) {
			n, c := e.NPC, e.Creature
			if n.LifeTime() > 7 && n.InMyTerritory() {
				if n.Int(script.SlotIntAI1) == 0 && s.Rnd(100) < 33 {
					addCastByType(n, n, npc.SkillTypeBuff, 1000000)
				}
				n.SetInt(script.SlotIntAI1, 1)
			}
			// A creature other than a player or summon reaches no parent.
			if !c.Playable() {
				return
			}
			defaultnpc.TryToAttack(s, n, c)
			base.SeeCreature(s, e)
		},
	}).Named("WarriorCastingEnchantClanAggressive")
}

// OrfenPrivateRaikel is OrfenPrivateRaikel: Orfen's Raikel, a
// WarriorCastingEnchantClan that curses an attacker, and mutes a caster,
// more than 8 levels above it, and every 90 to 330 s may shuffle its hate.
// A hit adds no hate.
func OrfenPrivateRaikel() script.Script {
	base := CastingEnchantClanHooks()
	return script.Script{Dir: castingEnchantDir, Behavior: true, NPCs: []int32{29015}, Hooks: base.With(script.Hooks{
		OnCreated: func(s *script.Script, e script.Created) {
			s.StartTimer("5001", e.NPC, nil, coreShuffleDelay(s))
			base.Created(s, e)
		},
		OnTimer: func(s *script.Script, e script.Timer) string {
			if strings.EqualFold(e.Name, "5001") {
				if s.Rnd(100) < 66 {
					e.NPC.RandomizeAttack()
				}
				s.StartTimer("5001", e.NPC, nil, coreShuffleDelay(s))
			}
			// The parent runs the timer with no player.
			return base.Timer(s, script.Timer{Name: e.Name, NPC: e.NPC})
		},
		OnAttacked: func(_ *script.Script, e script.Attacked) {
			if e.Attacker.Level() > e.NPC.Level()+8 {
				e.NPC.AddCastDesire(e.Attacker, skill.Ref{ID: 4515, Level: 1}, 1000000)
			}
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
