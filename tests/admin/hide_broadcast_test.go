package admin

import (
	"testing"

	"github.com/fatal10110/acis_golang/internal/commons/wire"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/zone"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/clientpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
	"github.com/fatal10110/acis_golang/internal/testsupport"
)

// actionWalkRun is RequestActionUse's walk/run toggle.
const actionWalkRun = 1

func encodeActionUse(actionID int32) []byte {
	w := wire.NewPacketWriter(clientpackets.OpcodeRequestActionUse)
	w.WriteInt32(actionID)
	w.WriteInt32(0)
	w.WriteUint8(0)
	return w.Bytes()
}

// requireOneCharInfo requires frames to hold exactly one CharInfo of
// objectID, with invisible byte hidden.
func requireOneCharInfo(t *testing.T, who string, frames [][]byte, objectID int32, hidden byte) {
	t.Helper()
	shown := 0
	for _, f := range frames {
		if f[0] != serverpackets.OpcodeCharInfo {
			continue
		}
		id, got := charInfoHidden(t, f)
		if id != objectID {
			continue
		}
		shown++
		if got != hidden {
			t.Fatalf("%s: CharInfo invisible byte = %d, want %d", who, got, hidden)
		}
	}
	if shown != 1 {
		t.Fatalf("%s: frames %x hold %d CharInfo of %d, want 1", who, testsupport.FrameOpcodes(frames), shown, objectID)
	}
}

// TestHiddenGMInfoRefreshStaysInvisible pins the invisible byte of every
// CharInfo an invisible player broadcasts while on the grid (CharInfo.java
// writes isVisible per viewer), not just the one it is rediscovered with:
// a hidden GM toggling walk/run (Player.broadcastUserInfo) is still drawn
// invisible to a player and visible to a GM.
func TestHiddenGMInfoRefreshStaysInvisible(t *testing.T) {
	t.Parallel()
	srv, gmID := bootAdmin(t, adminLevel)
	gm := srv.Client
	enterWorld(t, gm)
	watcher, _ := addPlayer(t, srv, "player2", "Watcher", userLevel)
	otherGM, _ := addPlayer(t, srv, "player3", "OtherGM", adminLevel)
	drain(t, gm)
	drain(t, watcher)
	drain(t, otherGM)

	exchange(t, gm, encodeBuildCmd("hide"))
	settle(t, watcher)
	settle(t, otherGM)

	exchange(t, gm, encodeActionUse(actionWalkRun))
	requireOneCharInfo(t, "Watcher", settle(t, watcher), gmID, 1)
	requireOneCharInfo(t, "OtherGM", settle(t, otherGM), gmID, 0)

	exchange(t, gm, encodeBuildCmd("hide"))
	settle(t, watcher)
	settle(t, otherGM)
	exchange(t, gm, encodeActionUse(actionWalkRun))
	requireOneCharInfo(t, "Watcher once shown", settle(t, watcher), gmID, 0)
	requireOneCharInfo(t, "OtherGM once shown", settle(t, otherGM), gmID, 0)
}

// TestAdminHideInWaterRestartsBreath pins what //hide does to a submerged
// GM. decayMe clears the region (Creature.setRegion(null) →
// WorldRegion.removeFromZones → WaterZone.onExit), and Player.revalidateZone
// then stops the breath countdown (WaterTaskManager.remove, an empty gauge).
// spawnMe sets the region back and re-enters the water zone, which starts a
// fresh countdown (WaterTaskManager.add, a full gauge).
func TestAdminHideInWaterRestartsBreath(t *testing.T) {
	t.Parallel()
	form, err := zone.NewCuboid(spawnX-1_000, spawnX+1_000, spawnY-1_000, spawnY+1_000, -1_000, 150)
	if err != nil {
		t.Fatal(err)
	}
	zones := zone.NewIndex()
	zones.Add(zone.NewWater(1, form))
	srv, _ := bootAdmin(t, adminLevel, gameservertest.WithZones(zones), gameservertest.WithWater(nil))
	gm := srv.Client
	enterWorld(t, gm)

	var gauges []int32
	for _, f := range exchange(t, gm, encodeBuildCmd("hide")) {
		if f[0] != serverpackets.OpcodeSetupGauge {
			continue
		}
		r := wire.NewReader(f[1:])
		r.ReadInt32() // color
		gauges = append(gauges, r.ReadInt32())
	}
	if len(gauges) != 2 || gauges[0] != 0 || gauges[1] <= 0 {
		t.Fatalf("//hide in water sent breath gauges %v, want an empty one then a full one", gauges)
	}
}
