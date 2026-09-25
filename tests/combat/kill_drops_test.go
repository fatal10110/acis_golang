package combat

import (
	"testing"
	"time"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/npc"
	playermodel "github.com/fatal10110/acis_golang/internal/gameserver/model/actor/player"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/item"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/network"
	"github.com/fatal10110/acis_golang/internal/gameserver/skill/effect"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
)

// TestDebufferLevelPenalizesDrops exercises the cast event rather than the
// damage table: the high-level player never damages the monsters.
func TestDebufferLevelPenalizesDrops(t *testing.T) {
	t.Parallel()
	const (
		debuffID    = 1069
		aggReduceID = 1068
	)
	srv := gameservertest.Boot(t,
		gameservertest.WithCharacter("Debuffer", 10, 0),
		gameservertest.WithWantChars(1),
		gameservertest.WithDeepBlueDropRules(true),
		gameservertest.WithSkills(combatPersistence(t, []modelskill.Definition{
			{
				ID: debuffID, Level: 1, Activation: modelskill.ActivationActive, Target: modelskill.TargetOne,
				CastRange: 900, HitTime: 500, ReuseDelay: 0, StaticHitTime: true, StaticReuse: true,
				SkillType: "DEBUFF", Debuff: true, Offensive: true,
				Effects: []modelskill.EffectTemplate{{Name: "Sleep", Time: 10, EffectPower: 100, EffectPowerSet: true}},
			},
			{
				ID: aggReduceID, Level: 1, Activation: modelskill.ActivationActive, Target: modelskill.TargetOne,
				CastRange: 900, HitTime: 500, ReuseDelay: 0, StaticHitTime: true, StaticReuse: true,
				SkillType: "AGGREDUCE", Offensive: true,
			},
		})),
	)
	c, debufferID := srv.Client, srv.SoleObjectID(t)
	seedKnownSkill(t, srv, debufferID, debuffID, 1)
	seedKnownSkill(t, srv, debufferID, aggReduceID, 1)
	startInWorld(t, c)
	low := srv.SeedCharacterFor(t, "low", "Low", 1, 0)
	lowClient := srv.DialClient(t, "low", 1)
	startInWorld(t, lowClient)
	drainUntilQuiet(t, c)
	lowObj, ok := srv.State.Player(low.ObjectID())
	if !ok {
		t.Fatal("low-level player missing from world")
	}
	lowPlayer, ok := network.OnlineCharacter(lowObj)
	if !ok {
		t.Fatal("low-level player is not online")
	}

	const kills = 30
	for i := range kills {
		monster := srv.SpawnHostileNPCTemplateAt(t, dropMonsterTemplate(), location.Location{X: hostileX, Y: hostileY, Z: hostileZ})
		drainUntilQuiet(t, c)
		targetHostile(t, c, monster.ObjectID())
		drainUntilQuiet(t, c)
		if i == 0 {
			c.Send(encodeRequestMagicSkillUse(aggReduceID, false, false))
			readCastStartFrames(t, c, debufferID, aggReduceID, 1, 500, 0, monster.ObjectID())
			srv.Advance(t, time.Second)
			drainUntilQuiet(t, c)
			if got := monster.HighestAttackerLevel(1); got != 1 {
				t.Fatalf("highest attacker level after AGGREDUCE = %d, want empty-set fallback 1", got)
			}
		}
		c.Send(encodeRequestMagicSkillUse(debuffID, false, false))
		readCastStartFrames(t, c, debufferID, debuffID, 1, 500, 0, monster.ObjectID())
		srv.AdvanceUntil(t, "debuff hit", func() bool { return monster.HighestAttackerLevel(1) == 10 })
		drainUntilQuiet(t, c)
		if got := monster.HighestAttackerLevel(1); got != 10 {
			t.Fatalf("highest attacker level after debuff = %d, want 10", got)
		}
		if !monster.TakeDamage(1_000_000, lowPlayer) {
			t.Fatal("low-level player's hit did not kill the monster")
		}
		if got := monster.HighestAttackerLevel(0); got != 0 {
			t.Fatalf("highest attacker level after death = %d, want an empty set", got)
		}
		drainUntilQuiet(t, c)
	}
	if drops := groundDrops(srv); len(drops) == kills {
		t.Fatalf("all %d kills dropped, want the high-level debuffer's deep-blue penalty", kills)
	}
}

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
	t.Parallel()
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
	t.Parallel()
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
