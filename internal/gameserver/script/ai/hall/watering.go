package hall

import (
	"time"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/npc"
	"github.com/fatal10110/acis_golang/internal/gameserver/script"
	"github.com/fatal10110/acis_golang/internal/gameserver/script/ai/defaultnpc"
)

// Gourd is AzitWateringGroud: a Rainbow Springs hot springs gourd, which
// has nothing to say and, as it dies, tells the NPCs within 500 of it,
// with script event 5, that its killer broke it.
func Gourd() script.Script {
	base := defaultnpc.New().Hooks
	return script.Script{Dir: dir, Behavior: true, NPCs: []int32{35588, 35589, 35590, 35591}, Hooks: base.With(script.Hooks{
		OnFirstTalk: func(*script.Script, script.FirstTalk) string { return "" },
		OnMyDying: func(s *script.Script, e script.MyDying) {
			s.BroadcastScriptEventEx(e.NPC, 5, 40000, e.Killer.ObjectID(), 500)
		},
	})}
}

// Mimic is AzitWateringMimic: a Rainbow Springs treasure chest that fights
// back at the playable hitting it, by the damage over its level plus 7; a
// hit for nothing counts as one point.
func Mimic() script.Script {
	base := defaultnpc.New().Hooks
	return script.Script{Dir: dir, Behavior: true, NPCs: []int32{35593}, Hooks: base.With(script.Hooks{
		OnAttacked: func(_ *script.Script, e script.Attacked) {
			if !e.Attacker.Playable() {
				return
			}
			damage := e.Damage
			if damage == 0 {
				damage = 1
			}
			e.NPC.AddAttackDesire(e.Attacker, float64(damage)/float64(e.NPC.Level()+7)*100)
		},
	})}
}

// MimicMagical is AzitWateringMimicMagical: a Rainbow Springs treasure
// chest that, on a draw under 1 of 9 at each hit, wants to cast its DD
// magic at the attacker, and vanishes once a cast of it lands.
func MimicMagical() script.Script {
	base := defaultnpc.New().Hooks
	return script.Script{Dir: dir, Behavior: true, NPCs: []int32{35594}, Hooks: base.With(script.Hooks{
		OnAttacked: func(s *script.Script, e script.Attacked) {
			if s.Rnd(9) < 1 {
				castSkill(e.NPC, e.Attacker, npc.SkillTypeDDMagic)
			}
		},
		OnUseSkillFinished: func(_ *script.Script, e script.UseSkillFinished) {
			if dd, ok := e.NPC.SkillByType(npc.SkillTypeDDMagic); ok && dd == e.Skill && e.Success {
				e.NPC.DeleteMe()
			}
		},
	})}
}

// RecycleCan is AzitWateringRecycleCan: the Rainbow Springs sweeper. As it
// appears, then every 30 s, it looks for the items lying within 450 of it
// and wants to pick each up, the hot springs game's letters at twice the
// weight; it shouts as it picks up a bottle or a crystal of the game, says
// a line to whoever talks to it, and leaves the world 5 minutes after it
// appeared.
func RecycleCan() script.Script {
	base := defaultnpc.New().Hooks
	return script.Script{Dir: dir, Behavior: true, NPCs: []int32{35600}, Hooks: base.With(script.Hooks{
		OnCreated: func(s *script.Script, e script.Created) {
			s.StartTimer("1011", e.NPC, nil, 5*60*1000*time.Millisecond)
			s.StartTimerAtFixedRate("1002", e.NPC, nil, 30000*time.Millisecond, 30000*time.Millisecond)
			e.NPC.LookItem(450, 20)
		},
		OnFirstTalk: func(_ *script.Script, e script.FirstTalk) string {
			e.NPC.Say(script.NpcString(1010639))
			return ""
		},
		OnTimer: func(_ *script.Script, e script.Timer) string {
			switch e.Name {
			case "1002":
				e.NPC.LookItem(450, 20)
			case "1011":
				e.NPC.DeleteMe()
			}
			return ""
		},
		OnSeeItem: func(_ *script.Script, e script.SeeItem) {
			for i, item := range e.Items {
				weight := 10000 - i
				if item.ItemID >= 8035 && item.ItemID <= 8055 {
					weight = (10000 - i) * 2
				}
				e.NPC.AddPickUpDesire(item.ObjectID, float64(weight))
			}
		},
		OnPickedItem: func(_ *script.Script, e script.PickedItem) {
			if e.Item.ItemID == 8190 || e.Item.ItemID == 8689 {
				e.NPC.Say(script.NpcString(1800023))
			}
		},
	})}
}

// treasureDrops are the letters a Rainbow Springs treasure box drops, each
// for the draws of 100 up to its bound and over the previous one. A draw
// over 95 drops nothing.
func treasureDrops() []struct{ upTo, item int32 } {
	return []struct{ upTo, item int32 }{
		{5, 8035},
		{10, 8036},
		{15, 8037},
		{20, 8038},
		{25, 8039},
		{30, 8040},
		{35, 8041},
		{40, 8042},
		{45, 8043},
		{50, 8045},
		{55, 8046},
		{60, 8047},
		{65, 8048},
		{70, 8049},
		{75, 8050},
		{80, 8051},
		{85, 8052},
		{90, 8053},
		{95, 8054},
	}
}

// TreasureBox is AzitWateringTreasureBox: a Rainbow Springs treasure box,
// invulnerable from the moment it appears. Only a playable's barehanded
// blow, with no weapon in its right hand and no skill, takes its hit
// points, and the one that would take the last kills it. As it dies it
// drops one or two letters of the hot springs game to its killer.
func TreasureBox() script.Script {
	base := defaultnpc.New().Hooks
	return script.Script{Dir: dir, Behavior: true, NPCs: []int32{35595}, Hooks: base.With(script.Hooks{
		OnCreated: func(_ *script.Script, e script.Created) {
			e.NPC.SetInvul(true)
		},
		OnAttacked: func(_ *script.Script, e script.Attacked) {
			n, attacker := e.NPC, e.Attacker
			if !attacker.Playable() || attacker.HoldsWeapon() || e.Skill.ID != 0 {
				return
			}
			if hp := n.HP() - float64(e.Damage); hp <= 0 {
				n.DoDie(attacker)
			} else {
				n.SetHP(hp)
			}
		},
		OnMyDying: func(s *script.Script, e script.MyDying) {
			drops := s.Rnd(2) + 1
			for range drops {
				i0 := int32(s.Rnd(100))
				for _, d := range treasureDrops() {
					if i0 <= d.upTo {
						e.NPC.DropItem(e.Killer, d.item, 1)
						break
					}
				}
			}
		},
	})}
}
