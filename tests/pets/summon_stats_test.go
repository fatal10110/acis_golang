package pets

import (
	"context"
	"encoding/binary"
	"math"
	"testing"

	"github.com/fatal10110/acis_golang/internal/commons/wire"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/creature"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/npc"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/admin"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/item"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/itemcontainer"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
)

// longReachSiegeGolem is the Siege Golem template with the 80 attack range
// and the random damage its npc data carries, neither of which a summon
// uses.
func longReachSiegeGolem() *npc.Template {
	tmpl := siegeGolemTemplate()
	tmpl.BaseAttackRange = 80
	tmpl.BaseRandomDamage = 30
	return tmpl
}

// TestSiegeGolemChasesToItsFistRange pins a summon's physical attack range
// (CreatureStatus.getPhysicalAttackRange, the attack type's range, which
// SummonStatus keeps): a Siege Golem, whose npc data reads 80, chases a
// monster to the first step within 40 of it, as every summon does.
func TestSiegeGolemChasesToItsFistRange(t *testing.T) {
	t.Parallel()
	h, golem, _ := bootFearServitor(t, longReachSiegeGolem(), summonGolemSkillID,
		gameservertest.WithZones(battlefieldZones(t, -1_000, 1_000, true)))
	if got := golem.PhysicalAttackRange(); got != 40 {
		t.Fatalf("siege golem PhysicalAttackRange() = %d, want 40", got)
	}
	gx, gy, gz := golem.Position()
	target := location.Location{X: gx + 600, Y: gy, Z: gz}
	hostile := h.srv.SpawnHostileNPCAt(t, target)
	drainUntilQuiet(t, h.client)

	h.commandAt(t, hostile.ObjectID(), target, servitorAttackAction, false)
	drainUntilQuiet(t, h.client)
	if !golem.IsMoving() {
		t.Fatal("siege golem did not start chasing the out-of-range monster")
	}
	for i := 0; golem.IsMoving(); i++ {
		if i == 300 {
			t.Fatal("chase still under way after 30 s of position updates")
		}
		h.srv.TickPositions()
	}

	x, y, z := golem.Position()
	pos := location.Location{X: x, Y: y, Z: z}
	const reach = 40
	step := int(golem.Move().Speed()/10) + 1
	if !pos.In2DRadius(target, reach) || pos.In2DRadius(target, reach-step) {
		t.Fatalf("chase ended at %+v, %.1f from the monster; want the first step within the attack range %d",
			pos, pos.Distance2D(target), reach)
	}
	drainUntilQuiet(t, h.client)
}

// TestSummonHitRandomDamage pins the random damage multiplier of a summon's
// hit (Creature.getRandomDamageMultiplier, 1 + Rnd.get(-r, r)/100) on summons
// called through the real spawn paths: a Siege Golem, holding no weapon,
// spreads 5+sqrt(level), not its npc data's 30; a wolf holding a weapon
// spreads that weapon's random damage. Every roll is pinned to its lowest,
// so the multiplier is 1 - r/100.
func TestSummonHitRandomDamage(t *testing.T) {
	t.Parallel()
	lowest := func(int) int { return 0 }

	t.Run("siege golem", func(t *testing.T) {
		t.Parallel()
		_, golem, _ := bootFearServitor(t, longReachSiegeGolem(), summonGolemSkillID,
			gameservertest.WithZones(battlefieldZones(t, -1_000, 1_000, true)))
		var spread int
		var mul float64
		runOnPetQueue(t, golem, func() {
			golem.SetRollSource(lowest)
			spread = golem.RandomDamageSpread()
			mul = creature.RandomDamageMultiplier(golem, modelskill.Definition{})
		})
		// Level 60: 5 + (int)sqrt(60) = 12.
		if spread != -1 || math.Abs(mul-0.88) > 1e-9 {
			t.Fatalf("siege golem spread/multiplier = %d/%v, want -1/0.88", spread, mul)
		}
	})

	t.Run("wolf holding a weapon", func(t *testing.T) {
		t.Parallel()
		weapon := petWeaponTemplate()
		weapon.Weapon.RandomDamage = 10
		catalog := item.NewTable(append(gameservertest.ItemTemplates().All(), weapon))
		h := bootOwnerWithCollarOpts(t, []gameservertest.Option{gameservertest.WithItemTemplates(catalog)})
		wolf, _ := h.spawnWolf(t)
		var unarmed, spread int
		var mul float64
		runOnPetQueue(t, wolf, func() {
			unarmed = wolf.RandomDamageSpread()
			inv := wolf.PetInventory()
			inst := inv.AddNew(fixturePetWeaponID, 1, 1_900_000)
			tmpl, _ := inv.Templates().Get(fixturePetWeaponID)
			inv.SetPaperdollItem(itemcontainer.RHand, inst, tmpl)
			wolf.SetRollSource(lowest)
			spread = wolf.RandomDamageSpread()
			mul = creature.RandomDamageMultiplier(wolf, modelskill.Definition{})
		})
		if unarmed != -1 {
			t.Fatalf("unarmed wolf spread = %d, want -1 (the level fallback)", unarmed)
		}
		if spread != 10 || math.Abs(mul-0.90) > 1e-9 {
			t.Fatalf("armed wolf spread/multiplier = %d/%v, want 10/0.9", spread, mul)
		}
	})
}

// npcInfoTitle reads the title of the NpcInfo of objectID among frames.
func npcInfoTitle(t *testing.T, frames [][]byte, objectID int32) string {
	t.Helper()
	for _, f := range frames {
		if f[0] != serverpackets.OpcodeNPCInfo || int32(binary.LittleEndian.Uint32(f[1:5])) != objectID {
			continue
		}
		// The name string starts at 1+121 (see TestObserverSeesWhatAPetWears);
		// the title follows it.
		r := wire.NewReader(f[1+121:])
		r.ReadString()
		title := r.ReadString()
		if err := r.Err(); err != nil {
			t.Fatalf("read NpcInfo: %v", err)
		}
		return title
	}
	t.Fatalf("no NpcInfo of %d among % x", objectID, opcodes(frames))
	return ""
}

// TestPetTitleIsTheOwnersNameAtSummoning pins when a pet takes its title,
// the owner's name: once, as it is called out (SummonItems sets it on the new
// pet). A GM renaming the owner leaves the old name on the owner's PetInfo
// and PetStatusUpdate until the pet is called out again. Others' NpcInfo of
// the pet (SummonInfo) reads the owner's current name instead.
func TestPetTitleIsTheOwnersNameAtSummoning(t *testing.T) {
	t.Parallel()
	const gmLevel = 7
	adminData, err := admin.NewData([]admin.AccessLevel{{Level: 0}, {Level: gmLevel, IsGM: true, GiveDamage: true}},
		[]admin.Command{{Name: "admin_set", AccessLevel: gmLevel}})
	if err != nil {
		t.Fatal(err)
	}
	srv := bootPets(t, gameservertest.WithAdmin(adminData))
	ownerID := srv.SoleObjectID(t)
	if _, err := srv.DB.ExecContext(context.Background(), "UPDATE characters SET accesslevel = ? WHERE obj_Id = ?", gmLevel, ownerID); err != nil {
		t.Fatalf("set access level: %v", err)
	}
	collarID := srv.GiveItem(t, ownerID, wolfCollarID, 1)
	startInWorld(t, srv.Client)
	h := &petWorld{srv: srv, client: srv.Client, ownerID: ownerID, collarID: collarID, seeded: map[int32][]int32{}}
	oldName := h.character(t).CharacterName()
	const newName = "Renamed7"
	wolf, _ := h.spawnWolf(t)

	h.client.Send(encodeBuildCmd("set name " + newName))
	h.srv.AdvanceUntil(t, "owner renamed", func() bool { return h.character(t).CharacterName() == newName })
	drainUntilQuiet(t, h.client)

	renamed := renameTo(t, h, "Fenrir")
	if _, title := petInfoNameTitle(t, mustFirstOpcode(t, renamed, serverpackets.OpcodePetInfo, "pet rename PetInfo")); title != oldName {
		t.Fatalf("PetInfo title after the owner's rename = %q, want the name at summoning %q", title, oldName)
	}
	status := hurtAndReadStatus(t, h.srv, h.client, h.ownerID, wolf)
	if got := petStatusTitle(t, mustFirstOpcode(t, status, serverpackets.OpcodePetStatusUpdate, "PetStatusUpdate")); got != oldName {
		t.Fatalf("PetStatusUpdate title after the owner's rename = %q, want %q", got, oldName)
	}
	if got := npcInfoTitle(t, enterWorldSeeing(t, h.srv, "player2", "Late"), wolf.ObjectID()); got != newName {
		t.Fatalf("watcher NpcInfo title = %q, want the owner's current name %q", got, newName)
	}

	h.returnPet(t)
	_, burst := h.spawnWolf(t)
	if _, title := petInfoNameTitle(t, mustFirstOpcode(t, burst, serverpackets.OpcodePetInfo, "resummon PetInfo")); title != newName {
		t.Fatalf("resummoned PetInfo title = %q, want %q", title, newName)
	}
}
