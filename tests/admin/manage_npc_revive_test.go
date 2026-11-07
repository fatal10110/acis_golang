package admin

import (
	"encoding/binary"
	"slices"
	"testing"
	"time"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/npc"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/world"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
	"github.com/fatal10110/acis_golang/internal/testsupport"
)

// reviveCorpseTime is the corpse time of the NPCs revived here, in seconds.
const reviveCorpseTime = 7

// revivedHP is the HP a revived NPC of maxHP stands up with: the harness
// RespawnRestoreHP share of it, 0.7.
func revivedHP(maxHP float64) float64 { return maxHP * 0.7 }

// requireRevivedAfterHP fails unless frames carry, for id, a StatusUpdate
// of hp current HP, in whole points, followed by a Revive.
func requireRevivedAfterHP(t *testing.T, frames [][]byte, id int32, hp float64) {
	t.Helper()
	status, revive := -1, -1
	for i, f := range frames {
		if len(f) < 5 || int32(binary.LittleEndian.Uint32(f[1:5])) != id {
			continue
		}
		switch f[0] {
		case serverpackets.OpcodeStatusUpdate:
			// opcode, object id, attribute count, then (type, value) pairs.
			if len(f) >= 17 && binary.LittleEndian.Uint32(f[5:9]) == 1 &&
				binary.LittleEndian.Uint32(f[9:13]) == uint32(serverpackets.StatusCurrentHP) &&
				binary.LittleEndian.Uint32(f[13:17]) == uint32(hp) && status < 0 {
				status = i
			}
		case serverpackets.OpcodeRevive:
			revive = i
		}
	}
	if status < 0 || revive < status {
		t.Fatalf("//res frames = %x, want a StatusUpdate of %d HP then the Revive of %d", testsupport.FrameOpcodes(frames), int(hp), id)
	}
}

// requireNoRevive fails when frames carry a Revive or an ActionFailed.
func requireNoRevive(t *testing.T, what string, frames [][]byte) {
	t.Helper()
	ops := opcodesOf(frames)
	if slices.Contains(ops, serverpackets.OpcodeRevive) || slices.Contains(ops, serverpackets.OpcodeActionFailed) {
		t.Fatalf("%s frames = %x, want neither Revive nor ActionFailed", what, testsupport.FrameOpcodes(frames))
	}
}

// TestAdminManageMonsterKillRes pins //kill then //res on a selected
// monster: the res stands the corpse up with the configured share of its
// maximum HP and its MP as it was, its watcher sees that HP before the
// Revive, and its corpse no longer decays: it is still in the world, alive,
// past its corpse time. A //res on the living monster changes nothing.
func TestAdminManageMonsterKillRes(t *testing.T) {
	t.Parallel()
	decay := newManualDecay(t)
	srv, gmID := bootAdmin(t, adminLevel, gameservertest.WithDecay(decay.task))
	decay.state = srv.State
	gm := srv.Client
	enterWorld(t, gm)
	x, y, z := srv.PlayerPosition(t, gmID)
	h := srv.SpawnHostileNPCTemplateAt(t, &npc.Template{
		ID: 100, TemplateID: 100, Type: "Monster", Level: 1, HPMax: 1000, MPMax: 100, CorpseTime: reviveCorpseTime,
		AtkSpd: 300, RunSpeed: 120, WalkSpeed: 60, CollisionRadius: 8, CollisionHeight: 20,
	}, location.Location{X: x + 60, Y: y, Z: z})
	drain(t, gm)
	exchange(t, gm, encodeAction(h.ObjectID()))
	var mp float64
	onParaQueue(t, h.Queue(), func() {
		h.ReduceMP(40)
		mp = h.MPValue()
	})
	drain(t, gm)

	requireNoRevive(t, "//res on a living monster", exchange(t, gm, encodeBuildCmd("res")))
	onParaQueue(t, h.Queue(), func() {})
	if h.HP() != h.MaxHPValue() || h.MPValue() != mp {
		t.Fatalf("living monster after //res HP %v MP %v, want %v and %v", h.HP(), h.MPValue(), h.MaxHPValue(), mp)
	}

	exchange(t, gm, encodeBuildCmd("kill"))
	onParaQueue(t, h.Queue(), func() {})
	if !h.Dead() || !decay.task.Tracked(h) {
		t.Fatalf("after //kill: dead %v, decay tracked %v, want both", h.Dead(), decay.task.Tracked(h))
	}
	drain(t, gm)

	frames := exchange(t, gm, encodeBuildCmd("res"))
	onParaQueue(t, h.Queue(), func() {})
	frames = append(frames, settle(t, gm)...)
	requireRevivedAfterHP(t, frames, h.ObjectID(), revivedHP(h.MaxHPValue()))
	requireNoSystemMessage(t, "//res", frames)
	onParaQueue(t, h.Queue(), func() {
		if h.Dead() || h.HP() != revivedHP(h.MaxHPValue()) || h.MPValue() != mp {
			t.Errorf("revived monster dead %v HP %v MP %v, want alive, %v, %v", h.Dead(), h.HP(), h.MPValue(), revivedHP(h.MaxHPValue()), mp)
		}
	})
	if decay.task.Tracked(h) || h.HasCorpse() {
		t.Fatalf("revived monster: decay tracked %v, corpse %v, want neither", decay.task.Tracked(h), h.HasCorpse())
	}

	decay.passAndTick(t, srv, 2*reviveCorpseTime*time.Second)
	if obj, ok := srv.State.Object(h.ObjectID()); !ok || obj != world.Tracked(h) || h.Dead() || h.Decayed() {
		t.Fatal("revived monster left the world at its old corpse decay")
	}
}

// TestAdminManageFolkKillRes pins //kill then //res on a selected civilian
// NPC: as for a monster, it stands up with the configured share of its HP
// and its MP, its watcher sees that HP before the Revive, and its corpse no
// longer decays.
func TestAdminManageFolkKillRes(t *testing.T) {
	t.Parallel()
	decay := newManualDecay(t)
	srv, gmID := bootAdmin(t, adminLevel, gameservertest.WithDecay(decay.task))
	decay.state = srv.State
	gm := srv.Client
	enterWorld(t, gm)
	x, y, z := srv.PlayerPosition(t, gmID)
	f := srv.SpawnFolkNPCAt(t, &npc.Template{
		ID: 31932, TemplateID: 31932, Type: "Merchant", Level: 1, HPMax: 1000, MPMax: 100, CorpseTime: reviveCorpseTime,
		AtkSpd: 300, RunSpeed: 120, WalkSpeed: 60, CollisionRadius: 8, CollisionHeight: 20,
	}, location.Location{X: x + 60, Y: y, Z: z})
	drain(t, gm)
	exchange(t, gm, encodeAction(f.ObjectID()))
	var mp float64
	onParaQueue(t, f.Queue(), func() {
		f.ReduceMP(40)
		mp = f.MPValue()
	})
	drain(t, gm)

	exchange(t, gm, encodeBuildCmd("kill"))
	onParaQueue(t, f.Queue(), func() {})
	if !f.Dead() || !decay.task.Tracked(f) {
		t.Fatalf("after //kill: dead %v, decay tracked %v, want both", f.Dead(), decay.task.Tracked(f))
	}
	drain(t, gm)

	frames := exchange(t, gm, encodeBuildCmd("res"))
	onParaQueue(t, f.Queue(), func() {})
	frames = append(frames, settle(t, gm)...)
	requireRevivedAfterHP(t, frames, f.ObjectID(), revivedHP(f.MaxHPValue()))
	onParaQueue(t, f.Queue(), func() {
		if f.Dead() || f.HP() != revivedHP(f.MaxHPValue()) || f.MPValue() != mp {
			t.Errorf("revived folk dead %v HP %v MP %v, want alive, %v, %v", f.Dead(), f.HP(), f.MPValue(), revivedHP(f.MaxHPValue()), mp)
		}
	})
	if decay.task.Tracked(f) || f.HasCorpse() {
		t.Fatalf("revived folk: decay tracked %v, corpse %v, want neither", decay.task.Tracked(f), f.HasCorpse())
	}

	decay.passAndTick(t, srv, 2*reviveCorpseTime*time.Second)
	if obj, ok := srv.State.Object(f.ObjectID()); !ok || obj != world.Tracked(f) || f.Dead() || f.Decayed() {
		t.Fatal("revived folk left the world at its old corpse decay")
	}
}
