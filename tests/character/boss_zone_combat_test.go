package character

import (
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/ai"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/attackable"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/zone"
	"github.com/fatal10110/acis_golang/internal/gameserver/network"
)

// TestBossZoneLastPlayableOutSendsFightingRaidHome pins BossZone.onExit's
// playable branch for a raid that is in combat: returnHome drops the
// fight's aggro (cleanAllHate, with every queued attack desire) and sets
// the raid off for its spawn, unless a skill-cast hate list is still
// filled, in which case it stays and keeps its fight.
func TestBossZoneLastPlayableOutSendsFightingRaidHome(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name     string
		castHate bool
		wantHome bool
	}{
		{"aggro list only", false, true},
		{"skill-cast hate filled", true, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			srv, _, objID := bootInBossZone(t)
			raid, _ := spawnLairNPCs(t, srv)
			obj, ok := srv.State.Player(objID)
			if !ok {
				t.Fatal("player missing from world state")
			}
			player, ok := network.OnlineCharacter(obj)
			if !ok {
				t.Fatalf("player %T is not an online character", obj)
			}
			victim := obj.(attackable.Combatant)

			raid.AddDamageHate(victim, 10, 50)
			raid.AI().Desires().AddOrUpdate(&ai.Desire{Kind: ai.IntentionAttack, FinalTarget: victim, Weight: 50})
			if tc.castHate {
				raid.AI().Hates().Add(victim, 30)
			}

			_, y, z := srv.PlayerPosition(t, objID)
			for range zone.StepsPerRevalidation {
				player.SyncPosition(location.Location{X: 1_200, Y: y, Z: z})
			}
			srv.Settle(t)

			hate := raid.AI().ThreatsIfBuilt().Hate(victim)
			attackQueued := raid.AI().Desires().HasKind(ai.IntentionAttack)
			if tc.wantHome {
				if hate != 0 || attackQueued {
					t.Fatalf("fighting raid sent home keeps hate %v, attack desire queued %v; want both dropped", hate, attackQueued)
				}
				if !raid.IsMoving() {
					t.Fatal("fighting raid did not set off for home")
				}
				return
			}
			if hate != 50 || !attackQueued {
				t.Fatalf("raid with a filled hate list has hate %v, attack desire queued %v; want its fight kept (50, true)", hate, attackQueued)
			}
			if raid.IsMoving() {
				t.Fatal("raid with a filled hate list set off for home")
			}
		})
	}
}
