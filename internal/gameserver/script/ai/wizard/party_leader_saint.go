package wizard

import (
	"strings"
	"time"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/npc"
	"github.com/fatal10110/acis_golang/internal/gameserver/script"
	"github.com/fatal10110/acis_golang/internal/gameserver/script/ai/defaultnpc"
)

// castUncheckedByType makes n want to cast at target, weight 1000000 and
// without the reuse and cost checks, the skill its template lists under t;
// a template that lists none casts nothing.
func castUncheckedByType(n *script.NPC, target script.Creature, t npc.SkillType) {
	if ref, ok := n.SkillByType(t); ok {
		n.AddCastDesireUnchecked(target, ref, 1000000)
	}
}

// PartyLeaderSaintAggressive is PartyLeaderWizardSaintAggressive: a
// PartyLeaderWizard that buffs the creatures around it 5 s after it spawns
// and every 120 s after, unless it fights (then on a draw under 33).
// Settled in its territory with an empty hate list, it curses or strikes
// the playable it sees. It strikes what it fights with its first long range
// DD magic, its party's attackers with its second or its range debuff, and
// steps back from its most hated once a cast ends with it closer than 100.
func PartyLeaderSaintAggressive() script.Script {
	base := partyLeaderHooks()
	return script.Script{Dir: partyLeaderDir, Behavior: true, NPCs: []int32{21544, 21541}, Hooks: base.With(script.Hooks{
		OnCreated: func(s *script.Script, e script.Created) {
			s.StartTimer("1005", e.NPC, nil, 5000*time.Millisecond)
			base.Created(s, e)
		},
		OnTimer: func(s *script.Script, e script.Timer) string {
			n := e.NPC
			if strings.EqualFold(e.Name, "1005") {
				if n.Intention() != script.IntentionAttack || s.Rnd(100) < 33 {
					castByType(n, n, npc.SkillTypeSelfRangeBuff, 1000000)
				}
				s.StartTimer("1005", n, nil, 120000*time.Millisecond)
			}
			return base.Timer(s, e)
		},
		OnSeeCreature: func(s *script.Script, e script.SeeCreature) {
			n, c := e.NPC, e.Creature
			if !c.Playable() {
				base.SeeCreature(s, e)
				return
			}
			hateList := n.HateList()
			if hateList.Empty() && n.LifeTime() > 7 && n.InMyTerritory() {
				// The range debuff's distance check measures the creature
				// from itself, so only the draw decides.
				if s.Rnd(100) < 33 {
					castOrAttack(n, c, npc.SkillTypeRangeDebuff, false)
				} else {
					castOrAttack(n, c, npc.SkillTypeWLongRangeDDMagic1, true)
				}
				hateList.AddDefault(c)
				base.SeeCreature(s, e)
			}
		},
		OnAttacked: func(s *script.Script, e script.Attacked) {
			n, attacker := e.NPC, e.Attacker
			hateList := n.HateList()
			if attacker.Playable() {
				f0 := damageHate(n, float64(e.Damage), defaultnpc.HateRatio(n, attacker))
				if hateList.Empty() {
					hateList.Add(attacker, (f0*100)+300)
				} else {
					hateList.Add(attacker, f0*100)
				}
			}
			if attacker.Playable() {
				switch {
				case n.Int(script.SlotIntAI0) != 0:
					f0 := damageHate(n, float64(e.Damage), defaultnpc.HateRatio(n, attacker))
					n.AddAttackDesire(attacker, f0*100)
				case hateList.MostHated() != nil:
					if s.Rnd(100) < 80 {
						castOrAttack(n, attacker, npc.SkillTypeWLongRangeDDMagic1, true)
					} else {
						player := script.ActingPlayer(attacker)
						inParty := player != nil && player.InParty()
						if inParty || !script.Same(n.TopDesireTarget(), attacker) {
							if s.Rnd(100) < 33 {
								castOrAttack(n, attacker, npc.SkillTypeWLongRangeDDMagic2, true)
							} else {
								castOrAttack(n, attacker, npc.SkillTypeRangeDebuff, false)
							}
						}
					}
				case s.Rnd(100) < 10:
					castOrAttack(n, attacker, npc.SkillTypeWLongRangeDDMagic1, true)
				}
			}
			base.Attacked(s, e)
		},
		OnPartyAttacked: func(s *script.Script, e script.PartyAttacked) {
			called, target := e.Called, e.Target
			if s.Rnd(100) < 80 {
				castUncheckedByType(called, target, npc.SkillTypeWLongRangeDDMagic2)
			} else {
				castByType(called, target, npc.SkillTypeRangeDebuff, 1000000)
			}
			base.PartyAttacked(s, e)
		},
		OnClanAttacked: func(s *script.Script, e script.ClanAttacked) {
			called, attacker := e.Called, e.Attacker
			hateList := called.HateList()
			hateList.Refresh()
			if attacker.Playable() && called.LifeTime() > 7 && hateList.Empty() {
				castOrAttack(called, attacker, npc.SkillTypeWLongRangeDDMagic1, true)
			}
			base.ClanAttacked(s, e)
		},
		OnMoveToFinished: func(_ *script.Script, e script.MoveToFinished) {
			n := e.NPC
			if mostHatedHI := n.HateList().MostHated(); mostHatedHI != nil {
				castOrAttack(n, mostHatedHI, npc.SkillTypeWLongRangeDDMagic1, true)
			}
		},
		OnUseSkillFinished: func(_ *script.Script, e script.UseSkillFinished) {
			n := e.NPC
			mostHatedHI := n.HateList().MostHated()
			if mostHatedHI == nil {
				return
			}
			if n.Distance2D(mostHatedHI) < 100 {
				n.AddFleeDesire(mostHatedHI, n.MaxDriftRange(), 1000000)
			} else {
				castOrAttack(n, mostHatedHI, npc.SkillTypeWLongRangeDDMagic1, true)
			}
		},
	})}
}
