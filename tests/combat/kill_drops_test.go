package combat

import (
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/npc"
	playermodel "github.com/fatal10110/acis_golang/internal/gameserver/model/actor/player"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/item"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/network"
	"github.com/fatal10110/acis_golang/internal/gameserver/skill/effect"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
)

// dropMonsterAdena is the guaranteed currency drop of dropMonsterTemplate.
const dropMonsterAdena = 10

// dropMonsterTemplate is a monster with one guaranteed currency drop and one
// guaranteed spoil entry, so any reward roll that runs is observable.
func dropMonsterTemplate() *npc.Template {
	return &npc.Template{
		ID: 100, TemplateID: 100, Type: "Monster", Level: 1, HPMax: 1000,
		AtkSpd: 300, RunSpeed: 120, WalkSpeed: 60, CollisionRadius: 8, CollisionHeight: 20,
		Drops: []item.DropCategory{
			{Kind: item.DropCurrency, Chance: 100, Drops: []item.Drop{{ItemID: item.AdenaID, Min: dropMonsterAdena, Max: dropMonsterAdena, Chance: 100}}},
			{Kind: item.DropSpoil, Chance: 100, Drops: []item.Drop{{ItemID: item.AdenaID, Min: 1, Max: 1, Chance: 100}}},
		},
	}
}

// spawnSpoiledDropMonster boots a player in world beside a spoiled drop
// monster and returns the player's object id with the monster.
func spawnSpoiledDropMonster(t *testing.T) (*gameservertest.Server, int32, *npc.Hostile) {
	t.Helper()
	srv := gameservertest.Boot(t,
		gameservertest.WithCharacter("Newbie", 5, 0),
		gameservertest.WithWantChars(1),
	)
	c, objID := srv.Client, srv.SoleObjectID(t)
	startInWorld(t, c)
	monster := srv.SpawnHostileNPCTemplateAt(t, dropMonsterTemplate(), location.Location{X: hostileX, Y: hostileY, Z: hostileZ})
	drainUntilQuiet(t, c)
	if !monster.SpoilPool().Mark(objID) {
		t.Fatal("spoil mark refused on a fresh monster")
	}
	return srv, objID, monster
}

func groundDrops(srv *gameservertest.Server) []item.GroundSnapshot {
	return srv.GroundItems.Snapshots(func(int32) bool { return false })
}

// TestPlayerKillDropsProtectedLoot is the control: a player kill rolls the
// drop onto the ground reserved to the player and fills the spoil pool.
func TestPlayerKillDropsProtectedLoot(t *testing.T) {
	srv, objID, monster := spawnSpoiledDropMonster(t)
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

	drops := groundDrops(srv)
	if len(drops) != 1 || drops[0].TemplateID != item.AdenaID || drops[0].Count != dropMonsterAdena {
		t.Fatalf("ground drops = %+v, want one stack of %d adena", drops, dropMonsterAdena)
	}
	if drops[0].OwnerID != objID {
		t.Fatalf("drop protected to %d, want the killer %d", drops[0].OwnerID, objID)
	}
	if !monster.SpoilPool().Sweepable() {
		t.Fatal("spoil pool empty after a player kill, want the spoil roll")
	}
}

// TestDeadTopDealerReceivesDrops pins the drop receiver to the top damage
// dealer even after that player died: a guard finishing the monster still
// drops the loot reserved to the dead player and fills the spoil pool.
func TestDeadTopDealerReceivesDrops(t *testing.T) {
	srv, objID, monster := spawnSpoiledDropMonster(t)
	obj, ok := srv.State.Player(objID)
	if !ok {
		t.Fatal("player missing from world state")
	}
	player, ok := network.OnlineCharacter(obj)
	if !ok {
		t.Fatalf("player %T is not an online character", obj)
	}
	guard := srv.SpawnHostileNPCKindAt(t, "Guard", location.Location{X: hostileX + 40, Y: hostileY, Z: hostileZ})

	if monster.TakeDamage(int(monster.CurrentHP())/2, player) {
		t.Fatal("player's half-HP hit killed the monster")
	}
	srv.MarkPlayerDead(t, objID)
	if !monster.TakeDamage(1_000_000, guard) {
		t.Fatal("guard's lethal hit did not kill the monster")
	}

	drops := groundDrops(srv)
	if len(drops) != 1 || drops[0].TemplateID != item.AdenaID || drops[0].Count != dropMonsterAdena {
		t.Fatalf("ground drops = %+v, want one stack of %d adena", drops, dropMonsterAdena)
	}
	if drops[0].OwnerID != objID {
		t.Fatalf("drop protected to %d, want the dead top dealer %d", drops[0].OwnerID, objID)
	}
	if !monster.SpoilPool().Sweepable() {
		t.Fatal("spoil pool empty, want the dead top dealer's spoil roll")
	}
}

// TestFakeDeadAttackerKeepsKillExp pins the exp gate to real death: an
// attacker playing dead when a guard finishes the monster still earns the
// kill's exp and SP.
func TestFakeDeadAttackerKeepsKillExp(t *testing.T) {
	srv := gameservertest.Boot(t,
		gameservertest.WithCharacter("Newbie", 5, 0),
		gameservertest.WithWantChars(1),
		gameservertest.WithLevels(levelTableFor(t)),
	)
	c, objID := srv.Client, srv.SoleObjectID(t)
	startInWorld(t, c)
	monster := spawnRewardedNPC(t, srv, 5000, 25)
	guard := srv.SpawnHostileNPCKindAt(t, "Guard", location.Location{X: hostileX + 40, Y: hostileY, Z: hostileZ})
	drainUntilQuiet(t, c)
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
	fakeDeath, err := effect.New(effect.Skill{ID: 1}, modelskill.EffectTemplate{Name: "FakeDeath"})
	if err != nil {
		t.Fatalf("new fake-death effect: %v", err)
	}
	fakeDeath.Effected = player
	player.EffectList().Add(fakeDeath)
	if !player.FakeDead() || player.Dead() {
		t.Fatalf("player FakeDead=%v Dead=%v, want fake death only", player.FakeDead(), player.Dead())
	}
	if !monster.TakeDamage(1_000_000, guard) {
		t.Fatal("guard's lethal hit did not kill the monster")
	}

	// The guard is no reward entry, so the player's damage is the whole
	// total and earns the full share.
	wantExp, wantSp := playermodel.KillRewardExpAndSp(5000, 25, 1, 1, 5-1)
	readExpSpGain(t, c, wantExp, wantSp)
}

// TestKillWithoutPlayerReceiverDropsNothing covers every death no player
// earned: the corpse drops nothing and its spoil pool stays empty.
func TestKillWithoutPlayerReceiverDropsNothing(t *testing.T) {
	cases := map[string]func(*testing.T, *gameservertest.Server, *npc.Hostile) bool{
		// No attacker registers any threat: the threat table is empty.
		"no threat": func(_ *testing.T, _ *gameservertest.Server, monster *npc.Hostile) bool {
			return monster.TakeDamage(1_000_000, nil)
		},
		// A guard kill leaves only a non-playable in the threat table and
		// the guard as killer.
		"guard killer": func(t *testing.T, srv *gameservertest.Server, monster *npc.Hostile) bool {
			guard := srv.SpawnHostileNPCKindAt(t, "Guard", location.Location{X: hostileX + 40, Y: hostileY, Z: hostileZ})
			return monster.TakeDamage(1_000_000, guard)
		},
		// A guard's hit leaves non-player threat, then the monster's own
		// lethal HP cost names itself as killer.
		"self killer": func(t *testing.T, srv *gameservertest.Server, monster *npc.Hostile) bool {
			guard := srv.SpawnHostileNPCKindAt(t, "Guard", location.Location{X: hostileX + 40, Y: hostileY, Z: hostileZ})
			monster.TakeDamage(10, guard)
			monster.ConsumeHP(float64(monster.CurrentHP()))
			return monster.Dead()
		},
	}
	for name, kill := range cases {
		t.Run(name, func(t *testing.T) {
			srv, _, monster := spawnSpoiledDropMonster(t)

			if !kill(t, srv, monster) {
				t.Fatal("lethal hit did not kill the monster")
			}

			if drops := groundDrops(srv); len(drops) != 0 {
				t.Fatalf("ground drops = %+v, want none without a player receiver", drops)
			}
			if monster.SpoilPool().Sweepable() {
				t.Fatal("spoil pool filled without a player receiver")
			}
		})
	}
}
