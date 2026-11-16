package character

import (
	"testing"

	"github.com/fatal10110/acis_golang/internal/commons/wire"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
)

// TestSelectStaticObjectShowsSelectorOnly pins Player.setTarget on a static
// object (Player.java:2468-2472): the selector gets MyTargetSelected with a
// neutral color, then StaticObjectInfo; no ValidateLocation, no
// StatusUpdate, and a player watching the selector receives nothing.
func TestSelectStaticObjectShowsSelectorOnly(t *testing.T) {
	t.Parallel()
	srv := bootBodied(t)
	c := srv.Client
	srv.SeedCharacterFor(t, "player2", "Second", 1, 0)
	watcher := srv.DialClient(t, "player2", 1)
	enterWorld(t, watcher)
	drainQuiet(t, watcher)
	drainQuiet(t, c)
	sign := spawnTownMap(t, srv, c, 0)
	drainQuiet(t, watcher)

	frames := clickStatic(t, c, sign, false)
	if len(frames) != 2 || frames[0][0] != serverpackets.OpcodeMyTargetSelected || frames[1][0] != serverpackets.OpcodeStaticObjectInfo {
		t.Fatalf("select frames = % x, want MyTargetSelected then StaticObjectInfo", opcodes(frames))
	}
	r := wire.NewReader(frames[0][1:])
	if id, color := r.ReadInt32(), r.ReadUint16(); id != sign.ObjectID() || color != 0 {
		t.Fatalf("MyTargetSelected = (%d, %d), want (%d, 0)", id, color, sign.ObjectID())
	}
	r = wire.NewReader(frames[1][1:])
	if staticID, id := r.ReadInt32(), r.ReadInt32(); int(staticID) != sign.StaticObjectID() || id != sign.ObjectID() {
		t.Fatalf("StaticObjectInfo = (%d, %d), want (%d, %d)", staticID, id, sign.StaticObjectID(), sign.ObjectID())
	}
	if got := syncFrames(t, watcher); len(got) != 0 {
		t.Fatalf("watcher frames = % x, want none", opcodes(got))
	}
}
