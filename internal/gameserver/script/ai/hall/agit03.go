package hall

import (
	"strings"
	"time"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/npc"
	"github.com/fatal10110/acis_golang/internal/gameserver/script"
	"github.com/fatal10110/acis_golang/internal/gameserver/script/ai/defaultnpc"
)

// The Fortress of the Dead's von Hellmann family: Lidia and the two she
// raises, Alfred and Giselle.
const (
	alfredID  = 35630
	giselleID = 35631
)

// castSkill makes n want to cast the skill its template lists under t at
// target; a template listing none casts nothing.
func castSkill(n *script.NPC, target script.Creature, t npc.SkillType) {
	if ref, ok := n.SkillByType(t); ok {
		n.AddCastDesire(target, ref, 1000000)
	}
}

// vonHellmannAttacked is the attacked hook Alfred and Giselle share: each
// hit makes the NPC want to attack its playable attacker by the damage, and
// on a draw under 1 of 750 it casts its DD magic at an attacker more than
// 300 away.
func vonHellmannAttacked(s *script.Script, e script.Attacked) {
	n, attacker := e.NPC, e.Attacker
	if attacker.Playable() {
		n.AddAttackDesire(attacker, damageWeight(n, e.Damage, 1000))
	}
	if n.Distance2D(attacker) > 300 && s.Rnd(25*30) < 1 {
		castSkill(n, attacker, npc.SkillTypeDDMagic)
	}
}

// Alfred is Agit03Alfred: Alfred von Hellmann, who fights back at whoever
// hits him and casts his DD magic at a distant attacker.
func Alfred() script.Script {
	base := defaultnpc.New().Hooks
	return script.Script{Dir: dir, Behavior: true, NPCs: []int32{alfredID}, Hooks: base.With(script.Hooks{
		OnCreated: func(s *script.Script, e script.Created) {
			e.NPC.Shout(script.NpcString(1010636))
			e.NPC.SetInt(script.SlotIntAI0, 0)
			base.Created(s, e)
		},
		OnAttacked: vonHellmannAttacked,
		OnMyDying: func(_ *script.Script, e script.MyDying) {
			e.NPC.Shout(script.NpcString(1010625))
		},
	})}
}

// Giselle is Agit03Giselle: Giselle von Hellmann, who fights as Alfred does
// and casts her range buff on herself 1 s after she appears, then every
// 300 s.
func Giselle() script.Script {
	base := defaultnpc.New().Hooks
	return script.Script{Dir: dir, Behavior: true, NPCs: []int32{giselleID}, Hooks: base.With(script.Hooks{
		OnCreated: func(s *script.Script, e script.Created) {
			e.NPC.Shout(script.NpcString(1010637))
			s.StartTimer("1003", e.NPC, nil, 1000*time.Millisecond)
			base.Created(s, e)
		},
		OnAttacked: vonHellmannAttacked,
		OnTimer: func(s *script.Script, e script.Timer) string {
			if strings.EqualFold(e.Name, "1003") {
				castSkill(e.NPC, e.NPC, npc.SkillTypeSelfRangeBuffA)
				s.StartTimer("1003", e.NPC, e.Player, 300000*time.Millisecond)
			}
			return ""
		},
		OnMyDying: func(_ *script.Script, e script.MyDying) {
			e.NPC.Shout(script.NpcString(1010625))
		},
	})}
}

// Lidia is Agit03Lidia: Lidia von Hellmann, the Fortress of the Dead's
// mistress. She raises Giselle, Alfred and 15 vampires as she appears and
// buffs herself then and every 30 s until both Giselle and Alfred have
// fallen. She fights back at whoever hits her or her clan inside her
// territory, may cast her range and DD magic at an attacker, and jumps
// home dropping her attacks when hit out of her territory.
func Lidia() script.Script {
	base := defaultnpc.New().Hooks
	return script.Script{Dir: dir, Behavior: true, NPCs: []int32{35629}, Hooks: base.With(script.Hooks{
		OnCreated: func(s *script.Script, e script.Created) {
			n := e.NPC
			n.Shout(script.NpcString(1010624))
			for _, p := range lidiaPrivates() {
				s.Spawner().CreateOnePrivateEx(n, p.id, script.Loc{X: p.x, Y: p.y, Z: p.z, Heading: p.heading}, 0, script.Params{})
			}
			castSkill(n, n, npc.SkillTypeSelfBuff)
			s.StartTimer("1001", n, nil, 30000*time.Millisecond)
			n.SetInt(script.SlotIntAI1, 0)
			base.Created(s, e)
		},
		OnAttacked: func(s *script.Script, e script.Attacked) {
			n, attacker := e.NPC, e.Attacker
			if n.InMyTerritory() && attacker.Playable() {
				n.AddAttackDesire(attacker, damageWeight(n, e.Damage, 1000))
			}
			if s.Rnd(15*30) < 1 || (n.HPRatio() < 0.2 && s.Rnd(15*30) < 1) {
				castSkill(n, attacker, npc.SkillTypeSelfRangeDDMagic)
			}
			if n.Distance2D(attacker) > 300 && s.Rnd(25*30) < 1 {
				castSkill(n, attacker, npc.SkillTypeDDMagic)
			}
			if !n.InMyTerritory() {
				n.RemoveAllAttackDesire()
				n.TeleportTo(n.SpawnLocation())
			}
		},
		OnClanAttacked: func(_ *script.Script, e script.ClanAttacked) {
			if e.Attacker.Playable() {
				e.Called.AddAttackDesire(e.Attacker, damageWeight(e.Called, e.Damage, 1000))
			}
		},
		OnPartyDied: func(_ *script.Script, e script.PartyDied) {
			if e.Caller.NpcID() == giselleID {
				e.Called.SetInt(script.SlotIntAI1, e.Called.Int(script.SlotIntAI1)+1)
			}
			if e.Caller.NpcID() == alfredID {
				e.Called.SetInt(script.SlotIntAI1, e.Called.Int(script.SlotIntAI1)+1)
			}
		},
		OnTimer: func(s *script.Script, e script.Timer) string {
			if strings.EqualFold(e.Name, "1001") {
				castSkill(e.NPC, e.NPC, npc.SkillTypeSelfBuff)
				if e.NPC.Int(script.SlotIntAI1) < 2 {
					s.StartTimer("1001", e.NPC, e.Player, 30000*time.Millisecond)
				}
			}
			return ""
		},
		OnMyDying: func(_ *script.Script, e script.MyDying) {
			e.NPC.Shout(script.NpcString(1010638))
		},
	})}
}

// lidiaPrivates are the privates Lidia raises: Giselle, Alfred and 15
// vampires.
func lidiaPrivates() []private {
	return []private{
		{giselleID, 56619, -27866, 569, 54000},
		{alfredID, 59282, -26496, 569, 48000},
		{35647, 57905, -27648, 608, 33540},
		{35647, 57905, -27712, 608, 33540},
		{35647, 58233, -27182, 608, 33540},
		{35647, 58233, -27232, 608, 33540},
		{35647, 58233, -27282, 608, 33540},
		{35647, 58233, -27332, 608, 33540},
		{35647, 58233, -27382, 608, 33540},
		{35647, 58233, -27432, 608, 33540},
		{35647, 58233, -27482, 608, 33540},
		{35647, 58233, -27532, 608, 33540},
		{35647, 58233, -27582, 608, 33540},
		{35647, 58233, -27632, 608, 33540},
		{35647, 58233, -27682, 608, 33540},
		{35647, 58233, -27732, 608, 33540},
		{35647, 58233, -27782, 608, 33540},
	}
}
