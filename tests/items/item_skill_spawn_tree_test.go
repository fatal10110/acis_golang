package items

import (
	"context"
	"testing"
	"time"

	"github.com/fatal10110/acis_golang/internal/commons/wire"
	"github.com/fatal10110/acis_golang/internal/gameserver/network"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
	"github.com/fatal10110/acis_golang/internal/testsupport"
)

// Reference: L2SkillSpawn.useSkill places the skill's npcId through a
// plain Spawn whatever its type, so a ChristmasTree template becomes a
// decoration at the caster's position and heading, removed by
// Npc.scheduleDespawn after despawnDelay ms. Skill 2137-1 Summon Regular
// Tree (target SELF) places Christmas Tree 13006 for 1200000 ms.
const (
	summonRegularTreeID int32 = 2137
	regularTreeNPCID          = 13006
	regularTreeDespawn        = 1200 * time.Second
)

// TestSummonRegularTreeSpawnsDecorationThatDespawns casts skill 2137: a
// Christmas Tree decoration appears where the caster stands, facing its
// way, stays until its despawn delay and is then deleted from the world
// and from the client.
func TestSummonRegularTreeSpawnsDecorationThatDespawns(t *testing.T) {
	t.Parallel()
	srv := bootItemSkillTypes(t, gameservertest.WithNPCs(shippedNPCs()), gameservertest.WithNpcSpawns(nil))
	c, objID := srv.Client, srv.SoleObjectID(t)
	if err := srv.KnownSkills.SetKnownSkill(context.Background(), objID, 0, int(summonRegularTreeID), 1); err != nil {
		t.Fatalf("seed known skill: %v", err)
	}
	startInWorld(t, c)
	obj, ok := srv.State.Player(objID)
	if !ok {
		t.Fatal("player not in the world")
	}
	player, ok := network.OnlineCharacter(obj)
	if !ok {
		t.Fatal("player not online")
	}
	px, py, pz := player.Position()
	heading := player.Heading()

	c.Send(encodeRequestMagicSkillUse(summonRegularTreeID))
	// The cast must have begun before the loop below waits for it to end.
	frames := readUntilOpcode(t, c, serverpackets.OpcodeMagicSkillUse)
	srv.AdvanceUntil(t, "tree cast ends", func() bool { return !srv.PlayerCastingNow(t, objID) })
	frames = append(frames, collectUntilQuiet(t, c)...)
	var treeID int32
	for _, f := range frames {
		if f[0] != serverpackets.OpcodeNPCInfo {
			continue
		}
		r := wire.NewReader(f[1:])
		id, tmpl := r.ReadInt32(), r.ReadInt32()
		r.ReadInt32() // attackable
		x, y, z, h := r.ReadInt32(), r.ReadInt32(), r.ReadInt32(), r.ReadInt32()
		if tmpl != regularTreeNPCID+1000000 {
			t.Fatalf("NpcInfo template = %d, want Christmas Tree", tmpl-1000000)
		}
		if int(x) != px || int(y) != py || int(z) != pz || int(h) != heading {
			t.Fatalf("tree at (%d,%d,%d) heading %d, want the player's (%d,%d,%d) heading %d", x, y, z, h, px, py, pz, heading)
		}
		treeID = id
	}
	if treeID == 0 {
		t.Fatalf("frames %x show no NpcInfo", testsupport.FrameOpcodes(frames))
	}
	if _, ok := srv.State.Object(treeID); !ok {
		t.Fatal("tree not in the world")
	}

	srv.Advance(t, regularTreeDespawn-time.Second)
	drainUntilQuiet(t, c)
	if _, ok := srv.State.Object(treeID); !ok {
		t.Fatal("tree gone before its despawn delay")
	}
	srv.Advance(t, time.Second)
	gone := collectUntilQuiet(t, c)
	if _, ok := srv.State.Object(treeID); ok {
		t.Fatal("tree still in the world after its despawn delay")
	}
	deleted := false
	for _, f := range gone {
		if f[0] == serverpackets.OpcodeDeleteObject && int32FromFrame(f, 1) == treeID {
			deleted = true
		}
	}
	if !deleted {
		t.Fatalf("frames %x hold no DeleteObject of the tree", testsupport.FrameOpcodes(gone))
	}
}
