package combat

import (
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/item"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	"github.com/fatal10110/acis_golang/internal/gameserver/network"
)

// TestNPCTeleportForgetsNearbyAttackerDamage pins reward attribution across an
// NPC teleport that lands next to its attacker. Leaving the grid forgets every
// creature around the old position, so damage dealt before the jump no longer
// counts even though the attacker is still in sight afterwards; damage dealt
// after the jump does.
func TestNPCTeleportForgetsNearbyAttackerDamage(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name          string
		hitBeforeJump bool
		wantDrop      bool
	}{
		{"damage before teleport", true, false},
		{"damage after teleport", false, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
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

			hit := func() {
				if monster.TakeDamage(int(monster.CurrentHP())/2, player) {
					t.Fatal("player's half-HP hit killed the monster")
				}
			}
			if tc.hitBeforeJump {
				hit()
			}
			monster.TeleportTo(location.Location{X: hostileX + 50, Y: hostileY, Z: hostileZ})
			if !monster.Knows(player) {
				t.Fatal("player out of sight after the teleport; the case needs an attacker still known")
			}
			if !tc.hitBeforeJump {
				hit()
			}

			guard := srv.SpawnHostileNPCKindAt(t, "Guard", location.Location{X: hostileX + 90, Y: hostileY, Z: hostileZ})
			if !monster.TakeDamage(1_000_000, guard) {
				t.Fatal("guard's lethal hit did not kill the monster")
			}

			drops := groundDrops(srv)
			if !tc.wantDrop {
				// Only the guard's entry is left: no player earned the kill.
				if len(drops) != 0 {
					t.Fatalf("ground drops = %+v, want none once the player's damage was forgotten", drops)
				}
				if monster.SpoilPool().Sweepable() {
					t.Fatal("spoil pool filled from forgotten damage")
				}
				return
			}
			if len(drops) != 1 || drops[0].TemplateID != item.AdenaID || drops[0].Count != dropMonsterAdena {
				t.Fatalf("ground drops = %+v, want one stack of %d adena", drops, dropMonsterAdena)
			}
			if drops[0].OwnerID != objID {
				t.Fatalf("drop protected to %d, want the post-teleport attacker %d", drops[0].OwnerID, objID)
			}
			if !monster.SpoilPool().Sweepable() {
				t.Fatal("spoil pool empty, want the post-teleport attacker's spoil roll")
			}
		})
	}
}
