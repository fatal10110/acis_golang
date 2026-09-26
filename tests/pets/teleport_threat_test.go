package pets

import (
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	"github.com/fatal10110/acis_golang/internal/gameserver/network"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/clientpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
)

// TestOwnerTeleportDropsSummonThreatInNearbyHostiles pins the summon's own
// teleport at the end of its owner's: the Appearing that completes the
// owner's jump moves the pet after it, and every hostile around the pet's old
// position forgets it, even one that still sees it afterwards. The owner's
// jump alone leaves the pet's entry in place.
func TestOwnerTeleportDropsSummonThreatInNearbyHostiles(t *testing.T) {
	t.Parallel()
	h := bootOwnerWithCollar(t)
	pet, _ := h.spawnWolf(t)
	monster := h.srv.SpawnHostileNPCTemplateAt(t, dropMonsterTemplate(), location.Location{X: hostileX, Y: hostileY, Z: hostileZ})
	drainUntilQuiet(t, h.client)

	if monster.TakeDamage(10, pet) {
		t.Fatal("pet's hit killed the monster")
	}
	obj, ok := h.srv.State.Player(h.ownerID)
	if !ok {
		t.Fatal("owner missing from world state")
	}
	owner, ok := network.OnlineCharacter(obj)
	if !ok {
		t.Fatalf("world.Player(%d) = %T is not an online character", h.ownerID, obj)
	}

	// A bystander keeps the region awake while the owner is off the grid;
	// alone, the owner's departure would put the monster back to peace.
	h.srv.SeedCharacterFor(t, "bystander", "Bystander", 1, 0)
	startInWorld(t, h.srv.DialClient(t, "bystander", 1))
	drainUntilQuiet(t, h.client)

	x, y, z := h.srv.PlayerPosition(t, h.ownerID)
	owner.TeleportTo(x+300, y, z, 0)
	readUntilOpcode(t, h.client, serverpackets.OpcodeTeleportToLocation, "owner TeleportToLocation")
	if _, ok := monster.AI().Threats().Get(pet); !ok {
		t.Fatal("pet's threat entry dropped by the owner's jump, want it kept until the pet's own")
	}

	h.client.Send(encodeSingleOpcode(clientpackets.OpcodeAppearing))
	readUntilOpcode(t, h.client, serverpackets.OpcodeTeleportToLocation, "pet TeleportToLocation")
	if !monster.Knows(pet) {
		t.Fatal("pet out of the monster's sight after the teleport; the case needs it still known")
	}
	if got, ok := monster.AI().Threats().Get(pet); ok {
		t.Fatalf("pet's threat entry survived its teleport: %+v", got)
	}
}
