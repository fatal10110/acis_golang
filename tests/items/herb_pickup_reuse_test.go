package items

import (
	"testing"
	"time"

	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
)

// pickupSeededHerb picks up the herb lying at the spawn point and returns
// how many casts of its skill and how many reuse rejections it produced.
func pickupSeededHerb(t *testing.T, srv *gameservertest.Server, objID, groundID int32) (casts, reuse int) {
	t.Helper()
	c := srv.Client
	c.Send(encodeAction(groundID, spawnX, spawnY, spawnZ, false))
	for _, f := range collectUntilQuiet(t, c) {
		switch {
		case f[0] == serverpackets.OpcodeMagicSkillUse:
			assertMagicSkillUseSelf(t, f, objID, 2278, 1, 0, 0)
			casts++
		case f[0] == serverpackets.OpcodeSystemMessage && systemMessageID(t, f) == serverpackets.SystemMessageS1PreparedForReuse:
			assertSystemMessageSkill(t, f, serverpackets.SystemMessageS1PreparedForReuse, 2278, 1)
			reuse++
		}
	}
	if len(srv.GroundItems.Snapshots(nil)) != 0 {
		t.Fatalf("herb still on the ground after pickup")
	}
	return casts, reuse
}

// reseedHerb drops a fresh Herb of Life at the spawn point after advancing
// the clock by wait, and returns its object id.
func reseedHerb(t *testing.T, srv *gameservertest.Server, objID int32, wait time.Duration) int32 {
	t.Helper()
	srv.Advance(t, wait)
	srv.SeedGroundItem(t, objID, 8600, 1, spawnX, spawnY, spawnZ)
	drainUntilQuiet(t, srv.Client)
	return soleGroundObjectID(t, srv)
}

// The pickup's brief paralysis follows the herb's use: a herb picked up
// after its skill's reuse ran out casts again instead of reporting the
// skill as still being prepared.
func TestHerbPickupCastsAgainAfterReuseExpires(t *testing.T) {
	t.Parallel()
	srv, objID, groundID := bootHerbField(t)

	if casts, reuse := pickupSeededHerb(t, srv, objID, groundID); casts != 1 || reuse != 0 {
		t.Fatalf("first herb pickup: casts=%d reuse=%d, want 1/0", casts, reuse)
	}
	groundID = reseedHerb(t, srv, objID, 3*time.Second)
	if casts, reuse := pickupSeededHerb(t, srv, objID, groundID); casts != 1 || reuse != 0 {
		t.Fatalf("herb pickup after reuse expired: casts=%d reuse=%d, want 1/0", casts, reuse)
	}
}

// A herb picked up while its skill is still cooling is consumed without
// casting and reports the skill as being prepared for reuse.
func TestHerbPickupRejectsWhileReuseActive(t *testing.T) {
	t.Parallel()
	srv, objID, groundID := bootHerbField(t)

	if casts, reuse := pickupSeededHerb(t, srv, objID, groundID); casts != 1 || reuse != 0 {
		t.Fatalf("first herb pickup: casts=%d reuse=%d, want 1/0", casts, reuse)
	}
	groundID = reseedHerb(t, srv, objID, time.Second)
	if casts, reuse := pickupSeededHerb(t, srv, objID, groundID); casts != 0 || reuse != 1 {
		t.Fatalf("herb pickup during reuse: casts=%d reuse=%d, want 0/1", casts, reuse)
	}
}
