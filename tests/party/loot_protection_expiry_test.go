package party

import (
	"testing"
	"time"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/npc"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/item"
	"github.com/fatal10110/acis_golang/internal/gameserver/network"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
)

// lootLockDrops is the one guaranteed drop of the loot-lock suite: a single
// potion.
var lootLockDrops = []item.DropCategory{{Kind: item.DropNormal, Chance: 100, Drops: []item.Drop{{ItemID: lootPotionID, Min: 1, Max: 1, Chance: 100}}}}

func lootLockMonster() *npc.Template {
	return &npc.Template{
		ID: 100, TemplateID: 100, Type: "Monster", Level: 40, HPMax: 1000,
		AtkSpd: 300, RunSpeed: 120, WalkSpeed: 60, CollisionRadius: 8, CollisionHeight: 20,
		Drops: lootLockDrops,
	}
}

func lootLockBoss() *npc.Template {
	tmpl := raidBoss(raidBossID)
	tmpl.Drops = lootLockDrops
	return tmpl
}

// TestLootProtectionLiftsAtItsDeadline: a drop reserved to its killer
// refuses a stranger's pickup until the protection runs out and admits it
// from that very moment, 15 seconds after a monster's death and 300 after a
// raid boss's, not on the next ground-item sweep.
func TestLootProtectionLiftsAtItsDeadline(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name    string
		tmpl    func() *npc.Template
		protect time.Duration
	}{
		{"monster", lootLockMonster, 15 * time.Second},
		{"raid boss", lootLockBoss, 300 * time.Second},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			g := bootGroup(t, []seat{{"Killer", 40}, {"Looter", 40}})
			mob := g.spawnBesideLeader(t, tc.tmpl())
			if !mob.TakeDamage(1_000_000, g.combatant(t, 0)) {
				t.Fatal("Killer's hit did not kill the mob")
			}
			g.srv.Settle(t)
			drops := g.srv.GroundItems.Snapshots(nil)
			if len(drops) != 1 || drops[0].TemplateID != lootPotionID {
				t.Fatalf("ground drops = %+v, want the one potion", drops)
			}
			drop := drops[0]
			if drop.OwnerID != g.players[0].id {
				t.Fatalf("drop reserved to %d, want the killer %d", drop.OwnerID, g.players[0].id)
			}

			// The looter stands on the drop, so a click picks it up with
			// no walk and no time passing.
			obj, _ := g.srv.State.Player(g.players[1].id)
			looter, ok := network.OnlineCharacter(obj)
			if !ok {
				t.Fatalf("Looter is %T", obj)
			}
			looter.TeleportTo(drop.X, drop.Y, drop.Z, 0)
			g.srv.Settle(t)
			g.quiet(t)

			g.srv.Advance(t, tc.protect-time.Millisecond)
			g.quiet(t)
			g.players[1].c.Send(encodeAction(drop.ObjectID, drop.X, drop.Y, drop.Z))
			g.srv.Settle(t)
			requireMessage(t, "Looter", drainFrames(t, g.players[1].c), serverpackets.SystemMessageFailedToPickupS1, lootPotionID)
			if got := g.carried(t, 1, lootPotionID); got != 0 {
				t.Fatalf("Looter carries %d potions a moment before the protection ends, want 0", got)
			}

			g.srv.Advance(t, time.Millisecond)
			g.players[1].c.Send(encodeAction(drop.ObjectID, drop.X, drop.Y, drop.Z))
			g.srv.SettlePickup(t, g.players[1].id)
			requireNoMessage(t, "Looter", drainFrames(t, g.players[1].c), serverpackets.SystemMessageFailedToPickupS1)
			if got := g.carried(t, 1, lootPotionID); got != 1 {
				t.Fatalf("Looter carries %d potions once the protection ends, want 1", got)
			}
			if left := g.srv.GroundItems.Snapshots(nil); len(left) != 0 {
				t.Fatalf("ground drops after the pickup = %+v, want none", left)
			}
		})
	}
}
