package pets

import (
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/attack"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/attackable"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/npc"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
)

// Wyvern (npc 12621) footprint from the shipped npc data
// (aCis_datapack/data/xml/npcs/12000-12999.xml: radius 60.0, height 80.0).
const (
	wyvernRadius = 60.0
	wyvernHeight = 80.0
)

func wyvernTemplate() *npc.Template {
	return &npc.Template{
		ID: wyvernNPCID, TemplateID: wyvernNPCID, Name: "Wyvern",
		AtkSpd: 300, RunSpeed: 120, WalkSpeed: 60,
		CollisionRadius: wyvernRadius, CollisionHeight: wyvernHeight,
	}
}

// riderBody is the mounted player's footprint and attack-gate surface.
type riderBody interface {
	CollisionRadius() float64
	CollisionHeight() float64
	AttackDisabled() bool
}

// TestWyvernRiderUsesMountFootprint pins Player.getCollisionRadius /
// getCollisionHeight: once the wyvern collar mounts the owner, the player's
// body is the wyvern template's, so an attacker's physical reach against the
// rider widens by the difference.
func TestWyvernRiderUsesMountFootprint(t *testing.T) {
	t.Parallel()
	h := bootOwnerWithCollarOpts(t, []gameservertest.Option{
		gameservertest.WithNPCs(npc.NewTable([]*npc.Template{wolfTemplate(), treeTemplate(), wyvernTemplate()})),
	}, seedItem{TemplateID: wyvernCollarID, Count: 1})

	obj, ok := h.srv.State.Player(h.ownerID)
	if !ok {
		t.Fatal("owner missing from world state")
	}
	rider, ok := obj.(riderBody)
	if !ok {
		t.Fatalf("world player %T does not expose its body", obj)
	}
	footRadius, footHeight := rider.CollisionRadius(), rider.CollisionHeight()
	if footRadius >= wyvernRadius || footHeight >= wyvernHeight {
		t.Fatalf("unmounted body = %v/%v, want smaller than the wyvern's %v/%v", footRadius, footHeight, wyvernRadius, wyvernHeight)
	}

	// An attacker parked one unit past its reach against the unmounted body.
	px, py, pz := h.srv.PlayerPosition(t, h.ownerID)
	probe := h.srv.SpawnHostileNPCAt(t, location.Location{X: px + 2000, Y: py, Z: pz})
	offset := attack.PhysicalReach(probe.PhysicalAttackRange(), probe.CollisionRadius(), footRadius, false) + 1
	attacker := h.srv.SpawnHostileNPCAt(t, location.Location{X: px + offset, Y: py, Z: pz})
	drainUntilQuiet(t, h.client)
	if attacker.InAttackRange(rider.(attackable.Combatant)) {
		t.Fatalf("attacker at %d reaches the unmounted player, want out of range", offset)
	}

	h.client.Send(encodeUseItem(h.seededItem(t, wyvernCollarID), false))
	assertFrameOpcode(t, mustRead(t, h.client, "Ride broadcast"), serverpackets.OpcodeRide, "Ride")
	readUntilOpcode(t, h.client, serverpackets.OpcodeUserInfo, "mounted UserInfo")

	if got := rider.CollisionRadius(); got != wyvernRadius {
		t.Fatalf("mounted CollisionRadius() = %v, want %v", got, wyvernRadius)
	}
	if got := rider.CollisionHeight(); got != wyvernHeight {
		t.Fatalf("mounted CollisionHeight() = %v, want %v", got, wyvernHeight)
	}
	if !attacker.InAttackRange(rider.(attackable.Combatant)) {
		t.Fatalf("attacker at %d does not reach the wyvern rider, want in range", offset)
	}
}

// TestWyvernRiderCannotAttack pins the flying term of
// Creature.isAttackingDisabled (Player.isFlying is mount type 2): a wyvern
// rider's click on an in-range monster answers ActionFailed and never swings.
func TestWyvernRiderCannotAttack(t *testing.T) {
	t.Parallel()
	h := bootOwnerWithCollarOpts(t, []gameservertest.Option{
		gameservertest.WithNPCs(npc.NewTable([]*npc.Template{wolfTemplate(), treeTemplate(), wyvernTemplate()})),
	}, seedItem{TemplateID: wyvernCollarID, Count: 1})

	px, py, pz := h.srv.PlayerPosition(t, h.ownerID)
	hostile := h.srv.SpawnHostileNPCAt(t, location.Location{X: px + 20, Y: py, Z: pz})
	drainUntilQuiet(t, h.client)

	h.client.Send(encodeUseItem(h.seededItem(t, wyvernCollarID), false))
	assertFrameOpcode(t, mustRead(t, h.client, "Ride broadcast"), serverpackets.OpcodeRide, "Ride")
	drainUntilQuiet(t, h.client)

	obj, ok := h.srv.State.Player(h.ownerID)
	if !ok {
		t.Fatal("owner missing from world state")
	}
	if rider, ok := obj.(riderBody); !ok || !rider.AttackDisabled() {
		t.Fatalf("wyvern rider %T AttackDisabled() = false, want true", obj)
	}

	h.client.Send(encodeAction(hostile.ObjectID(), int32(px), int32(py), int32(pz), false))
	drainUntilQuiet(t, h.client)
	before := hostile.CurrentHP()
	h.client.Send(encodeAction(hostile.ObjectID(), int32(px), int32(py), int32(pz), false))
	frames := readUntilOpcode(t, h.client, serverpackets.OpcodeActionFailed, "rider attack ActionFailed")
	frames = append(frames, drainFrames(t, h.client)...)
	for _, f := range frames {
		if f[0] == serverpackets.OpcodeAttack || f[0] == serverpackets.OpcodeAutoAttackStart {
			t.Fatalf("wyvern rider click produced opcode %#x, want no swing", f[0])
		}
	}
	if got := hostile.CurrentHP(); got != before {
		t.Fatalf("hostile HP = %d after rider click, want unchanged %d", got, before)
	}
}

// TestMountedPlayerCannotEquipWeapon pins UseItem's mounted hand-slot gate:
// a rider equipping a weapon answers CANNOT_EQUIP_ITEM_DUE_TO_BAD_CONDITION
// alone — no UserInfo refresh — and the weapon stays in the inventory.
func TestMountedPlayerCannotEquipWeapon(t *testing.T) {
	t.Parallel()
	const swordID = int32(30)
	h := bootOwnerWithCollarOpts(t, []gameservertest.Option{
		gameservertest.WithNPCs(npc.NewTable([]*npc.Template{wolfTemplate(), treeTemplate(), wyvernTemplate()})),
	}, seedItem{TemplateID: wyvernCollarID, Count: 1}, seedItem{TemplateID: swordID, Count: 1})

	h.client.Send(encodeUseItem(h.seededItem(t, wyvernCollarID), false))
	assertFrameOpcode(t, mustRead(t, h.client, "Ride broadcast"), serverpackets.OpcodeRide, "Ride")
	drainUntilQuiet(t, h.client)

	sword := h.seededItem(t, swordID)
	h.client.Send(encodeUseItem(sword, false))
	assertStaticSystemMessage(t, mustRead(t, h.client, "mounted equip refusal"), serverpackets.SystemMessageCannotEquipItemDueToBadCondition)
	if extra := drainFrames(t, h.client); len(extra) != 0 {
		t.Fatalf("mounted equip refusal sent %d more frames (first opcode %#x), want none", len(extra), extra[0][0])
	}

	inv := h.ownerInventory(t)
	if inst := inv.ItemByObjectID(sword); inst == nil || inst.Snapshot().Equipped() {
		t.Fatalf("sword after the mounted refusal = %+v, want held and unequipped", inst)
	}
}
