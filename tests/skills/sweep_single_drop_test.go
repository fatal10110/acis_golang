package skills

import (
	"testing"
	"time"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/npc"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/item"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/network"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
)

// Reference: Sweep.java hands each swept item to Player.addEarnedItem(itemId,
// count, true), which names the whole count as EARNED_S2_S1_S
// (Player.java:1886-1890) and then calls ItemContainer.addItem(itemId,
// count): one instance per unit of a non-stackable, stopping after the
// first when MultipleItemDrop is off (ItemContainer.java:224-253).

// sweepWeaponsTemplate is a monster whose spoil always yields three of the
// catalog's non-stackable weapon.
func sweepWeaponsTemplate() *npc.Template {
	tmpl := spoiledMonsterTemplate()
	tmpl.Drops = []item.DropCategory{{
		Kind: item.DropSpoil, Chance: 100,
		Drops: []item.Drop{{ItemID: 30, Min: 3, Max: 3, Chance: 100}},
	}}
	return tmpl
}

// sweepThreeWeapons spoils, kills and sweeps a sweepWeaponsTemplate monster
// with the player alone. The sweep must name all three weapons in one
// earned line.
func sweepThreeWeapons(t *testing.T, opts ...gameservertest.Option) (*gameservertest.Server, int32) {
	t.Helper()
	srv := gameservertest.Boot(t, append([]gameservertest.Option{
		gameservertest.WithCharacter("Sweeper", 5, 0),
		gameservertest.WithWantChars(1),
		gameservertest.WithSkills(skillPersistence(t, []modelskill.Definition{sweeperSkill()})),
	}, opts...)...)
	c, objID := srv.Client, srv.SoleObjectID(t)
	seedKnownSkill(t, srv, objID, sweeperSkillID, 1)
	startInWorld(t, c)

	mob := srv.SpawnHostileNPCTemplateAt(t, sweepWeaponsTemplate(), location.Location{X: hostileX, Y: hostileY, Z: hostileZ})
	drainUntilQuiet(t, c)
	targetHostile(t, c, mob.ObjectID())
	drainUntilQuiet(t, c)
	if !mob.SpoilPool().Mark(objID) {
		t.Fatal("fresh monster already spoiled")
	}
	obj, _ := srv.State.Player(objID)
	killer, ok := network.OnlineCharacter(obj)
	if !ok {
		t.Fatalf("player %T is not an online character", obj)
	}
	if !mob.TakeDamage(1_000_000, killer) {
		t.Fatal("lethal hit did not kill the monster")
	}
	mob.SetCorpseDeadline(time.Now().Add(time.Minute))
	drainUntilQuiet(t, c)

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
	if len(messages) != 1 {
		t.Fatalf("system messages after the launch = %d, want the earned line", len(messages))
	}
	assertSystemMessageParams(t, messages[0], serverpackets.SystemMessageEarnedS2S1S,
		systemMessageParam{serverpackets.SystemMessageParamItemName, 30},
		systemMessageParam{serverpackets.SystemMessageParamItemNumber, 3})
	return srv, objID
}

// TestSweepSplitsNonStackableUnderMultipleItemDrop: with the shipped
// MultipleItemDrop three swept weapons arrive as three instances.
func TestSweepSplitsNonStackableUnderMultipleItemDrop(t *testing.T) {
	t.Parallel()
	srv, objID := sweepThreeWeapons(t)
	if got := len(srv.PlayerInventory(t, objID).ItemsByTemplateID(30)); got != 3 {
		t.Fatalf("weapon instances = %d, want 3", got)
	}
}

// TestSweepKeepsOneInstanceWithoutMultipleItemDrop: with MultipleItemDrop
// off the same sweep adds one weapon, still named for all three.
func TestSweepKeepsOneInstanceWithoutMultipleItemDrop(t *testing.T) {
	t.Parallel()
	srv, objID := sweepThreeWeapons(t, gameservertest.WithSingleItemDrop())
	held := srv.PlayerInventory(t, objID).ItemsByTemplateID(30)
	if len(held) != 1 || held[0].CountValue() != 1 {
		t.Fatalf("weapon instances = %d, want one count-1 instance", len(held))
	}
}
