package raid

import (
	"strings"
	"time"

	"github.com/fatal10110/acis_golang/internal/gameserver/script"
)

// BossForTeleportDungeon is RaidBossForTeleportDungeon, the raid boss of a
// dimensional rift room. It tells its npcmaker script event 2 as it
// appears and 3 as it dies. From 180 s after it appears, every minute, it
// tells its npcmaker 3 when nothing hit it for more than 180 s, else 2. A
// hit while it stands out of its territory goes unanswered.
func BossForTeleportDungeon() script.Script {
	return script.Script{Dir: "ai/individual/Monster/RaidBoss/RaidBossAlone/RaidBossType1/RaidBossForTeleportDungeon", Behavior: true, NPCs: []int32{25333, 25334, 25335, 25336, 25337, 25338}, Hooks: bossForTeleportDungeonHooks()}
}

// bossForTeleportDungeonHooks are RaidBossForTeleportDungeon's hooks. Its
// dying hook calls no parent.
func bossForTeleportDungeonHooks() script.Hooks {
	base := BossType1Hooks()
	return base.With(script.Hooks{
		OnAttacked: func(s *script.Script, e script.Attacked) {
			n := e.NPC
			if !n.InMyTerritory() {
				return
			}
			n.SetInt(script.SlotIntAI0, s.CurrentTick())
			base.Attacked(s, e)
		},
		OnCreated: func(s *script.Script, e script.Created) {
			n := e.NPC
			s.StartTimerAtFixedRate("2003", n, nil, 180000*time.Millisecond, 60000*time.Millisecond)
			s.Spawner().SendMakerEvent(n.MakerName(), "2", 0, 0)
			n.SetInt(script.SlotIntAI0, s.CurrentTick())
			base.Created(s, e)
		},
		OnMyDying: func(s *script.Script, e script.MyDying) {
			s.Spawner().SendMakerEvent(e.NPC.MakerName(), "3", 0, 0)
		},
		OnTimer: func(s *script.Script, e script.Timer) string {
			n := e.NPC
			if strings.EqualFold(e.Name, "2003") {
				if s.ElapsedTicks(n.Int(script.SlotIntAI0)) > 60*3 {
					s.Spawner().SendMakerEvent(n.MakerName(), "3", 0, 0)
				} else {
					s.Spawner().SendMakerEvent(n.MakerName(), "2", 0, 0)
				}
			}
			return base.Timer(s, e)
		},
	}).Named("RaidBossForTeleportDungeon")
}
