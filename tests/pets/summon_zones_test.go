package pets

import (
	"sync/atomic"
	"testing"
	"time"

	"github.com/fatal10110/acis_golang/internal/commons"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/zone"
	"github.com/fatal10110/acis_golang/internal/gameserver/network"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/clientpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
)

// summonZoneBox is a zone volume over x in [minX, maxX] around the owner's
// spawn row; the owner stands at x 10 and its wolf appears 40 east of it.
func summonZoneBox(t *testing.T, minX, maxX int) zone.Form {
	t.Helper()
	form, err := zone.NewCuboid(minX, maxX, -1_000, 1_000, -10_000, 10_000)
	if err != nil {
		t.Fatalf("zone form: %v", err)
	}
	return form
}

// TestSummonEnteringBossZoneWithoutLeaveIsDismissed pins BossZone.onEnter's
// summon branch (BossZone.java:111-120): a summon whose owner holds no entry
// permission is dismissed on entering the lair, while one whose owner was
// let in stays.
func TestSummonEnteringBossZoneWithoutLeaveIsDismissed(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name      string
		permitted bool
	}{
		{name: "owner without permission", permitted: false},
		{name: "permitted owner", permitted: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			set := commons.NewStatSet()
			set.Set("InvadeTime", "600000")
			// The lair covers the wolf's spawn point, not its owner.
			boss, err := zone.NewBoss(1, summonZoneBox(t, 30, 1_000), set)
			if err != nil {
				t.Fatalf("build boss zone: %v", err)
			}
			var summonEnters atomic.Int32
			boss.OnEnter(func(a zone.Actor) {
				if a.Class() == zone.ClassSummon {
					summonEnters.Add(1)
				}
			})
			zones := zone.NewIndex()
			zones.Add(boss)
			h := bootOwnerWithCollarOpts(t, []gameservertest.Option{gameservertest.WithZones(zones)})
			if tc.permitted {
				boss.AllowEntry(h.ownerID, time.Minute)
				wolf, _ := h.spawnWolf(t)
				h.srv.Settle(t)
				if !wolf.InsideZone(zone.FlagBoss) {
					t.Fatal("wolf spawned inside the boss zone is not in it")
				}
				if _, ok := h.srv.State.Summon(h.ownerID); !ok {
					t.Fatal("the wolf of a permitted owner was dismissed")
				}
				return
			}
			h.client.Send(encodeUseItem(h.collarID, false))
			h.srv.AdvanceUntil(t, "wolf entered the lair and was dismissed", func() bool {
				_, ok := h.srv.State.Summon(h.ownerID)
				return summonEnters.Load() == 1 && !ok
			})
			readUntilOpcode(t, h.client, serverpackets.OpcodePetDelete, "PetDelete")
			if got := len(boss.Occupants()); got != 0 {
				t.Fatalf("boss zone occupants after the dismissal = %d, want 0", got)
			}
		})
	}
}

// TestSummonZonesFollowOwnerTeleport pins a summon's zone membership across
// its owner's teleport (Creature.teleportTo, Creature.java:386-429): the
// summon leaves its zones and enters those at the destination, and a swamp
// it stands in slows it (PlayableStatus.getMoveSpeed, PetStatus.java:84-91)
// until it leaves.
func TestSummonZonesFollowOwnerTeleport(t *testing.T) {
	t.Parallel()
	peace := zone.NewPeace(1, summonZoneBox(t, -100_000, 100_000))
	var enters, exits atomic.Int32
	peace.OnEnter(func(a zone.Actor) {
		if a.Class() == zone.ClassSummon {
			enters.Add(1)
		}
	})
	peace.OnExit(func(a zone.Actor) {
		if a.Class() == zone.ClassSummon {
			exits.Add(1)
		}
	})
	// The swamp covers the wolf's spawn point, not its owner.
	swamp, err := zone.NewSwamp(2, summonZoneBox(t, 30, 1_000), commons.NewStatSet())
	if err != nil {
		t.Fatalf("build swamp: %v", err)
	}
	zones := zone.NewIndex()
	zones.Add(peace)
	zones.Add(swamp)
	h := bootOwnerWithCollarOpts(t, []gameservertest.Option{gameservertest.WithZones(zones)})
	wolf, _ := h.spawnWolf(t)
	if got, gotExits := enters.Load(), exits.Load(); got != 1 || gotExits != 0 {
		t.Fatalf("after the summon: wolf peace enters/exits = %d/%d, want 1/0", got, gotExits)
	}
	base := wolfTemplate().RunSpeed
	if !wolf.InsideZone(zone.FlagSwamp) {
		t.Fatal("wolf spawned in a swamp is not in it")
	}
	swampSpeed := wolf.MoveSpeed(base)
	drainUntilQuiet(t, h.client)

	obj, ok := h.srv.State.Player(h.ownerID)
	if !ok {
		t.Fatal("owner missing from world")
	}
	owner, ok := network.OnlineCharacter(obj)
	if !ok {
		t.Fatalf("owner = %T, want online character", obj)
	}
	x, y, z := h.srv.PlayerPosition(t, h.ownerID)
	owner.TeleportTo(x+5_000, y, z, 0)
	readUntilOpcode(t, h.client, serverpackets.OpcodeTeleportToLocation, "owner teleport")
	drainUntilQuiet(t, h.client)
	h.client.Send(encodeSingleOpcode(clientpackets.OpcodeAppearing))
	h.srv.AdvanceUntil(t, "wolf back in the peace zone", func() bool { return enters.Load() == 2 })
	if got := exits.Load(); got != 1 {
		t.Fatalf("after the teleport: wolf peace exits = %d, want 1", got)
	}
	if wolf.InsideZone(zone.FlagSwamp) {
		t.Fatal("wolf teleported out of the swamp is still in it")
	}
	// The swamp scales the base speed before the RUN_SPEED stat, which
	// only multiplies it here.
	speed := wolf.MoveSpeed(base)
	if want := speed * float64(100+swamp.MoveBonus) / 100; swampSpeed != want {
		t.Fatalf("wolf move speed in the swamp = %v, want %v (%v out of it)", swampSpeed, want, speed)
	}
	drainUntilQuiet(t, h.client)
}
