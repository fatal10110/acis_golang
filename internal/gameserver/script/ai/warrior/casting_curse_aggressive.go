package warrior

import (
	"strings"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/npc"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/script"
	"github.com/fatal10110/acis_golang/internal/gameserver/script/ai/defaultnpc"
)

const castingCurseAggressiveDir = "ai/individual/Monster/WarriorBase/Warrior/WarriorCastingCurse/WarriorCastingCurseAggressive"

// CastingCurseAggressive is WarriorCastingCurseAggressive: a
// WarriorCastingCurse that attacks the players and summons it sees, and,
// settled and wandering in its territory, may first lay its debuff on one.
func CastingCurseAggressive() script.Script {
	return script.Script{Dir: castingCurseAggressiveDir, Behavior: true, NPCs: castingCurseAggressiveIDs(), Hooks: CastingCurseAggressiveHooks()}
}

// castingCurseAggressiveIDs are WarriorCastingCurseAggressive's own NPCs.
func castingCurseAggressiveIDs() []int32 {
	return []int32{
		20087, 20043, 20796, 20345, 20649, 20145, 20676, 20671, 20652, 20787, 20848, 20849, 20036, 20233,
		20050, 20561, 20839, 21611, 21612, 20229, 20338, 20383, 20490, 20928, 21012, 21097, 21145, 21154,
		21157, 21161, 21163, 21298, 21613, 21670, 21693, 21716, 21739, 21762, 21785,
	}
}

// CastingCurseAggressiveHooks are WarriorCastingCurseAggressive's hooks,
// which its children derive theirs from.
func CastingCurseAggressiveHooks() script.Hooks {
	base := CastingCurseHooks()
	return base.With(script.Hooks{
		OnSeeCreature: func(s *script.Script, e script.SeeCreature) {
			n, c := e.NPC, e.Creature
			if !c.Playable() {
				base.SeeCreature(s, e)
				return
			}
			if n.LifeTime() > 7 && n.Intention() == script.IntentionWander && n.InMyTerritory() {
				maybeDebuff(s, n, c, npc.SkillTypeDebuff)
			}
			defaultnpc.TryToAttack(s, n, c)
			base.SeeCreature(s, e)
		},
	}).Named("WarriorCastingCurseAggressive")
}

// CorePrivateDicor is CorePrivateDicor: Core's Dicor, a
// WarriorCastingCurseAggressive that curses an attacker, and mutes a
// caster, more than 8 levels above it, and every 90 to 330 s may shuffle
// its hate. A hit adds no hate.
func CorePrivateDicor() script.Script {
	base := CastingCurseAggressiveHooks()
	return script.Script{Dir: castingCurseAggressiveDir, Behavior: true, NPCs: []int32{29009}, Hooks: base.With(script.Hooks{
		OnCreated: func(s *script.Script, e script.Created) {
			s.StartTimer("3001", e.NPC, nil, coreShuffleDelay(s))
			base.Created(s, e)
		},
		OnTimer: func(s *script.Script, e script.Timer) string {
			if strings.EqualFold(e.Name, "3001") {
				if s.Rnd(100) < 66 {
					e.NPC.RandomizeAttack()
				}
				s.StartTimer("3001", e.NPC, nil, coreShuffleDelay(s))
			}
			return base.Timer(s, e)
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
