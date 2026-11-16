package skills

import (
	"testing"
	"time"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/npc"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/item"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
)

// Reference: DropCategory.calculateDrop (DropCategory.java:49-90) merges the
// spoil roll into a HashMap<>(1); Monster.doItemDrop (Monster.java:486-491)
// puts its entries into the SpoilState HashMap in that map's iteration
// order, and Sweep.java:28-47 earns the SpoilState entries in its own
// iteration order. Potion 20 and Lesser Healing Potion 1060 share bucket 4
// of the 16-bucket SpoilState and bucket 0 of the two-bucket roll map, where
// the newer key heads the bucket: 1060, rolled second, is earned first.

// sameBucketSpoilTemplate is a monster whose spoil category always yields
// three potions 20 and two potions 1060, in that list order.
func sameBucketSpoilTemplate() *npc.Template {
	tmpl := spoiledMonsterTemplate()
	tmpl.Drops = []item.DropCategory{{
		Kind: item.DropSpoil, Chance: 100,
		Drops: []item.Drop{
			{ItemID: 20, Min: 3, Max: 3, Chance: 100},
			{ItemID: 1060, Min: 2, Max: 2, Chance: 100},
		},
	}}
	return tmpl
}

// TestSweepEarnsSameBucketSpoilNewestRollFirst sweeps a corpse whose spoil
// holds two items of one bucket and reads the earned lines in roll-map
// order, the later-listed item first.
func TestSweepEarnsSameBucketSpoilNewestRollFirst(t *testing.T) {
	t.Parallel()
	srv, objID, mob := sweepSceneOf(t, sameBucketSpoilTemplate(), func(_ *gameservertest.Server, sweeperID int32) int32 { return sweeperID })
	c := srv.Client

	c.Send(encodeRequestMagicSkillUse(sweeperSkillID, false, false))
	readCastStartFrames(t, c, objID, sweeperSkillID, 1, 500, 500, mob.ObjectID())
	first := c.ReadWithTimeout(2 * time.Second)
	if first == nil {
		t.Fatal("nothing followed the sweep's launch")
	}
	var messages [][]byte
	for _, f := range append([][]byte{first}, collectUntilQuiet(t, c)...) {
		if f[0] == serverpackets.OpcodeSystemMessage {
			messages = append(messages, f)
		}
	}
	if len(messages) != 2 {
		t.Fatalf("system messages after the launch = %d, want the two earned lines", len(messages))
	}
	assertSystemMessageParams(t, messages[0], serverpackets.SystemMessageEarnedS2S1S,
		systemMessageParam{serverpackets.SystemMessageParamItemName, 1060},
		systemMessageParam{serverpackets.SystemMessageParamItemNumber, 2})
	assertSystemMessageParams(t, messages[1], serverpackets.SystemMessageEarnedS2S1S,
		systemMessageParam{serverpackets.SystemMessageParamItemName, 20},
		systemMessageParam{serverpackets.SystemMessageParamItemNumber, 3})

	inv := srv.PlayerInventory(t, objID)
	if got := inv.ItemCount(1060, -1, true); got != 2 {
		t.Fatalf("carried 1060 = %d, want 2", got)
	}
	if got := inv.ItemCount(20, -1, true); got != 3 {
		t.Fatalf("carried 20 = %d, want 3", got)
	}
}
