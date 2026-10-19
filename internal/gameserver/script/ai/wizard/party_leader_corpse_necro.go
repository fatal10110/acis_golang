package wizard

import (
	"strings"
	"time"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/npc"
	"github.com/fatal10110/acis_golang/internal/gameserver/script"
	"github.com/fatal10110/acis_golang/internal/gameserver/script/ai/defaultnpc"
)

// PartyLeaderCorpseNecroAggressive is
// PartyLeaderWizardCorpseNecroAggressive: an aggressive PartyLeaderWizardDD2
// that looks around every 5 s and raises the dead. Fighting, it may place
// its SummonPrivate on a dead playable it sees within 100, told to attack
// the creature it hates most, and clear the corpse. Below 40% of its HP it
// may strike its top desire target with its DD magic when that target hits
// it.
func PartyLeaderCorpseNecroAggressive() script.Script {
	base := partyLeaderDD2Hooks()
	return script.Script{Dir: partyLeaderDir, Behavior: true, NPCs: []int32{21596, 21599}, Hooks: base.With(script.Hooks{
		OnCreated: func(s *script.Script, e script.Created) {
			s.StartTimerAtFixedRate("3456", e.NPC, nil, 5000*time.Millisecond, 5000*time.Millisecond)
			base.Created(s, e)
		},
		OnSeeCreature: func(s *script.Script, e script.SeeCreature) {
			n, c := e.NPC, e.Creature
			if !c.Playable() {
				base.SeeCreature(s, e)
				return
			}
			defaultnpc.TryToAttack(s, n, c)
			if c.Dead() && n.Intention() == script.IntentionAttack && s.Rnd(100) < 50 && n.Distance2D(c) < 100 {
				if mostHated := n.MostHated(); mostHated != nil {
					at := script.LocOf(c)
					s.Spawner().CreateOnePrivateEx(n, n.IntParam("SummonPrivate"), script.Loc{X: at.X, Y: at.Y, Z: at.Z}, 0, script.Params{P1: 1000, P2: mostHated.ObjectID()})
					castByType(n, c, npc.SkillTypeClearCorpse, 1000000)
				}
			}
			base.SeeCreature(s, e)
		},
		OnTimer: func(s *script.Script, e script.Timer) string {
			if strings.EqualFold(e.Name, "3456") {
				e.NPC.LookNeighbor()
			}
			return base.Timer(s, e)
		},
		OnAttacked: func(s *script.Script, e script.Attacked) {
			base.Attacked(s, e)
			n, attacker := e.NPC, e.Attacker
			if attacker.Playable() {
				if mostHatedHI := n.HateList().MostHated(); mostHatedHI != nil && n.Int(script.SlotIntAI0) == 0 {
					if script.Same(n.TopDesireTarget(), attacker) && s.Rnd(100) < 33 && n.HPRatio() < 0.4 {
						castByType(n, attacker, npc.SkillTypeDDMagic1, 1000000)
					}
				}
			}
			base.Attacked(s, e)
		},
	})}
}
