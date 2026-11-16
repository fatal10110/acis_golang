package items

import (
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/summon"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
)

// herbReuseKey is the reuse key of the Herb of Life's carried skill 2278/1.
const herbReuseKey = 2278*256 + 1

// spawnHerbServitor gives objID an active servitor standing at at.
func spawnHerbServitor(t *testing.T, srv *gameservertest.Server, objID int32, at location.Location) *summon.Actor {
	t.Helper()
	servitor, err := summon.NewServitor(summon.ServitorConfig{
		ObjectID: srv.NewObjectID(),
		Level:    44,
		Stats:    summon.CombatStats{MaxHP: 500, MaxMP: 200},
	})
	if err != nil {
		t.Fatal(err)
	}
	servitor.SetQueue(srv.PlayerQueue(t, objID))
	srv.State.AddSummon(objID, servitor)
	srv.State.Spawn(servitor, at.X, at.Y, at.Z, 0)
	return servitor
}

// pickUpHerb picks the ground herb up and returns what follows GetItem.
func pickUpHerb(t *testing.T, srv *gameservertest.Server, groundID int32) [][]byte {
	t.Helper()
	c := srv.Client
	drainUntilQuiet(t, c)
	c.Send(encodeAction(groundID, spawnX, spawnY, spawnZ, false))
	assertFrameOpcode(t, c.Read(), serverpackets.OpcodeActionFailed, "pickup pending-action release")
	assertFrameOpcode(t, c.Read(), serverpackets.OpcodeGetItem, "GetItem")
	return collectUntilQuiet(t, c)
}

// onlyFrameIndex returns the index of the single frame match accepts.
func onlyFrameIndex(t *testing.T, frames [][]byte, what string, match func([]byte) bool) int {
	t.Helper()
	at := -1
	for i, f := range frames {
		if !match(f) {
			continue
		}
		if at >= 0 {
			t.Fatalf("%s sent twice, at %d and %d", what, at, i)
		}
		at = i
	}
	if at < 0 {
		t.Fatalf("%s missing from %d frames", what, len(frames))
	}
	return at
}

// TestHerbMirrorFollowsOwnersWholeCast pins the herb mirror's order (issue
// #4819): the servitor's instant cast runs only after the owner's whole
// cast — owner's MagicSkillUse, USE_S1, then the owner's effects — and the
// servitor takes the skill's reuse itself.
func TestHerbMirrorFollowsOwnersWholeCast(t *testing.T) {
	t.Parallel()
	srv, objID, groundID := bootHerbField(t)
	servitor := spawnHerbServitor(t, srv, objID, location.Location{X: spawnX, Y: spawnY, Z: spawnZ})

	frames := pickUpHerb(t, srv, groundID)

	castBy := func(id int32) func([]byte) bool {
		return func(f []byte) bool {
			return f[0] == serverpackets.OpcodeMagicSkillUse && selfCasts(t, [][]byte{f}, id, 2278) == 1
		}
	}
	ownerCast := onlyFrameIndex(t, frames, "owner MagicSkillUse", castBy(objID))
	useS1 := onlyFrameIndex(t, frames, "USE_S1", func(f []byte) bool {
		return f[0] == serverpackets.OpcodeSystemMessage && systemMessageID(t, f) == serverpackets.SystemMessageUseS1
	})
	servitorCast := onlyFrameIndex(t, frames, "servitor MagicSkillUse", castBy(servitor.ObjectID()))
	if ownerCast >= useS1 || useS1 >= servitorCast {
		t.Fatalf("frame order owner cast %d, USE_S1 %d, servitor cast %d: want the owner's cast and USE_S1 before the servitor's cast", ownerCast, useS1, servitorCast)
	}
	if !servitor.SkillDisabled(herbReuseKey) {
		t.Fatal("servitor's copy of the herb skill is not under reuse, want its 2000ms reuse")
	}
}

// TestHerbMirrorCastReachesServitorObservers pins the servitor cast's
// audience (issue #4819): the players that know the servitor, not the
// owner's observers. With the servitor three regions from its owner, a
// player beside it sees only the servitor's cast and the owner only its own.
func TestHerbMirrorCastReachesServitorObservers(t *testing.T) {
	t.Parallel()
	srv, objID, groundID := bootHerbField(t)
	away := location.Location{X: spawnX + 3*2048, Y: spawnY, Z: spawnZ}
	servitor := spawnHerbServitor(t, srv, objID, away)
	srv.SeedCharacterForAt(t, "herbwatch", "Herbwatch", 1, 0, location.Location{X: away.X + 50, Y: away.Y, Z: away.Z})
	watcher := srv.DialClient(t, "herbwatch", 1)
	startInWorld(t, watcher)
	drainUntilQuiet(t, watcher)

	owner := pickUpHerb(t, srv, groundID)
	seen := collectUntilQuiet(t, watcher)

	for _, tc := range []struct {
		who           string
		frames        [][]byte
		owner, mirror int
	}{{"owner", owner, 1, 0}, {"servitor's watcher", seen, 0, 1}} {
		if got := selfCasts(t, tc.frames, objID, 2278); got != tc.owner {
			t.Fatalf("%s saw %d owner herb casts, want %d", tc.who, got, tc.owner)
		}
		if got := selfCasts(t, tc.frames, servitor.ObjectID(), 2278); got != tc.mirror {
			t.Fatalf("%s saw %d servitor herb casts, want %d", tc.who, got, tc.mirror)
		}
	}
}
