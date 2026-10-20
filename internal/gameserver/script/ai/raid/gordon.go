package raid

import (
	"strings"
	"time"

	"github.com/fatal10110/acis_golang/internal/gameserver/script"
)

// Gordon is the raid boss Gordon, a RaidBossType1 that hunts the cursed
// weapons Zariche (8190) and Akamanah (8689). It walks the gordon route.
// It looks for a cursed weapon on the ground within 500 every 3 s and
// after each attack that leaves its target dead or was not on a player,
// and wants to pick up each one it finds at 1000000; 3 s after picking one
// up it leaves the world. It wants to attack a player wielding one it sees,
// at 1000000, and one carrying one that hits it by 10 per point of damage.
// Every 40 s it looks at the creatures around it. It does not play the
// raid music nor run the raid boss's minute check.
func Gordon() script.Script {
	base := BossType1Hooks()
	return script.Script{Dir: "ai/individual/Monster/RaidBoss/RaidBossAlone/RaidBossType1/Gordon", Behavior: true, NPCs: []int32{29095}, Hooks: base.With(script.Hooks{
		OnAttacked: func(s *script.Script, e script.Attacked) {
			// Only a player carries items a cursed weapon can be among.
			if p, ok := e.Attacker.(*script.Player); ok && p.HasAtLeastOneItem(8190, 8689) {
				e.NPC.AddAttackDesire(e.Attacker, float64(e.Damage*10))
			}
			base.Attacked(s, e)
		},
		OnAttackFinished: func(s *script.Script, e script.AttackFinished) {
			if _, isPlayer := e.Target.(*script.Player); e.Target.Dead() || !isPlayer {
				e.NPC.LookItem(500, 1, 8190, 8689)
			}
			base.AttackFinished(s, e)
		},
		OnCreated: func(s *script.Script, e script.Created) {
			n := e.NPC
			n.SetRunning(false)
			n.AddMoveRouteDesire("gordon", 10)
			s.StartTimerAtFixedRate("2001", n, nil, 40000*time.Millisecond, 40000*time.Millisecond)
			s.StartTimerAtFixedRate("2003", n, nil, 3000*time.Millisecond, 3000*time.Millisecond)
		},
		OnPickedItem: func(s *script.Script, e script.PickedItem) {
			if e.Item.ItemID == 8190 || e.Item.ItemID == 8689 {
				s.StartTimer("2002", e.NPC, nil, 3000*time.Millisecond)
			}
			base.PickedItem(s, e)
		},
		OnSeeCreature: func(s *script.Script, e script.SeeCreature) {
			p := script.ActingPlayer(e.Creature)
			if p == nil {
				return
			}
			if p.CursedWeaponEquipped() {
				e.NPC.AddAttackDesire(e.Creature, 1000000)
			}
			base.SeeCreature(s, e)
		},
		OnSeeItem: func(_ *script.Script, e script.SeeItem) {
			for _, item := range e.Items {
				e.NPC.AddPickUpDesire(item.ObjectID, 1000000)
			}
		},
		OnTimer: func(s *script.Script, e script.Timer) string {
			switch n := e.NPC; {
			case strings.EqualFold(e.Name, "2001"):
				n.LookNeighbor()
			case strings.EqualFold(e.Name, "2002"):
				n.DeleteMe()
			case strings.EqualFold(e.Name, "2003"):
				n.LookItem(500, 1, 8190, 8689)
			}
			return base.Timer(s, e)
		},
	})}
}
