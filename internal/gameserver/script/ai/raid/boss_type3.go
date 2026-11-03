package raid

import (
	"strings"
	"time"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/npc"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/script"
)

// bossType3Dir is the directory RaidBossType3 and its child share.
const bossType3Dir = "ai/individual/Monster/RaidBoss/RaidBossAlone/RaidBossParty/RaidBossType3"

// BossType3 is RaidBossType3: a RaidBossParty that, one time in 15, wants
// its physical special at 1000000 on its top desire target when that
// target, a playable, hits its party or casts in its sight.
func BossType3() script.Script {
	return script.Script{Dir: bossType3Dir, Behavior: true, NPCs: bossType3IDs(), Hooks: bossType3Hooks()}
}

// bossType3IDs are RaidBossType3's own NPCs.
func bossType3IDs() []int32 {
	return []int32{
		25269, 25035, 25342, 25299, 25146, 25004, 25010, 25026, 25041, 25057, 25067, 25085, 25106, 25115,
		25131, 25149, 25170, 25185, 25211, 25217, 25241, 25256, 25260, 25362, 25388, 25412, 25444, 25475,
		25490, 25501, 29096,
	}
}

// bossType3Hooks are RaidBossType3's hooks, which its child derives its
// own from.
func bossType3Hooks() script.Hooks {
	base := bossPartyHooks()
	return base.With(script.Hooks{
		OnPartyAttacked: func(s *script.Script, e script.PartyAttacked) {
			if e.Target.Playable() {
				if script.Same(e.Target, e.Called.TopDesireTarget()) && s.Rnd(15) < 1 {
					castByType(e.Called, e.Target, npc.SkillTypePhysicalSpecialB)
				}
			}
			base.PartyAttacked(s, e)
		},
		OnSeeSpell: func(s *script.Script, e script.SeeSpell) {
			if top := e.NPC.TopDesireTarget(); top != nil && script.Same(e.Caster, top) && s.Rnd(15) < 1 {
				castByType(e.NPC, e.Caster, npc.SkillTypePhysicalSpecialB)
			}
			base.SeeSpell(s, e)
		},
	}).Named("RaidBossType3")
}

// BossType3AnaisAggressive is Anais, a RaidBossType3 that wants to attack,
// at 200, a playable it sees while it stands in its territory. Finding
// itself out of its territory with nothing to do, hit, on a hit on its
// party out of both their territories, or as a cast of its own ends, it
// goes back to its home and, 1 s later, drops its attack desires and its
// hate. A hit on its party from out of the caller's territory gets no
// further answer.
func BossType3AnaisAggressive() script.Script {
	return script.Script{Dir: bossType3Dir, Behavior: true, NPCs: []int32{29096}, Hooks: bossType3AnaisAggressiveHooks()}
}

// anaisGoHome jumps n back to Anais's home, (112800, -76160, 10), and arms
// the 1 s timer that drops its attack desires and hate.
func anaisGoHome(s *script.Script, n *script.NPC) {
	n.TeleportTo(script.Loc{X: 112800, Y: -76160, Z: 10})
	s.StartTimer("3001", n, nil, 1000*time.Millisecond)
}

// bossType3AnaisAggressiveHooks are Anais's hooks. Its see-creature and
// skill-finished hooks call no parent, and its party-attacked hook calls
// it only for a caller in its territory.
func bossType3AnaisAggressiveHooks() script.Hooks {
	base := bossType3Hooks()
	return base.With(script.Hooks{
		OnSeeCreature: func(_ *script.Script, e script.SeeCreature) {
			if !e.Creature.Playable() {
				return
			}
			if e.NPC.InMyTerritory() {
				e.NPC.AddAttackDesire(e.Creature, 200)
			}
		},
		OnNoDesire: func(s *script.Script, e script.NoDesire) {
			if !e.NPC.InMyTerritory() {
				anaisGoHome(s, e.NPC)
			}
			base.NoDesire(s, e)
		},
		OnAttacked: func(s *script.Script, e script.Attacked) {
			if !e.NPC.InMyTerritory() {
				anaisGoHome(s, e.NPC)
			}
			base.Attacked(s, script.Attacked{NPC: e.NPC, Attacker: e.Attacker, Damage: e.Damage, Skill: skill.Ref{}})
		},
		OnTimer: func(s *script.Script, e script.Timer) string {
			if strings.EqualFold(e.Name, "3001") {
				e.NPC.RemoveAllAttackDesire()
				e.NPC.HateList().CleanAll()
			}
			return base.Timer(s, e)
		},
		OnPartyAttacked: func(s *script.Script, e script.PartyAttacked) {
			if !e.Caller.InMyTerritory() && !e.Called.InMyTerritory() {
				anaisGoHome(s, e.Called)
			}
			if !e.Caller.InMyTerritory() {
				return
			}
			base.PartyAttacked(s, e)
		},
		OnUseSkillFinished: func(s *script.Script, e script.UseSkillFinished) {
			if !e.NPC.InMyTerritory() {
				anaisGoHome(s, e.NPC)
			}
		},
	}).Named("RaidBossType3AnaisAggressive")
}
