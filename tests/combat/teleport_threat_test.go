package combat

import (
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	"github.com/fatal10110/acis_golang/internal/gameserver/network"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/clientpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
)

// TestPlayerTeleportDropsItsThreatInNearbyHostiles pins reward attribution
// across a player teleport that lands still in sight of a monster it hit.
// Leaving its old position makes every hostile around it forget the player,
// so the damage no longer counts when a guard finishes the monster: no drops
// and no spoil. The monster's spell hate for the player is kept.
func TestPlayerTeleportDropsItsThreatInNearbyHostiles(t *testing.T) {
	t.Parallel()
	srv, objID, monster := spawnSpoiledDropMonster(t)
	obj, ok := srv.State.Player(objID)
	if !ok {
		t.Fatal("player missing from world state")
	}
	player, ok := network.OnlineCharacter(obj)
	if !ok {
		t.Fatalf("player %T is not an online character", obj)
	}

	if monster.TakeDamage(int(monster.CurrentHP())/2, player) {
		t.Fatal("player's half-HP hit killed the monster")
	}
	monster.AddHate(player, 50)
	if _, ok := monster.AI().Threats().Get(player); !ok {
		t.Fatal("player's hit left no threat entry")
	}

	// The bystander keeps the region awake while the player is off the grid.
	joinBystander(t, srv)
	player.TeleportTo(hostileX+300, hostileY, hostileZ, 0)
	readUntil(t, srv.Client, serverpackets.OpcodeTeleportToLocation, "TeleportToLocation")
	srv.Client.Send(encodeSingleOpcode(clientpackets.OpcodeAppearing))
	readUntil(t, srv.Client, serverpackets.OpcodeUserInfo, "Appearing UserInfo")
	if !monster.Knows(player) {
		t.Fatal("player out of the monster's sight after the teleport; the case needs it still known")
	}
	if got, ok := monster.AI().Threats().Get(player); ok {
		t.Fatalf("player's threat entry survived the teleport: %+v", got)
	}
	if got := monster.AI().Hates().Hate(player); got != 50 {
		t.Fatalf("monster's spell hate for the player = %v, want 50 kept", got)
	}

	guard := srv.SpawnHostileNPCKindAt(t, "Guard", location.Location{X: hostileX + 90, Y: hostileY, Z: hostileZ})
	if !monster.TakeDamage(1_000_000, guard) {
		t.Fatal("guard's lethal hit did not kill the monster")
	}
	if drops := groundDrops(srv); len(drops) != 0 {
		t.Fatalf("ground drops = %+v, want none once the player's damage was forgotten", drops)
	}
	if monster.SpoilPool().Sweepable() {
		t.Fatal("spoil pool filled from forgotten damage")
	}
}

// TestNPCTeleportDropsItsThreatInNearbyHostiles pins the other side of an NPC
// teleport: a guard that damaged-and-hated the monster drops its threat entry
// for it even though the monster lands in the guard's sight. The guard's
// spell hate for the monster is kept.
func TestNPCTeleportDropsItsThreatInNearbyHostiles(t *testing.T) {
	t.Parallel()
	srv, _, monster := spawnSpoiledDropMonster(t)
	guard := srv.SpawnHostileNPCKindAt(t, "Guard", location.Location{X: hostileX + 90, Y: hostileY, Z: hostileZ})

	guard.AddDamageHate(monster, 10, 10)
	guard.AddHate(monster, 50)
	if _, ok := guard.AI().Threats().Get(monster); !ok {
		t.Fatal("guard recorded no threat for the monster")
	}

	monster.TeleportTo(location.Location{X: hostileX + 50, Y: hostileY, Z: hostileZ})
	if !guard.Knows(monster) {
		t.Fatal("monster out of the guard's sight after the teleport; the case needs it still known")
	}
	if got, ok := guard.AI().Threats().Get(monster); ok {
		t.Fatalf("guard's threat entry for the monster survived the teleport: %+v", got)
	}
	if got := guard.AI().Hates().Hate(monster); got != 50 {
		t.Fatalf("guard's spell hate for the monster = %v, want 50 kept", got)
	}
}
