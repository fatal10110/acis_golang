package skills

import (
	"math"
	"testing"

	"github.com/fatal10110/acis_golang/internal/commons/wire"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/door"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/item"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/clientpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
)

// Reference: CreatureAttack.doAttackHitByPole picks a pole swing's extra
// targets from the attacker's known Creatures in its attack range, doors
// among them, keeping those it faces within the attack angle that
// isAttackableBy lets it hit; Door.isAttackableBy holds for a playable
// only during the residence's siege.

const (
	poleWeaponID     = 9341
	poleCountSkillID = 9342
)

func poleCatalog() *item.Table {
	return item.NewTable(append(gameservertest.ItemTemplates().All(), &item.Template{
		ID: poleWeaponID, Name: "Pole", Kind: item.KindWeapon, Slot: item.SlotLRHand,
		Duration: -1, Destroyable: true, DefaultAction: item.ActionEquip,
		Weapon: &item.WeaponDetail{Type: item.WeaponPole},
	}))
}

// poleCount is a passive letting a pole swing reach three targets.
func poleCount() modelskill.Definition {
	return modelskill.Definition{
		ID: poleCountSkillID, Level: 1, Activation: modelskill.ActivationPassive, Target: modelskill.TargetSelf,
		Funcs: []modelskill.FuncTemplate{{Op: modelskill.FuncAdd, Stat: "atkCountMax", Value: 3}},
	}
}

func TestPoleSwingSplashesOntoDoor(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name  string
		siege bool
	}{{name: "under siege", siege: true}, {name: "outside a siege"}} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			tmpl := combatDoorTemplate(door.KindDoor)
			srv := gameservertest.Boot(t,
				gameservertest.WithCharacter("Newbie", 5, 0),
				gameservertest.WithWantChars(1),
				gameservertest.WithSkills(skillPersistence(t, []modelskill.Definition{poleCount()})),
				gameservertest.WithItemTemplates(poleCatalog()),
				gameservertest.WithDoors(tmpl),
			)
			c, objID := srv.Client, srv.SoleObjectID(t)
			pole := srv.GiveItem(t, objID, poleWeaponID, 1)
			seedKnownSkill(t, srv, objID, poleCountSkillID, 1)
			c.Send(encodeRequestGameStart(0))
			assertFrameOpcode(t, c.Read(), serverpackets.OpcodeSSQInfo, "SSQInfo")
			assertFrameOpcode(t, c.Read(), serverpackets.OpcodeCharSelected, "CharSelected")
			c.Send(encodeEnterWorld())
			drainUntilQuiet(t, c)
			w := wire.NewPacketWriter(clientpackets.OpcodeUseItem)
			w.WriteInt32(pole)
			w.WriteInt32(0)
			c.Send(w.Bytes())
			srv.InventoryUpdates.Tick()
			drainUntilQuiet(t, c)

			gate, ok := srv.WorldObjects.Door(tmpl.ID)
			if !ok {
				t.Fatal("door not spawned")
			}
			if tc.siege {
				srv.BeginDoorSiege(t, gate.DoorID())
			}
			steadyHits(t, srv, objID)

			// The monster stands beside the door, both in reach and in
			// front of the player once it faces the monster.
			px, py, pz := srv.PlayerPosition(t, objID)
			dx, dy, _ := gate.Position()
			hostile := srv.SpawnHostileNPCAt(t, location.Location{X: dx, Y: dy + 30, Z: pz})
			if d := math.Hypot(float64(dx-px), float64(dy-py)); d > 60 {
				t.Fatalf("door %v away from the player, want it in pole reach", d)
			}
			drainUntilQuiet(t, c)

			startMelee(t, srv, objID, hostile.ObjectID())
			hp := hostile.CurrentHP()
			srv.AdvanceUntil(t, "the swing lands on the monster", func() bool { return hostile.CurrentHP() < hp || hostile.Dead() })
			drainUntilQuiet(t, c)
			if hit := gate.CurrentHP() < combatDoorMaxHP; hit != tc.siege {
				t.Fatalf("door hit by the splash = %v (HP %v), want %v", hit, gate.CurrentHP(), tc.siege)
			}
		})
	}
}
