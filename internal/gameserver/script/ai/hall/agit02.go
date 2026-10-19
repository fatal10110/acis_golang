package hall

import (
	"strings"
	"time"

	"github.com/fatal10110/acis_golang/internal/gameserver/script"
	"github.com/fatal10110/acis_golang/internal/gameserver/script/ai/defaultnpc"
)

// The Devastated Castle defenders' skills: the escape Gustav and his
// lieutenants cast on themselves when their master is all but dead,
// Gustav's and Mikhail's attack skills, and the debuff Dietrich casts where
// his healer stands.
const (
	escape     = 4235
	gustavHit  = 4236
	mikhailHit = 4237
	reuseCurse = 4238
)

// The classes whose spells Dietrich follows to their caster: the healers
// and the orc buffers.
const (
	classBishop        = 16
	classProphet       = 17
	classElvenElder    = 30
	classShillienElder = 43
	classOverlord      = 51
	classWarcryer      = 52
)

// partyHate makes called want to attack target, a playable that hit its
// party member caller, by the damage's share of called's hit points times
// the damage and caller's weight point.
func partyHate(called, caller *script.NPC, target script.Creature, damage int32) {
	if target.Playable() {
		called.AddAttackDesire(target, float64(damage)/float64(called.MaxHP())/0.05*float64(damage)*float64(caller.Int(script.SlotWeightPoint))/1000000)
	}
}

// escapeOnce makes called, once, cast its escape on itself and say line,
// when its master is below 5% of its hit points.
func escapeOnce(called *script.NPC, line string) {
	if master := called.Master(); master != nil && master.HPRatio() < 0.05 && called.Int(script.SlotIntAI0) == 0 {
		called.SetInt(script.SlotIntAI0, 1)
		called.AddCastDesire(called, level1(escape), 1000000000)
		called.Say(line)
	}
}

// escaped sends n, done casting its escape, to the castle's inner hall,
// dropping its attacks; it reports whether the cast was the escape.
func escaped(n *script.NPC, e script.UseSkillFinished) bool {
	if e.Skill.ID != escape {
		return false
	}
	n.TeleportTo(script.Loc{X: 177134, Y: -18807, Z: -2263})
	n.RemoveAllAttackDesire()
	return true
}

// spellHate makes the NPC, attacking the caster it wants most, want to
// attack it further by the aggro of its skill.
func spellHate(s *script.Script, e script.SeeSpell) {
	n := e.NPC
	if aggro := s.Skill(e.Skill).AggroPoints(); aggro > 0 && n.Intention() == script.IntentionAttack && script.Same(n.TopDesireTarget(), e.Caster) {
		n.AddAttackDesire(e.Caster, damageWeight(n, int32(aggro), 150))
	}
}

// shuffle may shuffle the NPC's hate, on a draw under 1 of 5.
func shuffle(s *script.Script, n *script.NPC) {
	if s.Rnd(5) < 1 {
		n.RandomizeAttack()
	}
}

// Dietrich is Agit02Dietrich: Gustav's lieutenant. Every 60 s, out of his
// territory, he may jump to his master, and he may shuffle his hate. He
// follows a healer or buffer's spell to its caster, walks to that caster
// when his party is hit and casts his debuff there.
func Dietrich() script.Script {
	base := defaultnpc.New().Hooks
	return script.Script{Dir: dir, Behavior: true, NPCs: []int32{35408}, Hooks: base.With(script.Hooks{
		OnCreated: func(s *script.Script, e script.Created) {
			n := e.NPC
			n.Shout(script.NpcString(1000277))
			s.StartTimerAtFixedRate("1001", n, nil, 1000*time.Millisecond, 60000*time.Millisecond)
			n.SetSlot(script.SlotCreature0, n)
			n.SetInt(script.SlotIntAI0, 0)
			base.Created(s, e)
		},
		OnTimer: func(s *script.Script, e script.Timer) string {
			if strings.EqualFold(e.Name, "1001") {
				n := e.NPC
				if !n.InMyTerritory() && s.Rnd(3) < 1 && n.HasMaster() {
					n.TeleportTo(n.Master())
					n.RemoveAllAttackDesire()
				}
				shuffle(s, n)
			}
			return base.Timer(s, e)
		},
		OnPartyAttacked: func(s *script.Script, e script.PartyAttacked) {
			called := e.Called
			if c0 := called.Slot(script.SlotCreature0); s.Rnd(3) < 1 && !script.Same(c0, called) && called.InSpawnTerritory(c0) && called.CanAttemptCast(called, level1(reuseCurse)) {
				called.AddMoveToDesire(script.LocOf(c0), 100000000)
			}
			partyHate(called, e.Caller, e.Target, e.Damage)
			escapeOnce(called, script.NpcString(1000280))
		},
		OnUseSkillFinished: func(_ *script.Script, e script.UseSkillFinished) {
			escaped(e.NPC, e)
		},
		OnSeeSpell: func(s *script.Script, e script.SeeSpell) {
			switch e.Caster.ClassID() {
			case classBishop, classProphet, classElvenElder, classShillienElder, classOverlord, classWarcryer:
				e.NPC.SetSlot(script.SlotCreature0, e.Caster)
			}
			spellHate(s, e)
		},
		OnMoveToFinished: func(_ *script.Script, e script.MoveToFinished) {
			e.NPC.AddCastDesire(e.NPC, level1(reuseCurse), 1000000)
		},
	})}
}

// Gustav is Agit02Gustav: the Devastated Castle's commander. He raises his
// lieutenants and 30 knights and archers around him as he appears. Every
// 60 s, out of his territory, he may jump home, and he may shuffle his
// hate. Once escaped, he dies 3 s later; until then a fallen private comes
// back after 300 s.
func Gustav() script.Script {
	base := defaultnpc.New().Hooks
	return script.Script{Dir: dir, Behavior: true, NPCs: []int32{35410}, Hooks: base.With(script.Hooks{
		OnCreated: func(s *script.Script, e script.Created) {
			n := e.NPC
			n.Shout(script.NpcString(1000275))
			n.SetInt(script.SlotWeightPoint, 100)
			n.SetInt(script.SlotIntAI0, 0)
			n.SetInt(script.SlotIntAI1, 0)
			for _, p := range gustavPrivates() {
				s.Spawner().CreateOnePrivateEx(n, p.id, script.Loc{X: p.x, Y: p.y, Z: p.z, Heading: p.heading}, 0, script.Params{})
			}
			s.StartTimerAtFixedRate("1001", n, nil, 1000*time.Millisecond, 60000*time.Millisecond)
			base.Created(s, e)
		},
		OnTimer: func(s *script.Script, e script.Timer) string {
			n := e.NPC
			if strings.EqualFold(e.Name, "1001") {
				if !n.InMyTerritory() && s.Rnd(3) < 1 {
					n.TeleportTo(n.SpawnLocation())
					n.RemoveAllAttackDesire()
				}
				shuffle(s, n)
			}
			if strings.EqualFold(e.Name, "1002") {
				n.DoDie(n)
				n.SetInt(script.SlotIntAI1, 1)
			}
			return base.Timer(s, e)
		},
		OnPartyAttacked: func(s *script.Script, e script.PartyAttacked) {
			called := e.Called
			if s.Rnd(3) < 1 {
				called.AddCastDesire(e.Target, level1(gustavHit), 1000000)
			}
			partyHate(called, e.Caller, e.Target, e.Damage)
			escapeOnce(called, script.NpcString(1000278))
		},
		OnUseSkillFinished: func(s *script.Script, e script.UseSkillFinished) {
			if escaped(e.NPC, e) {
				s.StartTimer("1002", e.NPC, nil, 3000*time.Millisecond)
			}
		},
		OnSeeSpell: spellHate,
		OnPartyDied: func(s *script.Script, e script.PartyDied) {
			if !script.Same(e.Caller, e.Called) && e.Called.Int(script.SlotIntAI1) != 1 {
				s.Spawner().ScheduleRespawn(e.Caller, 300000*time.Millisecond)
			}
		},
	})}
}

// private is one private a hall NPC raises: its NPC id and where it
// stands and faces.
type private struct {
	id               int32
	x, y, z, heading int
}

// gustavPrivates are the privates Gustav raises: Mikhail, Dietrich, 15
// doom knights and 15 doom archers.
func gustavPrivates() []private {
	return []private{
		{35409, 178304, -17712, -2194, 32768},
		{35408, 178306, -17535, -2195, 32768},
		{35416, 178178, -17346, -2226, 32768},
		{35416, 178110, -17768, -2226, 32768},
		{35416, 178132, -17683, -2226, 32768},
		{35416, 178180, -17953, -2226, 32768},
		{35416, 178147, -17172, -2226, 32768},
		{35416, 178056, -17424, -2226, 32768},
		{35416, 178120, -17331, -2226, 32768},
		{35416, 178124, -17577, -2226, 32768},
		{35416, 178159, -18138, -2226, 32768},
		{35416, 178396, -18050, -2226, 32768},
		{35416, 178078, -17917, -2226, 32768},
		{35416, 178261, -17269, -2226, 32768},
		{35416, 178152, -18063, -2226, 32768},
		{35416, 178025, -17541, -2226, 32768},
		{35416, 178285, -18013, -2226, 32768},
		{35413, 178112, -17968, -2221, 32768},
		{35413, 178079, -18092, -2221, 32768},
		{35413, 178039, -17732, -2221, 32768},
		{35413, 178224, -18025, -2221, 32768},
		{35413, 178111, -17510, -2221, 32768},
		{35413, 178088, -17620, -2221, 32768},
		{35413, 178059, -17223, -2221, 32768},
		{35413, 178043, -17325, -2221, 32768},
		{35413, 178311, -17946, -2221, 32768},
		{35413, 178170, -17885, -2221, 32768},
		{35413, 178317, -18097, -2221, 32768},
		{35413, 178099, -17823, -2221, 32768},
		{35413, 178290, -17335, -2221, 32768},
		{35413, 178116, -17386, -2221, 32768},
		{35413, 178169, -17266, -2221, 32768},
	}
}

// Mikhail is Agit02Mikhail: Gustav's other lieutenant. Every 60 s, out of
// his territory, he may jump to his master, and he may shuffle his hate.
// He may strike whoever hits his party.
func Mikhail() script.Script {
	base := defaultnpc.New().Hooks
	return script.Script{Dir: dir, Behavior: true, NPCs: []int32{35409}, Hooks: base.With(script.Hooks{
		OnCreated: func(s *script.Script, e script.Created) {
			n := e.NPC
			n.Shout(script.NpcString(1000276))
			n.SetInt(script.SlotIntAI0, 0)
			s.StartTimerAtFixedRate("1001", n, nil, 1000*time.Millisecond, 60000*time.Millisecond)
			base.Created(s, e)
		},
		OnTimer: func(s *script.Script, e script.Timer) string {
			if strings.EqualFold(e.Name, "1001") {
				n := e.NPC
				if !n.InMyTerritory() && s.Rnd(3) < 1 && n.HasMaster() {
					n.TeleportTo(n.Master())
					n.RemoveAllAttackDesire()
				}
				shuffle(s, n)
			}
			return base.Timer(s, e)
		},
		OnPartyAttacked: func(s *script.Script, e script.PartyAttacked) {
			called := e.Called
			if s.Rnd(3) < 1 {
				called.AddCastDesire(e.Target, level1(mikhailHit), 1000000)
			}
			partyHate(called, e.Caller, e.Target, e.Damage)
			escapeOnce(called, script.NpcString(1000279))
		},
		OnUseSkillFinished: func(_ *script.Script, e script.UseSkillFinished) {
			escaped(e.NPC, e)
		},
		OnSeeSpell: spellHate,
	})}
}
