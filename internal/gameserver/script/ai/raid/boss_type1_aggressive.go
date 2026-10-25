package raid

import (
	"strings"
	"time"

	"github.com/fatal10110/acis_golang/internal/gameserver/script"
)

// aggressiveDir is the directory RaidBossType1Aggressive and its children
// share.
const aggressiveDir = "ai/individual/Monster/RaidBoss/RaidBossAlone/RaidBossType1/RaidBossType1Aggressive"

// BossType1Aggressive is RaidBossType1Aggressive: a RaidBossType1 that
// wants to attack, at 200, a playable it sees while it stands in its
// territory. It binds no NPC of its own.
func BossType1Aggressive() script.Script {
	return script.Script{Dir: aggressiveDir, Behavior: true, Hooks: bossType1AggressiveHooks()}
}

// bossType1AggressiveHooks are RaidBossType1Aggressive's hooks, which its
// children derive theirs from. Its see-creature hook calls no parent.
func bossType1AggressiveHooks() script.Hooks {
	base := BossType1Hooks()
	return base.With(script.Hooks{
		OnSeeCreature: func(_ *script.Script, e script.SeeCreature) {
			if e.Creature.Playable() && e.NPC.InMyTerritory() {
				e.NPC.AddAttackDesire(e.Creature, 200)
			}
		},
	}).Named("RaidBossType1Aggressive")
}

// DrChaosGiganticGolem is the gigantic golem Doctor Chaos calls. 1 s after
// it appears it plays a camera shot on itself for the players around, and
// 10 s later it starts fighting: from then on it wants to attack, at 200,
// a playable it sees, on top of its parent's. A hit or a sighting keeps it
// awake; with nothing to do 1200 s after the last, it says its farewell,
// tells its master event 10029 and leaves the world. Dying, it says its
// last line and tells its master 10029.
func DrChaosGiganticGolem() script.Script {
	return script.Script{Dir: aggressiveDir, Behavior: true, NPCs: []int32{25512}, Hooks: drChaosGiganticGolemHooks()}
}

// drChaosGiganticGolemHooks are DrChaosGiganticGolem's hooks. Its
// no-desire and dying hooks call no parent.
func drChaosGiganticGolemHooks() script.Hooks {
	base := bossType1AggressiveHooks()
	return base.With(script.Hooks{
		OnCreated: func(s *script.Script, e script.Created) {
			n := e.NPC
			s.StartTimer("1001", n, nil, 1000*time.Millisecond)
			n.SetInt(script.SlotIntAI1, s.CurrentTick())
			n.SetInt(script.SlotIntAI2, 0)
			base.Created(s, e)
		},
		OnTimer: func(s *script.Script, e script.Timer) string {
			n := e.NPC
			if strings.EqualFold(e.Name, "1001") {
				n.SpecialCamera(script.CameraShot{Distance: 30, Yaw: -200, Pitch: 20, Time: 6000, Duration: 8000, Turn: 0, Rise: 0, Widescreen: 0, Unknown: 0})
				s.StartTimer("1002", n, nil, 10000*time.Millisecond)
			} else if strings.EqualFold(e.Name, "1002") {
				n.SetInt(script.SlotIntAI2, 1)
			}
			return base.Timer(s, e)
		},
		OnAttacked: func(s *script.Script, e script.Attacked) {
			e.NPC.SetInt(script.SlotIntAI1, s.CurrentTick())
			base.Attacked(s, e)
		},
		OnSeeCreature: func(s *script.Script, e script.SeeCreature) {
			n := e.NPC
			n.SetInt(script.SlotIntAI1, s.CurrentTick())
			if !e.Creature.Playable() {
				return
			}
			if n.Int(script.SlotIntAI2) == 1 {
				n.AddAttackDesire(e.Creature, 200)
			}
			base.SeeCreature(s, e)
		},
		OnNoDesire: func(s *script.Script, e script.NoDesire) {
			n := e.NPC
			if s.ElapsedTicks(n.Int(script.SlotIntAI1)) > 1200 {
				n.Say(script.NpcString(1010582))
				if n.HasMaster() {
					s.SendScriptEvent(n.Master(), 10029, 0, 0)
				}
				n.DeleteMe()
			}
		},
		OnMyDying: func(s *script.Script, e script.MyDying) {
			n := e.NPC
			n.Say(script.NpcString(1010583))
			if n.HasMaster() {
				s.SendScriptEvent(n.Master(), 10029, 0, 0)
			}
		},
	}).Named("DrChaosGiganticGolem")
}

// FreyaRoyalGuard is Freya's royal guard. Dying, it runs maker script
// event 10005 on the npcmaker schuttgart13_mb2314_05m1 and tells the NPCs
// within 7000 event 11036 with 3.
func FreyaRoyalGuard() script.Script {
	return script.Script{Dir: aggressiveDir, Behavior: true, NPCs: []int32{29060}, Hooks: freyaRoyalGuardHooks()}
}

// freyaRoyalGuardHooks are FreyaRoyalGuard's hooks. Its dying hook calls
// no parent.
func freyaRoyalGuardHooks() script.Hooks {
	base := bossType1AggressiveHooks()
	return base.With(script.Hooks{
		OnMyDying: func(s *script.Script, e script.MyDying) {
			s.Spawner().SendMakerEvent("schuttgart13_mb2314_05m1", "10005", 0, 0)
			s.BroadcastScriptEvent(e.NPC, 11036, 3, 7000)
		},
	}).Named("FreyaRoyalGuard")
}
