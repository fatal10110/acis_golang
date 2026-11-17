package npcs

import (
	"sync/atomic"
	"testing"
	"time"

	"github.com/fatal10110/acis_golang/internal/commons/wire"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/script"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
)

// hostileDeathIdleID is the monster id the death idle scenario binds its
// no-desire behavior to.
const hostileDeathIdleID = 13032

// TestDeadHostileIdlesIntoItsWalkStance pins CreatureAI.onEvtDead's
// doIdleIntention and NpcAI.thinkIdle for a monster: a monster in its run
// stance dies, observers see it die and leave its attack stance, then see
// it switch back to its walk stance (ChangeMoveType, walk) and its NpcInfo
// again, and its no-desire hook runs once on the corpse.
func TestDeadHostileIdlesIntoItsWalkStance(t *testing.T) {
	t.Parallel()
	var deadNoDesire atomic.Int32
	behavior := func() script.Script {
		return script.Script{Behavior: true, NPCs: []int32{hostileDeathIdleID}, Hooks: script.Hooks{
			OnNoDesire: func(_ *script.Script, e script.NoDesire) {
				if e.NPC.Dead() {
					deadNoDesire.Add(1)
				}
			},
		}}
	}
	w := bootFolkWorld(t, nil, gameservertest.WithAttackStanceClock(time.Now),
		gameservertest.WithNPCScripts(map[int32]script.NPCKind{hostileDeathIdleID: script.KindHostile},
			[]script.Listing{{Path: "ai.HostileDeathIdle"}}, script.Catalog{"ai.HostileDeathIdle": behavior}))
	tmpl := spawnMonster(hostileDeathIdleID, "Corpse")
	tmpl.HPMax, tmpl.CON = 1, 43
	h := w.srv.SpawnHostileNPCTemplateAt(t, tmpl, location.Location{X: w.at.X + 40, Y: w.at.Y, Z: w.at.Z})
	drainUntilQuiet(t, w.c)
	if !h.Running() {
		t.Fatal("monster spawned walking, want run stance")
	}
	w.c.Send(encodeAction(h.ObjectID(), w.at, false))
	drainUntilQuiet(t, w.c)

	w.c.Send(encodeAttackRequest(h.ObjectID(), w.at))
	var frames [][]byte
	advanceUntilSwingLands(t, w.srv, "the monster dying to the swing", func() bool {
		frames = append(frames, drainFrames(t, w.c)...)
		return h.Dead()
	})
	w.srv.Settle(t)
	frames = append(frames, drainFrames(t, w.c)...)

	die := frameAt(frames, serverpackets.OpcodeDie, h.ObjectID())
	stop := frameAt(frames, serverpackets.OpcodeAutoAttackStop, h.ObjectID())
	if die < 0 || stop < die {
		t.Fatalf("death frames %x: Die at %d, AutoAttackStop at %d; want them in that order", opcodes(frames), die, stop)
	}
	walk, info := -1, -1
	for i := stop + 1; i < len(frames); i++ {
		if walk < 0 && objectFrame(frames[i], serverpackets.OpcodeChangeMoveType, h.ObjectID()) {
			walk = i
			r := wire.NewReader(frames[i][1:])
			r.ReadInt32()
			if running := r.ReadInt32(); running != 0 {
				t.Fatalf("ChangeMoveType after death running = %d, want 0 (walk)", running)
			}
		}
		if walk >= 0 && info < 0 && objectFrame(frames[i], serverpackets.OpcodeNPCInfo, h.ObjectID()) {
			info = i
		}
	}
	if walk < 0 || info < walk {
		t.Fatalf("death frames %x: ChangeMoveType at %d, NpcInfo at %d after AutoAttackStop at %d; want the walk stance then the NpcInfo", opcodes(frames), walk, info, stop)
	}
	if h.Running() {
		t.Fatal("dead monster still in run stance, want walk")
	}
	if got := deadNoDesire.Load(); got != 1 {
		t.Fatalf("no-desire hook ran %d times on the corpse, want once", got)
	}
}
