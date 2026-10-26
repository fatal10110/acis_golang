package wizard

import (
	"strings"
	"time"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/npc"
	"github.com/fatal10110/acis_golang/internal/gameserver/script"
	"github.com/fatal10110/acis_golang/internal/gameserver/script/ai/defaultnpc"
)

// CorpseNecroAggressive is WizardCorpseNecroAggressive: a WizardCorpseNecro
// that, settled in its territory with an empty hate list, strikes a
// playable it sees with its long range DD magic beyond 100, else its short
// range one, unchecked, and hates it; short of MP it gives up its spells
// and attacks instead. Unsettled, it ignores the playable.
func CorpseNecroAggressive() script.Script {
	base := corpseNecroHooks()
	return script.Script{Dir: corpseNecroDir, Behavior: true, NPCs: []int32{21581, 21560, 21566}, Hooks: base.With(script.Hooks{
		OnSeeCreature: func(s *script.Script, e script.SeeCreature) {
			n, c := e.NPC, e.Creature
			if !c.Playable() {
				base.SeeCreature(s, e)
				return
			}
			hateList := n.HateList()
			if n.LifeTime() > 7 && hateList.Empty() && n.InMyTerritory() {
				ddMagic2Spell(n, c)
				hateList.AddDefault(c)
				base.SeeCreature(s, e)
			}
		},
	})}
}

// CorpseNecroSummon is WizardCorpseNecroSummon: a WizardCorpseNecro that
// looks around every 5 s and raises the dead. Hating someone, it may clear
// a dead NPC it sees within 200, on a draw under 30, casting its corpse
// clearing at once past every check, and place its SummonPrivate on the
// corpse, told (1000, the corpse); a template without the corpse clearing
// raises nothing. It ignores any other non-playable it sees, and wants to
// attack a playable that hits it by the damage.
func CorpseNecroSummon() script.Script {
	return script.Script{Dir: corpseNecroDir, Behavior: true, NPCs: []int32{21563, 21583, 21557, 21572}, Hooks: corpseNecroSummonHooks()}
}

// corpseNecroSummonHooks are CorpseNecroSummon's hooks, which its child
// derives its from.
func corpseNecroSummonHooks() script.Hooks {
	base := corpseNecroHooks()
	return base.With(script.Hooks{
		OnCreated: func(s *script.Script, e script.Created) {
			s.StartTimerAtFixedRate("3456", e.NPC, nil, 5000*time.Millisecond, 5000*time.Millisecond)
			base.Created(s, e)
		},
		OnSeeCreature: func(s *script.Script, e script.SeeCreature) {
			n, c := e.NPC, e.Creature
			if _, isNPC := c.(*script.NPC); isNPC && c.Dead() && s.Rnd(100) < 30 && n.Distance2D(c) < 200 {
				if mostHatedHI := n.HateList().MostHated(); mostHatedHI != nil {
					// A template without the corpse clearing fails the
					// cast's start, and the hook stops before the private.
					if clearCorpse, ok := n.SkillByType(npc.SkillTypeClearCorpse); ok {
						n.CastUnchecked(c, clearCorpse)
						at := script.LocOf(c)
						s.Spawner().CreateOnePrivateEx(n, n.IntParam("SummonPrivate"), script.Loc{X: at.X, Y: at.Y, Z: at.Z}, 0, script.Params{P1: 1000, P2: c.ObjectID()})
					}
				}
				return
			}
			if !c.Playable() {
				return
			}
			base.SeeCreature(s, e)
		},
		OnAttacked: func(s *script.Script, e script.Attacked) {
			n, attacker := e.NPC, e.Attacker
			if attacker.Playable() {
				f0 := damageHate(n, float64(e.Damage), defaultnpc.HateRatio(n, attacker))
				n.AddAttackDesire(attacker, f0*100)
			}
			base.Attacked(s, e)
		},
		OnTimer: func(s *script.Script, e script.Timer) string {
			if strings.EqualFold(e.Name, "3456") {
				e.NPC.LookNeighbor()
			}
			return base.Timer(s, e)
		},
	}).Named("WizardCorpseNecroSummon")
}

// CorpseNecroSummonAggressive is WizardCorpseNecroSummonAggressive: a
// WizardCorpseNecroSummon that, settled in its territory with an empty
// hate list, strikes a playable it sees with its long range DD magic
// beyond 100, else its short range one, unchecked, and hates it; short of
// MP it gives up its spells and attacks instead. Unsettled, it ignores the
// playable.
func CorpseNecroSummonAggressive() script.Script {
	base := corpseNecroSummonHooks()
	return script.Script{Dir: corpseNecroDir, Behavior: true, NPCs: []int32{21559, 21564}, Hooks: base.With(script.Hooks{
		OnSeeCreature: func(s *script.Script, e script.SeeCreature) {
			n, c := e.NPC, e.Creature
			if !c.Playable() {
				base.SeeCreature(s, e)
				return
			}
			hateList := n.HateList()
			if n.LifeTime() > 7 && n.InMyTerritory() && hateList.Empty() {
				ddMagic2Spell(n, c)
				hateList.AddDefault(c)
				base.SeeCreature(s, e)
			}
		},
	})}
}
