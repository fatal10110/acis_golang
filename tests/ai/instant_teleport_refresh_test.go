package ai

import (
	"encoding/binary"
	"slices"
	"testing"

	"github.com/fatal10110/acis_golang/internal/commons/wire"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/npc"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/clientpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/script"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
	"github.com/fatal10110/acis_golang/internal/testsupport"
)

// enterOnlooker brings a second character into the world where the first
// one stands and returns its client, both clients drained.
func enterOnlooker(t *testing.T, srv *gameservertest.Server) *testsupport.ScriptedClient {
	t.Helper()
	srv.SeedCharacterFor(t, "onlooker", "Onlooker", 20, 0)
	c := srv.DialClient(t, "onlooker", 1)
	c.Send(encodeGameStart())
	for i := 0; ; i++ {
		if i == 100 {
			t.Fatal("no CharSelected for Onlooker within 100 frames")
		}
		if c.Read()[0] == serverpackets.OpcodeCharSelected {
			break
		}
	}
	c.Send(wire.NewPacketWriter(clientpackets.OpcodeEnterWorld).Bytes())
	srv.ReadQueued(t, c)
	srv.ReadQueued(t, srv.Client)
	return c
}

// opcodesAmong returns, in order, the opcodes of frames that are among
// keep.
func opcodesAmong(frames [][]byte, keep ...byte) []byte {
	var out []byte
	for _, f := range frames {
		if len(f) > 0 && slices.Contains(keep, f[0]) {
			out = append(out, f[0])
		}
	}
	return out
}

// indexOf returns the index of the first frame with opcode whose leading
// object id is id, or -1.
func indexOf(frames [][]byte, opcode byte, id int32) int {
	return slices.IndexFunc(frames, func(f []byte) bool {
		return len(f) >= 5 && f[0] == opcode && int32(binary.LittleEndian.Uint32(f[1:5])) == id
	})
}

// TestPlayerInstantTeleportRefreshesItsKnownList (Creature.instantTeleportTo,
// WorldObject.refreshKnownlist, Player.addKnownObject): a character that
// jumps within its region sees everything around it afresh, and everyone
// around sees it afresh. After the fast TeleportToLocation the jumper gets
// the known monster's NpcInfo and the onlooker's CharInfo once each; the
// onlooker gets the jumper's CharInfo and RelationChanged as it learns of
// the jumper, then the jumper's RelationChanged again as the jumper learns
// of it.
func TestPlayerInstantTeleportRefreshesItsKnownList(t *testing.T) {
	t.Parallel()
	w := bootWarriors(t, warriorTemplate(warriorID, nil))
	monster := w.spawn(t, warriorID, 150, 0)
	onlooker := enterOnlooker(t, w.srv)

	frames := w.instantTeleport(t, location.Location{X: w.at.X + 100, Y: w.at.Y, Z: w.at.Z})
	jumped := indexOf(frames, serverpackets.OpcodeTeleportToLocation, w.player.ObjectID())
	if jumped < 0 {
		t.Fatal("the jumper got no TeleportToLocation")
	}
	if got := len(framesOf(frames, serverpackets.OpcodeNPCInfo, monster.ObjectID())); got != 1 {
		t.Fatalf("jumper NpcInfo frames for the known monster = %d, want 1", got)
	}
	if at := indexOf(frames, serverpackets.OpcodeNPCInfo, monster.ObjectID()); at < jumped {
		t.Fatalf("the monster's NpcInfo (frame %d) came before the jump (frame %d)", at, jumped)
	}
	if got := opcodesAmong(frames[jumped:], serverpackets.OpcodeCharInfo); len(got) != 1 {
		t.Fatalf("jumper CharInfo frames after the jump = %d, want 1 for the onlooker", len(got))
	}

	seen := w.srv.ReadQueued(t, onlooker)
	want := []byte{
		serverpackets.OpcodeTeleportToLocation,
		serverpackets.OpcodeCharInfo, serverpackets.OpcodeRelationChanged,
		serverpackets.OpcodeRelationChanged,
	}
	got := opcodesAmong(seen, serverpackets.OpcodeTeleportToLocation, serverpackets.OpcodeCharInfo, serverpackets.OpcodeRelationChanged)
	if !slices.Equal(got, want) {
		t.Fatalf("onlooker frames = % x, want % x", got, want)
	}
}

// TestNPCInstantTeleportRefreshesItsObservers (Creature.instantTeleportTo,
// WorldObject.refreshKnownlist): a monster that jumps within its region is
// shown afresh to the character around it: the fast TeleportToLocation,
// then its NpcInfo once.
func TestNPCInstantTeleportRefreshesItsObservers(t *testing.T) {
	t.Parallel()
	w := bootWarriors(t, warriorTemplate(warriorID, nil))
	h := w.spawn(t, warriorID, 150, 0)

	to := location.Location{X: w.at.X + 300, Y: w.at.Y, Z: w.at.Z}
	done := make(chan struct{})
	h.Queue().Post(func() {
		defer close(done)
		script.NPCOf(h).InstantTeleportTo(script.Loc{X: to.X, Y: to.Y, Z: to.Z})
	})
	<-done
	w.srv.Settle(t)
	frames := w.srv.ReadQueued(t, w.srv.Client)
	jumped := indexOf(frames, serverpackets.OpcodeTeleportToLocation, h.ObjectID())
	shown := indexOf(frames, serverpackets.OpcodeNPCInfo, h.ObjectID())
	if jumped < 0 || shown < jumped || len(framesOf(frames, serverpackets.OpcodeNPCInfo, h.ObjectID())) != 1 {
		t.Fatalf("TeleportToLocation at %d, NpcInfo at %d (%d of them); want the jump, then one NpcInfo",
			jumped, shown, len(framesOf(frames, serverpackets.OpcodeNPCInfo, h.ObjectID())))
	}
}

// TestFolkInstantTeleportRefreshesItsObservers: a civilian NPC's jump, the
// one that cannot move as well as the one that can, is shown afresh to the
// character around it as a monster's is.
func TestFolkInstantTeleportRefreshesItsObservers(t *testing.T) {
	t.Parallel()
	srv, at := bootFolkDesires(t, script.Catalog{})
	for i, tmpl := range []*npc.Template{gameservertest.FolkTemplate("Folk", int(folkOfferingID)), folkFighterTemplate(folkPatrolID)} {
		f := srv.SpawnFolkNPCAt(t, tmpl, location.Location{X: at.X + 40, Y: at.Y + 100*i, Z: at.Z})
		srv.Settle(t)
		srv.ReadQueued(t, srv.Client)

		done := make(chan struct{})
		f.Queue().Post(func() {
			defer close(done)
			script.NPCOf(f).InstantTeleportTo(script.Loc{X: at.X + 300, Y: at.Y + 100*i, Z: at.Z})
		})
		<-done
		srv.Settle(t)
		frames := srv.ReadQueued(t, srv.Client)
		jumped := indexOf(frames, serverpackets.OpcodeTeleportToLocation, f.ObjectID())
		shown := indexOf(frames, serverpackets.OpcodeNPCInfo, f.ObjectID())
		if jumped < 0 || shown < jumped || len(framesOf(frames, serverpackets.OpcodeNPCInfo, f.ObjectID())) != 1 {
			t.Fatalf("folk %d: TeleportToLocation at %d, NpcInfo at %d; want the jump, then one NpcInfo", tmpl.ID, jumped, shown)
		}
	}
}
