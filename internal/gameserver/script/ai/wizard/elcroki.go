package wizard

import (
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/npc"
	"github.com/fatal10110/acis_golang/internal/gameserver/script"
	"github.com/fatal10110/acis_golang/internal/gameserver/script/ai/defaultnpc"
)

// Elcroki is a PartyLeaderWizard that calls up to two silhouettes at a time,
// five in all, when it sees a player in its territory and when one of them
// dies while it fights. It strikes from afar with its long range DD magic,
// up close with its self range one, buffs itself once below its HpChkRate3
// HP share and may curse the creatures around it below its HpChkRate5 one.
// Healed back and left alone, it walks home and tells its silhouettes to
// go.
func Elcroki() script.Script {
	return script.Script{Dir: partyLeaderDir, Behavior: true, NPCs: []int32{22214}, Hooks: elcrokiHooks()}
}

// castByType makes n want to cast at target, with weight, the skill its
// template lists under t; a template that lists none casts nothing.
func castByType(n *script.NPC, target script.Creature, t npc.SkillType, weight float64) {
	if ref, ok := n.SkillByType(t); ok {
		n.AddCastDesire(target, ref, weight)
	}
}

// castOrAttack makes n want to cast, weight 1000000, the skill its template
// lists under t at target, when it has the MP and HP the cast takes;
// unchecked, the desire skips the reuse and cost checks. Otherwise n gives
// up its spells (int slot 0 set to 1) and wants to attack target, weight
// 1000. A template that lists no such skill attacks.
func castOrAttack(n *script.NPC, target script.Creature, t npc.SkillType, unchecked bool) {
	ref, _ := n.SkillByType(t)
	if !n.MeetsHPMPConditions(target, ref) {
		n.SetInt(script.SlotIntAI0, 1)
		n.AddAttackDesire(target, 1000)
		return
	}
	if unchecked {
		n.AddCastDesireUnchecked(target, ref, 1000000)
	} else {
		n.AddCastDesire(target, ref, 1000000)
	}
}

// elcrokiSpells makes n answer c, from farther than its LongRangeSkillDist
// with its long range DD magic (or an attack when it cannot pay for it),
// else with its self range DD magic.
func elcrokiSpells(n *script.NPC, c script.Creature) {
	if n.Distance3D(c) > float64(n.IntParamOr("LongRangeSkillDist", 100)) {
		castOrAttack(n, c, npc.SkillTypeLongRangeDDMagic1, false)
	} else {
		castByType(n, c, npc.SkillTypeSelfRangeDDMagic1, 1000000)
	}
}

// elcrokiSilhouette places one silhouette of id within 100 on x and y of
// n, turning on target when there is one.
func elcrokiSilhouette(s *script.Script, n *script.NPC, id int32, heading int, target script.Creature) {
	at := script.LocOf(n)
	x := at.X + s.Rnd(100)
	y := at.Y + s.Rnd(100)
	params := script.Params{}
	if target != nil {
		params = script.Params{P1: 1000, P2: target.ObjectID(), P3: 0}
	}
	s.Spawner().CreateOnePrivateEx(n, id, script.Loc{X: x, Y: y, Z: at.Z, Heading: heading}, 0, params)
	n.SetInt(script.SlotIntAI3, n.Int(script.SlotIntAI3)+1)
	n.SetInt(script.SlotIntAI4, n.Int(script.SlotIntAI4)+1)
}

func elcrokiHooks() script.Hooks {
	base := partyLeaderHooks()
	return base.With(script.Hooks{
		OnCreated: func(s *script.Script, e script.Created) {
			n := e.NPC
			n.SetInt(npc.IntQuest0, 0)
			n.SetInt(script.SlotIntAI3, 0)
			n.SetInt(script.SlotIntAI4, 0)
			base.Created(s, e)
		},
		OnNoDesire: func(s *script.Script, e script.NoDesire) {
			n := e.NPC
			if !n.InCombat() {
				if n.HPRatio()*100 >= float64(n.IntParamOr("HpChkRate2", 80)) {
					if n.Int(script.SlotIntAI3) > 0 {
						s.BroadcastScriptEvent(n, 11039, 0, int(n.IntParamOr("BroadCastRange", 300)))
						n.SetInt(script.SlotIntAI3, 0)
						n.SetInt(script.SlotIntAI4, 0)
					}
					n.SetInt(npc.IntQuest0, 0)
				}
				n.AddMoveToDesire(n.SpawnLocation(), 30)
			}
			base.NoDesire(s, e)
		},
		OnAttacked: func(s *script.Script, e script.Attacked) {
			n, attacker := e.NPC, e.Attacker
			hpRatio := n.HPRatio()
			if n.Int(npc.IntQuest0) == 0 && hpRatio*100 <= float64(n.IntParamOr("HpChkRate3", 60)) {
				castByType(n, n, npc.SkillTypeSelfRangeBuff1, 1000000)
			}
			if hpRatio*100 <= float64(n.IntParamOr("HpChkRate5", 30)) && int32(s.Rnd(100)) < n.IntParamOr("ProbSelfRangeDeBuff1", 30) {
				castByType(n, n, npc.SkillTypeSelfRangeDebuff1, 1000000)
			}
			if int32(s.Rnd(100)) <= n.IntParamOr("ProbCond4", 30) {
				s.BroadcastScriptEvent(n, 10002, n.ObjectID(), int(n.IntParamOr("BroadCastRange", 300)))
			}
			base.Attacked(s, e)
			if attacker.Playable() {
				if n.Int(script.SlotIntAI0) == 0 {
					hated := !n.HateList().Empty()
					if n.Distance3D(attacker) > float64(n.IntParamOr("LongRangeSkillDist", 100)) {
						if hated || s.Rnd(100) < 2 {
							castOrAttack(n, attacker, npc.SkillTypeLongRangeDDMagic1, false)
						}
					} else if hated || s.Rnd(100) < 2 {
						castByType(n, attacker, npc.SkillTypeSelfRangeDDMagic1, 1000000)
					} else if s.Rnd(100) < 2 {
						castOrAttack(n, attacker, npc.SkillTypeSelfRangeDDMagic1, false)
					}
				} else {
					f0 := damageHate(n, float64(e.Damage), defaultnpc.HateRatio(n, attacker))
					n.AddAttackDesire(attacker, f0*100)
				}
			}
			base.Attacked(s, e)
		},
		OnClanAttacked: func(s *script.Script, e script.ClanAttacked) {
			called, attacker := e.Called, e.Attacker
			hateList := called.HateList()
			hateList.Refresh()
			if attacker.Playable() && called.LifeTime() > 7 && hateList.Empty() {
				elcrokiSpells(called, attacker)
			}
			base.ClanAttacked(s, e)
		},
		OnPartyAttacked: func(s *script.Script, e script.PartyAttacked) {
			called, target := e.Called, e.Target
			hateList := called.HateList()
			hateList.Refresh()
			if target.Playable() && hateList.Empty() {
				elcrokiSpells(called, target)
			}
			base.PartyAttacked(s, e)
		},
		OnUseSkillFinished: func(_ *script.Script, e script.UseSkillFinished) {
			n := e.NPC
			if buff, ok := n.SkillByType(npc.SkillTypeSelfRangeBuff1); e.Success && ok && e.Skill == buff {
				n.SetInt(npc.IntQuest0, 1)
			}
			if n.Int(script.SlotIntAI0) != 1 {
				if mostHatedHI := n.HateList().MostHated(); mostHatedHI != nil {
					if n.Distance3D(mostHatedHI) > float64(n.IntParamOr("LongRangeSkillDist", 100)) {
						castOrAttack(n, mostHatedHI, npc.SkillTypeLongRangeDDMagic1, false)
					}
					castByType(n, mostHatedHI, npc.SkillTypeSelfRangeDDMagic1, 1000000)
				}
			}
		},
		OnPartyDied: func(s *script.Script, e script.PartyDied) {
			caller, called := e.Caller, e.Called
			if script.Same(caller, called) {
				return
			}
			if called.Int(script.SlotIntAI3) > 0 {
				called.SetInt(script.SlotIntAI3, called.Int(script.SlotIntAI3)-1)
			}
			if called.InCombat() && called.Int(script.SlotIntAI3) < 2 && called.Int(script.SlotIntAI4) < called.IntParamOr("max_spawn_privates", 5) {
				topDesireTarget := called.TopDesireTarget()
				silhouette1 := called.IntParamOr("silhouette1", 0)
				if s.Rnd(2) == 0 {
					elcrokiSilhouette(s, called, silhouette1, 0, topDesireTarget)
				}
				elcrokiSilhouette(s, called, silhouette1, 0, topDesireTarget)
			}
		},
		OnSeeCreature: func(s *script.Script, e script.SeeCreature) {
			n, c := e.NPC, e.Creature
			if !isPlayer(c) || n.IntParam("ag_type") != 1 || !n.InMyTerritory() {
				return
			}
			hateList := n.HateList()
			if !hateList.Empty() {
				return
			}
			silhouette1 := n.IntParam("silhouette1")
			silhouette2 := n.IntParam("silhouette2")
			maxSpawnPrivates := n.IntParamOr("max_spawn_privates", 5)
			heading := script.LocOf(n).Heading
			room := func() bool {
				return n.Int(script.SlotIntAI3) < 2 && n.Int(script.SlotIntAI4) < maxSpawnPrivates
			}
			for range 2 {
				if s.Rnd(2) == 0 && room() {
					elcrokiSilhouette(s, n, silhouette1, heading, c)
				} else if room() {
					elcrokiSilhouette(s, n, silhouette2, heading, c)
				}
			}
			if n.Distance3D(c) > float64(n.IntParamOr("LongRangeSkillDist", 100)) {
				castOrAttack(n, c, npc.SkillTypeLongRangeDDMagic1, false)
			}
			castByType(n, c, npc.SkillTypeSelfRangeDDMagic1, 1000000)
			hateList.AddDefault(c)
			base.SeeCreature(s, e)
		},
	})
}
