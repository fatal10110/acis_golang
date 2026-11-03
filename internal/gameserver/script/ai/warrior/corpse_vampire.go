package warrior

import (
	"strings"
	"time"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/npc"
	"github.com/fatal10110/acis_golang/internal/gameserver/script"
	"github.com/fatal10110/acis_golang/internal/gameserver/script/ai/defaultnpc"
)

// CorpseVampire is WarriorCorpseVampire: a WarriorPhysicalSpecial that buffs
// itself when created and every 10 s out of a fight, and, hit by the
// player or summon it hates most, may debuff it unless the debuff already
// holds, and below half its hit points may blast it with its first DD
// magic.
func CorpseVampire() script.Script {
	return script.Script{Dir: physicalSpecialDir, Behavior: true, NPCs: []int32{21424, 21593}, Hooks: CorpseVampireHooks()}
}

// CorpseVampireHooks are WarriorCorpseVampire's hooks, which its children
// derive theirs from.
func CorpseVampireHooks() script.Hooks {
	base := PhysicalSpecialHooks()
	return base.With(script.Hooks{
		OnCreated: func(s *script.Script, e script.Created) {
			n := e.NPC
			n.SetInt(script.SlotIntAI0, 0)
			addCastByType(n, n, npc.SkillTypeSelfBuff, 1000000)
			s.StartTimerAtFixedRate("2001", n, nil, 10000*time.Millisecond, 10000*time.Millisecond)
			base.Created(s, e)
		},
		OnTimer: func(s *script.Script, e script.Timer) string {
			if strings.EqualFold(e.Name, "2001") {
				if n := e.NPC; n.Intention() != script.IntentionAttack {
					addCastByType(n, n, npc.SkillTypeSelfBuff, 1000000)
				}
			}
			return base.Timer(s, e)
		},
		OnAttacked: func(s *script.Script, e script.Attacked) {
			n, attacker := e.NPC, e.Attacker
			if attacker.Playable() {
				if script.Same(n.MostHated(), attacker) {
					debuff, ok := n.SkillByType(npc.SkillTypeDebuff)
					if s.Rnd(100) < 33 && ok && s.AbnormalLevel(attacker, debuff) <= 0 {
						n.AddCastDesire(attacker, debuff, 1000000)
					}
					if s.Rnd(100) < 33 && n.HPRatio() < 0.5 && n.Int(script.SlotIntAI0) == 0 {
						addCastByType(n, attacker, npc.SkillTypeDDMagic1, 1000000)
					}
				}
			}
			base.Attacked(s, e)
		},
	}).Named("WarriorCorpseVampire")
}

// CorpseVampireAggressive is WarriorCorpseVampireAggressive: a
// WarriorCorpseVampire that attacks the players and summons it sees and,
// settled in its territory and not attacking, may strike one farther than
// 100 with its physical special skill.
func CorpseVampireAggressive() script.Script {
	base := CorpseVampireHooks()
	return script.Script{Dir: physicalSpecialDir, Behavior: true, NPCs: []int32{21425, 21573, 21568}, Hooks: base.With(script.Hooks{
		OnSeeCreature: func(s *script.Script, e script.SeeCreature) {
			n, c := e.NPC, e.Creature
			// A creature other than a player or summon reaches no parent.
			if !c.Playable() {
				return
			}
			if n.LifeTime() > 7 && n.Intention() != script.IntentionAttack && n.InMyTerritory() {
				if n.Distance2D(c) > 100 && s.Rnd(100) < 33 {
					castPhysicalSpecial(n, c)
				}
			}
			defaultnpc.TryToAttack(s, n, c)
			base.SeeCreature(s, e)
		},
	})}
}

// CorpseVampireMobHate is WarriorCorpseVampireMobHate: a
// WarriorCorpseVampire that, settled and attacking, may blast with its
// second DD magic a player or summon hitting its clan closer than 100 to
// it.
func CorpseVampireMobHate() script.Script {
	base := CorpseVampireHooks()
	return script.Script{Dir: physicalSpecialDir, Behavior: true, NPCs: []int32{21582, 21594, 21587}, Hooks: base.With(script.Hooks{
		OnClanAttacked: func(s *script.Script, e script.ClanAttacked) {
			called, attacker := e.Called, e.Attacker
			if called.LifeTime() > 7 && attacker.Playable() && called.Intention() == script.IntentionAttack {
				if called.Distance2D(attacker) < 100 && s.Rnd(100) < 33 {
					addCastByType(called, attacker, npc.SkillTypeDDMagic2, 1000000)
				}
			}
			base.ClanAttacked(s, e)
		},
	})}
}
