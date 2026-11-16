package combat

import (
	"strings"
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/npc"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/item"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	"github.com/fatal10110/acis_golang/internal/gameserver/network"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
)

// unknownDropItemID is an item id the item table does not hold.
const unknownDropItemID = 999_999

// TestKillDropOfUnknownItemIsLogged pins that a kill drop the server cannot
// place is skipped on its own and said so: a monster whose drop table names
// an unknown item still dies and drops the rest of its loot, and the skip
// leaves one log line naming the NPC and the item.
func TestKillDropOfUnknownItemIsLogged(t *testing.T) {
	t.Parallel()
	srv := gameservertest.Boot(t,
		gameservertest.WithCharacter("Newbie", 5, 0),
		gameservertest.WithWantChars(1),
		gameservertest.WithCapturedLog(),
	)
	c, objID := srv.Client, srv.SoleObjectID(t)
	startInWorld(t, c)
	tmpl := &npc.Template{
		ID: 100, TemplateID: 100, Type: "Monster", Level: 1, HPMax: 1000,
		AtkSpd: 300, RunSpeed: 120, WalkSpeed: 60, CollisionRadius: 8, CollisionHeight: 20,
		Drops: []item.DropCategory{
			{Kind: item.DropCurrency, Chance: 100, Drops: []item.Drop{{ItemID: item.AdenaID, Min: 10, Max: 10, Chance: 100}}},
			{Kind: item.DropNormal, Chance: 100, Drops: []item.Drop{{ItemID: unknownDropItemID, Min: 1, Max: 1, Chance: 100}}},
		},
	}
	monster := srv.SpawnHostileNPCTemplateAt(t, tmpl, location.Location{X: hostileX, Y: hostileY, Z: hostileZ})
	drainUntilQuiet(t, c)
	obj, ok := srv.State.Player(objID)
	if !ok {
		t.Fatal("player missing from world state")
	}
	player, ok := network.OnlineCharacter(obj)
	if !ok {
		t.Fatalf("player %T is not an online character", obj)
	}

	if !monster.TakeDamage(1_000_000, player) {
		t.Fatal("lethal hit did not kill the monster")
	}
	srv.Settle(t)

	drops := groundDrops(srv)
	if len(drops) != 1 || drops[0].TemplateID != item.AdenaID || drops[0].Count != 10 {
		t.Fatalf("ground drops = %+v, want the 10 adena alone", drops)
	}
	var skipped []string
	for line := range strings.SplitSeq(srv.LogText(), "\n") {
		if strings.Contains(line, "kill reward: drop skipped") {
			skipped = append(skipped, line)
		}
	}
	if len(skipped) != 1 {
		t.Fatalf("drop-skipped log lines = %q, want one", skipped)
	}
	for _, field := range []string{`"npc_id":100`, `"item_id":999999`, `"count":1`, `"error":"unknown item id"`} {
		if !strings.Contains(skipped[0], field) {
			t.Fatalf("drop-skipped log line %q lacks %s", skipped[0], field)
		}
	}
}
