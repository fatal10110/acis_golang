package npcs

import (
	"sync/atomic"
	"testing"
	"time"

	"github.com/fatal10110/acis_golang/internal/commons/wire"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/script"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
)

// deathIdleID is the civilian id the death idle scenario binds its
// no-desire behavior to.
const deathIdleID = 13031

// TestDeadFolkIdlesIntoItsWalkStance pins CreatureAI.onEvtDead's
// doIdleIntention and NpcAI.thinkIdle: a civilian NPC in its run stance
// dies, observers see it die and leave its attack stance, then see
// it switch back to its walk stance (ChangeMoveType, walk) and its NpcInfo
// again, and its no-desire hook runs once.
func TestDeadFolkIdlesIntoItsWalkStance(t *testing.T) {
	t.Parallel()
	var noDesire atomic.Int32
	behavior := func() script.Script {
		return script.Script{Behavior: true, NPCs: []int32{deathIdleID}, Hooks: script.Hooks{
			OnNoDesire: func(*script.Script, script.NoDesire) { noDesire.Add(1) },
		}}
	}
	w := bootFolkWorld(t, nil, gameservertest.WithAttackStanceClock(time.Now),
		gameservertest.WithNPCScripts(map[int32]script.NPCKind{deathIdleID: script.KindFolk},
			[]script.Listing{{Path: "ai.FolkDeathIdle"}}, script.Catalog{"ai.FolkDeathIdle": behavior}))
	tmpl := folkTemplate("Folk", deathIdleID)
	tmpl.Undying = false
	tmpl.CanMove = true
	tmpl.HPMax = 1
	f := w.spawnFolk(t, tmpl, 40)
	if !f.Running() {
		t.Fatal("civilian NPC spawned walking, want run stance")
	}
	w.selectFolk(t, f)
	before := noDesire.Load()

	w.c.Send(encodeAttackRequest(f.ObjectID(), w.at))
	var frames [][]byte
	advanceUntilSwingLands(t, w.srv, "the civilian NPC dying to the swing", func() bool {
		frames = append(frames, drainFrames(t, w.c)...)
		return f.Dead()
	})
	w.srv.Settle(t)
	frames = append(frames, drainFrames(t, w.c)...)

	die := frameAt(frames, serverpackets.OpcodeDie, f.ObjectID())
	stop := frameAt(frames, serverpackets.OpcodeAutoAttackStop, f.ObjectID())
	if die < 0 || stop < die {
		t.Fatalf("death frames %x: Die at %d, AutoAttackStop at %d; want them in that order", opcodes(frames), die, stop)
	}
	walk, info := -1, -1
	for i := stop + 1; i < len(frames); i++ {
		if walk < 0 && objectFrame(frames[i], serverpackets.OpcodeChangeMoveType, f.ObjectID()) {
			walk = i
			r := wire.NewReader(frames[i][1:])
			r.ReadInt32()
			if running := r.ReadInt32(); running != 0 {
				t.Fatalf("ChangeMoveType after death running = %d, want 0 (walk)", running)
			}
		}
		if walk >= 0 && info < 0 && objectFrame(frames[i], serverpackets.OpcodeNPCInfo, f.ObjectID()) {
			info = i
		}
	}
	if walk < 0 || info < walk {
		t.Fatalf("death frames %x: ChangeMoveType at %d, NpcInfo at %d after AutoAttackStop at %d; want the walk stance then the NpcInfo", opcodes(frames), walk, info, stop)
	}
	if f.Running() {
		t.Fatal("dead NPC still in run stance, want walk")
	}
	if got := noDesire.Load() - before; got != 1 {
		t.Fatalf("no-desire hook ran %d times on death, want once", got)
	}
}
