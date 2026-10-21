package raid

import (
	"time"

	"github.com/fatal10110/acis_golang/internal/gameserver/script"
	"github.com/fatal10110/acis_golang/internal/gameserver/script/ai/defaultnpc"
)

// maxPrivateRespawns is how many times RaidBossAloneSummonPrivate brings
// a dead private back.
const maxPrivateRespawns = 32

// BossAloneSummonPrivate is RaidBossAloneSummonPrivate: a RaidBossType1
// that creates its privates as it spawns, with a weight point of 10. A
// playable's hit on its party makes it want to attack the playable at 100
// per point of damage (at least 1) over its level plus 7, truncated; each
// of its dying privates comes back after its spawn's respawn delay, 32
// times at most while it lives.
func BossAloneSummonPrivate() script.Script {
	return script.Script{Dir: "ai/individual/Monster/RaidBoss/RaidBossAlone/RaidBossType1/RaidBossAloneSummonPrivate", Behavior: true, NPCs: []int32{29040}, Hooks: bossAloneSummonPrivateHooks()}
}

// bossAloneSummonPrivateHooks are RaidBossAloneSummonPrivate's hooks. Its
// party-died hook calls no parent.
func bossAloneSummonPrivateHooks() script.Hooks {
	base := BossType1Hooks()
	return base.With(script.Hooks{
		OnCreated: func(s *script.Script, e script.Created) {
			n := e.NPC
			n.SetInt(script.SlotWeightPoint, 10)
			defaultnpc.CreatePrivates(s, n)
			n.SetInt(script.SlotIntAI0, 0)
			base.Created(s, e)
		},
		OnPartyAttacked: func(s *script.Script, e script.PartyAttacked) {
			if e.Target.Playable() {
				damage := e.Damage
				if damage == 0 {
					damage = 1
				}
				e.Called.AddAttackDesire(e.Target, float64(int32(((1.0*float64(damage))/float64(e.Called.Level()+7))*100)))
			}
			base.PartyAttacked(s, e)
		},
		OnPartyDied: func(s *script.Script, e script.PartyDied) {
			caller, called := e.Caller, e.Called
			if called.Int(script.SlotIntAI0) < maxPrivateRespawns && !script.Same(caller, called) && !called.Dead() {
				s.Spawner().ScheduleRespawn(caller, time.Duration(s.Spawner().RespawnDelay(caller))*1000*time.Millisecond)
				called.SetInt(script.SlotIntAI0, called.Int(script.SlotIntAI0)+1)
			}
		},
	}).Named("RaidBossAloneSummonPrivate")
}
