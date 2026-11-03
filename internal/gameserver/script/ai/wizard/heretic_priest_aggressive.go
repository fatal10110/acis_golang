package wizard

import (
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/npc"
	"github.com/fatal10110/acis_golang/internal/gameserver/script"
	"github.com/fatal10110/acis_golang/internal/gameserver/script/ai/defaultnpc"
)

// HereticPriestAggressive is WizardHereticPriestAggressive: a Wizard of the
// Pagan Temple that keeps walking home when idle and calls for help (event
// 10033) within its HelpCastRange against what it sees or what hits it
// unless it fights already. Until it gives up its spells it answers a
// playable's hit, and the end of each cast, with its debuff on a draw
// under 33 and its long or short range DD magic by distance (held in
// place when Hold is set); a settled one with an empty hate list answers
// a player's hit on its clan member the same way. Settled in its territory
// with an empty hate list, it strikes the playable it sees and hates it.
// It may bind a paladin, a phoenix knight or a cleric it sees, and it may
// send a player it sees in a party larger than its party_members
// parameter, with the player's summon, to one of its three teleport
// points. Out of its territory it drops its hate, heads home and calls
// those around (event 10035), unless it holds its ground.
func HereticPriestAggressive() script.Script {
	base := WizardHooks()
	return script.Script{Dir: wizardDir, Behavior: true, NPCs: []int32{22143, 22151}, Hooks: base.With(script.Hooks{
		OnNoDesire: func(_ *script.Script, e script.NoDesire) {
			e.NPC.AddMoveToDesire(e.NPC.SpawnLocation(), 30)
		},
		OnCreated: func(_ *script.Script, e script.Created) {
			e.NPC.AddMoveToDesire(e.NPC.SpawnLocation(), 30)
		},
		OnAttacked: func(s *script.Script, e script.Attacked) {
			base.Attacked(s, e)
			n, attacker := e.NPC, e.Attacker
			if !fighting(n.Intention()) {
				s.BroadcastScriptEventEx(n, 10033, attacker.ObjectID(), n.ObjectID(), int(n.IntParamOr("HelpCastRange", 500)))
			}
			if !attacker.Playable() {
				return
			}
			if n.Int(script.SlotIntAI0) == 0 {
				i0 := !n.HateList().Empty()
				priestEngage(s, n, attacker, true, i0)
				return
			}
			f0 := damageHate(n, float64(e.Damage), defaultnpc.HateRatio(n, attacker))
			if n.IntParam("Hold") == 1 {
				n.AddAttackDesireHold(attacker, f0*100)
			} else {
				n.AddAttackDesire(attacker, f0*100)
			}
		},
		OnClanAttacked: func(s *script.Script, e script.ClanAttacked) {
			called, attacker := e.Called, e.Attacker
			called.HateList().Refresh()
			if called.LifeTime() > 7 && isPlayer(attacker) && called.HateList().Size() == 0 {
				priestEngage(s, called, attacker, false, false)
			}
			base.ClanAttacked(s, e)
		},
		OnUseSkillFinished: func(s *script.Script, e script.UseSkillFinished) {
			n, c := e.NPC, e.Creature
			if n.Int(script.SlotIntAI0) == 0 {
				i0 := !n.HateList().Empty()
				priestEngage(s, n, c, true, i0)
			}
		},
		OnSeeCreature: func(s *script.Script, e script.SeeCreature) {
			n, c := e.NPC, e.Creature
			if !c.Playable() || n.Dead() {
				return
			}
			if !fighting(n.Intention()) {
				s.BroadcastScriptEventEx(n, 10033, c.ObjectID(), n.ObjectID(), int(n.IntParamOr("HelpCastRange", 500)))
			}
			if n.LifeTime() > 7 && n.InMyTerritory() && n.HateList().Empty() {
				switch {
				case n.IntParam("Hold") == 1:
					if n.Distance2D(c) > 40 && s.Rnd(100) < 80 {
						priestSpellOrAttack(n, npc.SkillTypeWLongRangeDDMagic, c, c, true)
					} else {
						priestSpellOrAttack(n, npc.SkillTypeWShortRangeDDMagic, c, c, true)
					}
				case n.Distance2D(c) > 100 && s.Rnd(100) < 80:
					priestSpellOrAttack(n, npc.SkillTypeWLongRangeDDMagic, c, c, false)
				case !priestSpell(n, npc.SkillTypeWShortRangeDDMagic, c, c, false):
					// This answer alone attacks without asking whether it
					// can.
					n.AddAttackDesire(c, 1000)
				}
				n.HateList().AddDefault(c)
			}
			acting := script.ActingPlayer(c)
			if acting == nil {
				// A playable with no player behind it has no class to
				// read: the hook stops here.
				return
			}
			playerClassID := acting.ClassID()
			isInClericGroup := acting.InClassGroup("@cleric_group")
			i0 := s.Rnd(100)
			if i0 < 33 && (playerClassID == 5 || playerClassID == 90 || isInClericGroup) {
				if holdMagic, ok := n.SkillByType(npc.SkillTypeHoldMagic); ok {
					n.AddCastDesireHold(c, holdMagic, 1000000)
				}
				priestSpellOrAttack(n, npc.SkillTypeWLongRangeDDMagic, c, c, true)
			}
			i0 = s.Rnd(100)
			if p, ok := c.(*script.Player); ok && i0 < 33 {
				if members := p.PartyMembers(); members != nil && len(members) > int(n.IntParam("party_members")) && n.IntParam("NoPCTeleport") == 0 && n.InSpawnTerritory(c) {
					priestSendAway(s, n, p)
				}
			}
			base.SeeCreature(s, e)
		},
		OnOutOfTerritory: func(s *script.Script, e script.OutOfTerritory) {
			n := e.NPC
			if n.IntParam("Hold") == 0 {
				n.RemoveAllAttackDesire()
				n.AddMoveToDesire(n.SpawnLocation(), 100)
				n.HateList().CleanAll()
				s.BroadcastScriptEvent(n, 10035, 0, int(n.IntParamOr("DistNoDesire", 500)))
			}
		},
	})}
}

// priestEngage is the heretic priest's answer to c: when it holds its
// ground (Hold 1), a draw under 33 wants its debuff on c held in place
// unless c already holds an effect of its kind, then its long range DD
// magic held on c from farther than 40 on a draw under 80, else its short
// range one, each only when priestEager allows. Free to move, the same
// debuff draw. Then, either way, its long range DD magic on c from farther
// than 100 on a draw under 80, else its short range one, closing in, each
// only when priestEager allows. The cost of each DD magic is checked on n
// itself.
func priestEngage(s *script.Script, n *script.NPC, c script.Creature, gated, hated bool) {
	debuff, hasDebuff := n.SkillByType(npc.SkillTypeDebuff)
	if n.IntParam("Hold") == 1 {
		if s.Rnd(100) < 33 && s.AbnormalLevel(c, debuff) <= 0 && hasDebuff {
			n.AddCastDesireHold(c, debuff, 1000000)
		}
		if n.Distance2D(c) > 40 && s.Rnd(100) < 80 {
			if priestEager(s, gated, hated) {
				priestSpellOrAttack(n, npc.SkillTypeWLongRangeDDMagic, n, c, true)
			}
		} else if priestEager(s, gated, hated) {
			priestSpellOrAttack(n, npc.SkillTypeWShortRangeDDMagic, n, c, true)
		}
	} else if s.Rnd(100) < 33 && s.AbnormalLevel(c, debuff) <= 0 && hasDebuff {
		n.AddCastDesireHold(c, debuff, 1000000)
	}
	if n.Distance2D(c) > 100 && s.Rnd(100) < 80 {
		if priestEager(s, gated, hated) {
			priestSpellOrAttack(n, npc.SkillTypeWLongRangeDDMagic, n, c, false)
		}
	} else if priestEager(s, gated, hated) {
		priestSpellOrAttack(n, npc.SkillTypeWShortRangeDDMagic, n, c, false)
	}
}

// priestEager reports whether a gated answer goes ahead: always when the
// priest hates someone (hated), else on a draw under 2. An ungated answer
// always goes ahead, with no draw.
func priestEager(s *script.Script, gated, hated bool) bool {
	return !gated || hated || s.Rnd(100) < 2
}

// priestSpell makes n want to cast at c, weight 1000000, the skill its
// template lists under t, held in place when hold is set, when it has the
// MP and HP the cast at checkOn takes, and reports true. Otherwise n gives
// up its spells (int slot 0 set to 1) and reports false. A template that
// lists no such skill gives up.
func priestSpell(n *script.NPC, t npc.SkillType, checkOn, c script.Creature, hold bool) bool {
	ref, _ := n.SkillByType(t)
	if !n.MeetsHPMPConditions(checkOn, ref) {
		n.SetInt(script.SlotIntAI0, 1)
		return false
	}
	if hold {
		n.AddCastDesireHold(c, ref, 1000000)
	} else {
		n.AddCastDesire(c, ref, 1000000)
	}
	return true
}

// priestSpellOrAttack is priestSpell, then, when n gave up its spells
// and can attack c, a desire to attack it, weight 1000, held in place when
// hold is set.
func priestSpellOrAttack(n *script.NPC, t npc.SkillType, checkOn, c script.Creature, hold bool) {
	if priestSpell(n, t, checkOn, c, hold) || !n.CanAutoAttack(c) {
		return
	}
	if hold {
		n.AddAttackDesireHold(c, 1000)
	} else {
		n.AddAttackDesire(c, 1000)
	}
}

// priestSendAway teleports p, and its summon after it, to one of n's
// three teleport points, picked at random.
func priestSendAway(s *script.Script, n *script.NPC, p *script.Player) {
	c0 := p.ActiveSummon()
	i1 := s.Rnd(3)
	telPosX1, telPosX2, telPosX3 := n.IntParam("TelPosX1"), n.IntParam("TelPosX2"), n.IntParam("TelPosX3")
	telPosY1, telPosY2, telPosY3 := n.IntParam("TelPosY1"), n.IntParam("TelPosY2"), n.IntParam("TelPosY3")
	telPosZ1, telPosZ2, telPosZ3 := n.IntParam("TelPosZ1"), n.IntParam("TelPosZ2"), n.IntParam("TelPosZ3")
	var to script.Loc
	switch i1 {
	case 0:
		to = script.Loc{X: int(telPosX1), Y: int(telPosY1), Z: int(telPosZ1)}
	case 1:
		to = script.Loc{X: int(telPosX2), Y: int(telPosY2), Z: int(telPosZ2)}
	default:
		to = script.Loc{X: int(telPosX3), Y: int(telPosY3), Z: int(telPosZ3)}
	}
	p.TeleportTo(to, 0)
	if c0 != nil {
		script.TeleportTo(c0, to)
	}
}
