package pets

import (
	"testing"

	"github.com/fatal10110/acis_golang/internal/commons/wire"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
)

// summonRelationView is one RelationChanged a player was sent.
type summonRelationView struct {
	objectID, relation, autoAttackable, karma, pvpFlag int32
}

func readSummonRelationView(t *testing.T, frame []byte) summonRelationView {
	t.Helper()
	if frame[0] != serverpackets.OpcodeRelationChanged {
		t.Fatalf("opcode %#x, want RelationChanged (%#x)", frame[0], serverpackets.OpcodeRelationChanged)
	}
	r := wire.NewReader(frame[1:])
	return summonRelationView{r.ReadInt32(), r.ReadInt32(), r.ReadInt32(), r.ReadInt32(), r.ReadInt32()}
}

// relationsAfterOwnerCharInfo returns the two RelationChanged frames that
// must directly follow ownerID's CharInfo in frames.
func relationsAfterOwnerCharInfo(t *testing.T, frames [][]byte, ownerID int32) (owner, pet summonRelationView) {
	t.Helper()
	for i, f := range frames {
		if f[0] != serverpackets.OpcodeCharInfo {
			continue
		}
		r := wire.NewReader(f[1:])
		for range 4 {
			r.ReadInt32()
		}
		if r.ReadInt32() != ownerID {
			continue
		}
		if i+2 >= len(frames) {
			t.Fatalf("CharInfo of %d is not followed by two frames", ownerID)
		}
		return readSummonRelationView(t, frames[i+1]), readSummonRelationView(t, frames[i+2])
	}
	t.Fatalf("no CharInfo of %d among %d frames", ownerID, len(frames))
	return summonRelationView{}, summonRelationView{}
}

// assertOwnerThenPetRelation checks Player.broadcastCharInfo and
// Player.sendInfo (Player.java:2254-2266, 6858-6863): the owner's CharInfo
// is followed by its RelationChanged, then its summon's, both carrying the
// owner's relation, auto-attackable flag, karma and PvP flag.
func assertOwnerThenPetRelation(t *testing.T, frames [][]byte, ownerID, petID int32, karma int32) {
	t.Helper()
	owner, pet := relationsAfterOwnerCharInfo(t, frames, ownerID)
	want := summonRelationView{ownerID, serverpackets.RelationHasKarma, 1, karma, 0}
	if owner != want {
		t.Fatalf("owner's RelationChanged = %+v, want %+v", owner, want)
	}
	want.objectID = petID
	if pet != want {
		t.Fatalf("summon's RelationChanged = %+v, want %+v", pet, want)
	}
}

// TestSummonRelationFollowsOwnerCharInfo has a PK owner with a wolf out:
// another player learns the owner's relation and then the wolf's right
// after the owner's CharInfo, both when the owner's CharInfo is refreshed
// in front of it and when it discovers the owner by entering the world.
func TestSummonRelationFollowsOwnerCharInfo(t *testing.T) {
	t.Parallel()
	const karma = 500
	t.Run("charinfo refresh", func(t *testing.T) {
		t.Parallel()
		h := bootKarmaOwnerWithCollar(t, 40, karma)
		pet, _ := h.spawnWolf(t)
		other := h.joinSecondPlayer(t, "Watcher")
		drainUntilQuiet(t, h.client)
		drainUntilQuiet(t, other.client)

		h.client.Send(encodeRequestChangeMoveType(false))
		assertOwnerThenPetRelation(t, drainFrames(t, other.client), h.ownerID, pet.ObjectID(), karma)
	})
	t.Run("discovery", func(t *testing.T) {
		t.Parallel()
		h := bootKarmaOwnerWithCollar(t, 40, karma)
		pet, _ := h.spawnWolf(t)
		drainUntilQuiet(t, h.client)

		h.srv.SeedCharacterFor(t, "player2", "Watcher", 1, 0)
		c := h.srv.DialClient(t, "player2", 1)
		c.Send(encodeRequestGameStart(0))
		for frame := c.Read(); frame[0] != serverpackets.OpcodeCharSelected; frame = c.Read() {
		}
		c.Send(encodeEnterWorld())
		assertOwnerThenPetRelation(t, drainFrames(t, c), h.ownerID, pet.ObjectID(), karma)
	})
}
