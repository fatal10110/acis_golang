package pets

import (
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/npc"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/zone"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
)

// TestMobAggroInPeaceZoneSeesLivePeaceMembership pins the npcs.properties
// MobAggroInPeaceZone switch against a live player and its summon whose
// peace-zone membership the zone runtime sets: with the shipped True an
// aggressive monster auto-attacks either inside a peace zone, and with False
// it skips both there, while outside any peace zone the switch is moot.
func TestMobAggroInPeaceZoneSeesLivePeaceMembership(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name    string
		inPeace bool
	}{
		{"inside a peace zone", true},
		{"outside any peace zone", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			var extra []gameservertest.Option
			if tc.inPeace {
				zones := zone.NewIndex()
				zones.Add(zone.NewPeace(1, wholeWorldForm(t)))
				extra = append(extra, gameservertest.WithZones(zones))
			}
			h := bootOwnerWithCollarOpts(t, extra)
			wolf, _ := h.spawnWolf(t)
			drainUntilQuiet(t, h.client)

			owner, ownerChar := onlinePlayer(t, h.srv, h.ownerID), onlineCharacterOf(t, h.srv, h.ownerID)
			if got := ownerChar.InPeaceZone(); got != tc.inPeace {
				t.Fatalf("owner InPeaceZone = %v, want %v", got, tc.inPeace)
			}
			if got := wolf.InPeaceZone(); got != tc.inPeace {
				t.Fatalf("wolf InPeaceZone = %v, want %v", got, tc.inPeace)
			}

			x, y, z := ownerChar.Position()
			monster := h.srv.SpawnHostileNPCTemplateAt(t, &npc.Template{
				ID: 100, TemplateID: 100, Type: "Monster", Level: 1, HPMax: 1000, AtkSpd: 300,
				RunSpeed: 120, WalkSpeed: 60, CollisionRadius: 8, CollisionHeight: 20, AggroRange: 500,
			}, location.Location{X: x + 40, Y: y, Z: z})

			for _, cfg := range []struct {
				name string
				ai   npc.AIConfig
				want bool
			}{
				{"MobAggroInPeaceZone=True", npc.DefaultAIConfig(), true},
				{"MobAggroInPeaceZone=False", npc.AIConfig{}, !tc.inPeace},
			} {
				monster.SetAIConfig(cfg.ai)
				if got := monster.AutoAttackTargetValid(owner, 500, false); got != cfg.want {
					t.Errorf("%s: monster auto-attacks the owner = %v, want %v", cfg.name, got, cfg.want)
				}
				if got := monster.AutoAttackTargetValid(wolf, 500, false); got != cfg.want {
					t.Errorf("%s: monster auto-attacks the wolf = %v, want %v", cfg.name, got, cfg.want)
				}
			}
		})
	}
}
