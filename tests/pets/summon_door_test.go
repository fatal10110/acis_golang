package pets

import (
	"testing"

	"github.com/fatal10110/acis_golang/internal/commons/wire"
	gamesql "github.com/fatal10110/acis_golang/internal/gameserver/data/sql"
	"github.com/fatal10110/acis_golang/internal/gameserver/data/sql/sqltest"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/npc"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/summon"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/door"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	skillstate "github.com/fatal10110/acis_golang/internal/gameserver/skill"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
)

// Reference: a door is a Creature, so RequestActionUse's pet attack
// (16/22) attacks a door its owner may attack (Door.isAttackableBy accepts
// any Playable during the residence's siege) and follows it otherwise;
// CreatureAttack.canAttack lets the summon's own swing through while
// door.isAttackableBy(summon) holds, and Door.reduceCurrentHp lowers the
// door's HP. Actions 41 (Wild Hog Cannon, skill 4230) and 1000 (Siege
// Golem, skill 4079) answer INVALID_TARGET to any target but a door and
// cast on one; actions 1039 and 1040 (Swoop Cannon, skills 5110 and 5111)
// answer INVALID_TARGET to a door.

const (
	petDoorID    = 24_500_001
	petDoorX     = -80
	petDoorMaxHP = 100_000

	wildCannonSkill = 4230
	swoopFodder     = 5110
	wildCannonHit   = 500
)

func petDoorTemplate() *door.Template {
	return &door.Template{
		ID: petDoorID, Name: "pet_gate", Kind: door.KindDoor, Level: 1,
		Position: location.Location{X: petDoorX, Y: 0, Z: hostileZ},
		Coordinates: []location.Point{
			{X: petDoorX - 8, Y: -8}, {X: petDoorX + 8, Y: -8}, {X: petDoorX + 8, Y: 8}, {X: petDoorX - 8, Y: 8},
		},
		HP: petDoorMaxHP, PDef: 10, MDef: 10, Height: 32, OpenKind: door.OpenNPC,
	}
}

// siegeShot is a commanded cannon shot: a single-target physical skill.
func siegeShot(id int) modelskill.Definition {
	return modelskill.Definition{
		ID: modelskill.ID(id), Level: 1, Activation: modelskill.ActivationActive,
		Target: modelskill.TargetOne, Offensive: true, SkillType: "PDAM",
		CastRange: 900, HitTime: wildCannonHit, StaticHitTime: true, Power: 1000,
	}
}

// bootPetDoor brings the owner in beside a door with a wolf that knows
// shots, summoned and standing by.
func bootPetDoor(t *testing.T, shots ...modelskill.Definition) (*petWorld, *summon.Actor, *door.Object) {
	t.Helper()
	wolf := wolfTemplate()
	wolf.Skills = map[int]int{}
	defs := []modelskill.Definition{{
		ID: summonCreatureID, Level: 1, Activation: modelskill.ActivationActive, Target: modelskill.TargetSelf,
		SkillType: "SUMMON_CREATURE", StaticHitTime: true, StaticReuse: true,
	}}
	for _, shot := range shots {
		wolf.Skills[int(shot.ID)] = shot.Level
		defs = append(defs, shot)
	}
	db := sqltest.SharedDB(t)
	skills := skillstate.NewPersistence(gamesql.NewSkillSaveStore(db), modelskill.NewTable(defs), gamesql.NewCharacterSkillStore(db))
	srv := bootPets(t,
		gameservertest.WithNPCs(npc.NewTable([]*npc.Template{wolf, treeTemplate()})),
		gameservertest.WithSkills(skills),
		gameservertest.WithDoors(petDoorTemplate()),
	)
	ownerID := srv.SoleObjectID(t)
	h := &petWorld{srv: srv, client: srv.Client, ownerID: ownerID, collarID: srv.GiveItem(t, ownerID, wolfCollarID, 1)}
	// The door's status joins the entry burst, so the burst is drained
	// rather than read frame by frame.
	h.client.Send(encodeRequestGameStart(0))
	assertFrameOpcode(t, mustRead(t, h.client, "SSQInfo"), serverpackets.OpcodeSSQInfo, "SSQInfo")
	assertFrameOpcode(t, mustRead(t, h.client, "CharSelected"), serverpackets.OpcodeCharSelected, "CharSelected")
	h.client.Send(encodeEnterWorld())
	drainUntilQuiet(t, h.client)
	petActor, _ := h.spawnWolf(t)
	gate, ok := h.srv.WorldObjects.Door(petDoorID)
	if !ok {
		t.Fatal("door not spawned")
	}
	drainUntilQuiet(t, h.client)
	return h, petActor, gate
}

// selectDoor makes the owner select gate.
func (h *petWorld) selectDoor(t *testing.T, gate *door.Object) {
	t.Helper()
	x, y, z := gate.Position()
	h.client.Send(encodeAction(gate.ObjectID(), int32(x), int32(y), int32(z), false))
	drainUntilQuiet(t, h.client)
}

// petCastAt reports whether frames hold the pet's MagicSkillUse of skill
// onto targetID.
func petCastAt(frames [][]byte, petActor *summon.Actor, skill, targetID int32) bool {
	for _, frame := range frames {
		if frame[0] != serverpackets.OpcodeMagicSkillUse {
			continue
		}
		r := wire.NewReader(frame[1:])
		if caster, target, id := r.ReadInt32(), r.ReadInt32(), r.ReadInt32(); caster == petActor.ObjectID() && target == targetID && id == skill {
			return true
		}
	}
	return false
}

func TestPetAttackCommandHitsDoorUnderSiege(t *testing.T) {
	t.Parallel()
	h, petActor, gate := bootPetDoor(t)
	h.srv.BeginDoorSiege(t, gate.DoorID())
	h.selectDoor(t, gate)

	h.client.Send(encodeRequestActionUse(petAttackAction, false))
	h.srv.AdvanceUntil(t, "the pet's hits lower the door's HP", func() bool {
		return gate.CurrentHP() < petDoorMaxHP
	})
	if got := petActor.Intent(); got != summon.IntentAttackTarget {
		t.Fatalf("pet intent = %v, want attack-target", got)
	}
}

// TestPetAttackCommandFollowsDoorOutsideSiege: a door nobody may attack is
// followed, never hit.
func TestPetAttackCommandFollowsDoorOutsideSiege(t *testing.T) {
	t.Parallel()
	h, petActor, gate := bootPetDoor(t)
	h.selectDoor(t, gate)

	h.client.Send(encodeRequestActionUse(petAttackAction, false))
	drainUntilQuiet(t, h.client)
	if got := petActor.Intent(); got != summon.IntentFollowTarget {
		t.Fatalf("pet intent = %v, want follow-target", got)
	}
	if gate.CurrentHP() != petDoorMaxHP {
		t.Fatalf("door HP = %v, want %d", gate.CurrentHP(), petDoorMaxHP)
	}
}

func TestDoorOnlyActionShootsDoor(t *testing.T) {
	t.Parallel()
	h, petActor, gate := bootPetDoor(t, siegeShot(wildCannonSkill))
	h.srv.BeginDoorSiege(t, gate.DoorID())
	h.selectDoor(t, gate)

	h.client.Send(encodeRequestActionUse(41, false))
	frames := drainFrames(t, h.client)
	if !petCastAt(frames, petActor, wildCannonSkill, gate.ObjectID()) {
		t.Fatalf("no cannon shot at the door: opcodes %x", frameOpcodes(frames))
	}
	h.srv.AdvanceUntil(t, "the shot lowers the door's HP", func() bool {
		return gate.CurrentHP() < petDoorMaxHP
	})
}

// TestDoorOnlyActionRefusesOtherTargets answers INVALID_TARGET to a
// monster or no target at all, and shoots nothing.
func TestDoorOnlyActionRefusesOtherTargets(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name    string
		monster bool
	}{{name: "monster", monster: true}, {name: "no target"}} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			h, petActor, _ := bootPetDoor(t, siegeShot(wildCannonSkill))
			if tc.monster {
				hostile := h.srv.SpawnHostileNPC(t)
				drainUntilQuiet(t, h.client)
				h.client.Send(encodeAction(hostile.ObjectID(), hostileX, hostileY, hostileZ, false))
				drainUntilQuiet(t, h.client)
			}
			h.client.Send(encodeRequestActionUse(41, false))
			frames := drainFrames(t, h.client)
			if _, ok := firstOpcode(frames, serverpackets.OpcodeMagicSkillUse); ok {
				t.Fatalf("refused shot started a cast: opcodes %x", frameOpcodes(frames))
			}
			if len(frames) == 0 || frames[0][0] != serverpackets.OpcodeSystemMessage || systemMessageID(t, frames[0]) != serverpackets.SystemMessageInvalidTarget {
				t.Fatalf("refused shot answered %x, want INVALID_TARGET first", frameOpcodes(frames))
			}
			if petActor.CastingNow() {
				t.Fatal("pet casting after a refused shot")
			}
		})
	}
}

// TestSwoopCannonActionRefusesDoor answers INVALID_TARGET to a door and
// shoots nothing.
func TestSwoopCannonActionRefusesDoor(t *testing.T) {
	t.Parallel()
	h, _, gate := bootPetDoor(t, siegeShot(swoopFodder))
	h.srv.BeginDoorSiege(t, gate.DoorID())
	h.selectDoor(t, gate)

	h.client.Send(encodeRequestActionUse(1039, false))
	frames := drainFrames(t, h.client)
	if _, ok := firstOpcode(frames, serverpackets.OpcodeMagicSkillUse); ok {
		t.Fatalf("Swoop Cannon shot a door: opcodes %x", frameOpcodes(frames))
	}
	if len(frames) == 0 || frames[0][0] != serverpackets.OpcodeSystemMessage || systemMessageID(t, frames[0]) != serverpackets.SystemMessageInvalidTarget {
		t.Fatalf("Swoop Cannon on a door answered %x, want INVALID_TARGET first", frameOpcodes(frames))
	}
	if gate.CurrentHP() != petDoorMaxHP {
		t.Fatalf("door HP = %v, want %d", gate.CurrentHP(), petDoorMaxHP)
	}
}
