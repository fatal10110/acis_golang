package items

import (
	"testing"
	"time"

	"github.com/fatal10110/acis_golang/internal/commons/wire"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
)

// TestUseItemSkillOutOfRangeWalksThenCasts pins an item's CAST intention
// taking the same approach as a skill-bar cast (ItemSkills.java:80 into
// PlayerAI.thinkCast, PlayerAI.java:259-271): forced onto a monster beyond
// the skill's 400 cast range, the key walks to it with MoveToPawn, still
// unspent, and its arrival casts it and spends the key.
func TestUseItemSkillOutOfRangeWalksThenCasts(t *testing.T) {
	t.Parallel()
	srv := gameservertest.Boot(t,
		gameservertest.WithSkills(ctrlKeySkills(t)),
		gameservertest.WithCharacter("Newbie", 5, 0),
		gameservertest.WithWantChars(1))
	c, objID := srv.Client, srv.SoleObjectID(t)
	key := srv.GiveItem(t, objID, gameservertest.UnlockableKeyID, 3)
	startInWorld(t, c)
	x, y, z := srv.PlayerPosition(t, objID)
	at := location.Location{X: x + 900, Y: y, Z: z}
	hostile := srv.SpawnHostileNPCAt(t, at)
	drainUntilQuiet(t, c)
	c.Send(encodeAction(hostile.ObjectID(), int32(at.X), int32(at.Y), int32(at.Z), false))
	drainUntilQuiet(t, c)

	c.Send(encodeUseItem(key, true))
	walk := c.Read()
	assertFrameOpcode(t, walk, serverpackets.OpcodeMoveToPawn, "item cast approach MoveToPawn")
	r := wire.NewReader(walk[1:])
	if obj, target, distance := r.ReadInt32(), r.ReadInt32(), r.ReadInt32(); obj != objID || target != hostile.ObjectID() || distance != 400 {
		t.Fatalf("MoveToPawn = object %d target %d distance %d, want %d/%d/400", obj, target, distance, objID, hostile.ObjectID())
	}
	assertItemCount(t, srv, objID, key, 3)

	var cast []byte
	for passed := time.Duration(0); cast == nil && passed < 20*time.Second; passed += 100 * time.Millisecond {
		srv.Advance(t, 100*time.Millisecond)
		for frame := c.ReadWithTimeout(20 * time.Millisecond); frame != nil; frame = c.ReadWithTimeout(20 * time.Millisecond) {
			if frame[0] == serverpackets.OpcodeMagicSkillUse {
				cast = frame
				break
			}
		}
	}
	if cast == nil {
		t.Fatal("no MagicSkillUse once the item cast approach arrived")
	}
	if caster, target, skillID, _, _, _ := decodeMagicSkillUse(cast); caster != objID || target != hostile.ObjectID() || skillID != 2236 {
		t.Fatalf("MagicSkillUse = caster %d target %d skill %d, want %d/%d/2236", caster, target, skillID, objID, hostile.ObjectID())
	}
	drainUntilQuiet(t, c)
	assertItemCount(t, srv, objID, key, 2)
}
