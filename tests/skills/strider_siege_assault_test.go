package skills

import (
	"context"
	"database/sql"
	"slices"
	"strconv"
	"testing"
	"time"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/door"
	castledata "github.com/fatal10110/acis_golang/internal/gameserver/model/residence/castle"
	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/zone"
	"github.com/fatal10110/acis_golang/internal/gameserver/network"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/siege"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
	"github.com/fatal10110/acis_golang/internal/testsupport/datapack"
)

// Reference: StriderSiegeAssault.check, run by PlayerCast.canCast with the
// cast and again by useSkill as it lands, refuses, in this order, a caster
// not riding a strider (S1_CANNOT_BE_USED naming the skill), a target that
// is not a door (INVALID_TARGET), and a caster whose clan does not attack
// the castle siege in progress where it stands (S1_CANNOT_BE_USED). The
// landing strikes a live door with Formulas.calcPhysicalSkillDamage, told
// to the caster (sendDamageMessage) before the door takes it
// (reduceCurrentHp); a hit dealing nothing is ATTACK_FAILED.

const assaultSkillID int32 = 9331

const (
	assaultGateID  = 19210001
	assaultGateHP  = 10_000_000
	assaultHit     = 1080
	assaultStrider = 12526
)

// assaultSkill mirrors Siege Assault (325) without its item cost and with
// a cast range the scene's door is always in.
func assaultSkill() modelskill.Definition {
	return modelskill.Definition{
		ID: modelskill.ID(assaultSkillID), Level: 1, Activation: modelskill.ActivationActive,
		Target: modelskill.TargetOne, SkillType: "STRIDER_SIEGE_ASSAULT", Power: 15005, SoulShotBoost: 2, BaseCritRate: 15,
		CastRange: 900, EffectRange: 1000, HitTime: assaultHit, CoolTime: 720, ReuseDelay: 5000,
	}
}

// assaultSetup is the Caster's standing when it casts.
type assaultSetup struct {
	defends bool
	noSiege bool
	riding  bool
}

// bootAssault boots Gludio with one gate by the Caster, who leads Knights
// on the side setup names, Rivals on the other, Gludio's battlefield over
// both, and returns the gate.
func bootAssault(t *testing.T, setup assaultSetup) (*gameservertest.Server, int32, *door.Object) {
	t.Helper()
	gate := combatDoorTemplate(door.KindDoor)
	gate.ID, gate.Name, gate.HP = assaultGateID, "gludio_castle_outter_001", assaultGateHP
	c, err := castledata.NewCastle(castledata.CastleAttrs{ID: gludioCastleID, Alias: "gludio_castle", Name: "Gludio", Gates: []string{gate.Name}}, nil, nil, nil, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	castles, err := castledata.NewTable([]*castledata.Castle{c})
	if err != nil {
		t.Fatal(err)
	}
	zones := zone.NewIndex()
	field := zone.NewSiege(1, gateBox(t, true))
	field.ResidenceID = gludioCastleID
	zones.Add(field)
	side, rivalSide := "ATTACKER", "DEFENDER"
	if setup.defends {
		side, rivalSide = "DEFENDER", "ATTACKER"
	}
	knights, rivals := strconv.Itoa(int(socialKnightsID)), strconv.Itoa(int(socialRivalsID))
	stmts := []string{
		`INSERT INTO clan_data (clan_id, clan_name, clan_level, leader_id) SELECT ` + knights + `, 'Knights', 5, obj_Id FROM characters WHERE char_name = 'Caster'`,
		`UPDATE characters SET clanid = ` + knights + ` WHERE char_name = 'Caster'`,
		`INSERT INTO characters (account_name, obj_Id, char_name, level, clanid) VALUES ('rl', ` + strconv.Itoa(int(siegeResRivalsLead)) + `, 'Red', 40, ` + rivals + `)`,
		`INSERT INTO clan_data (clan_id, clan_name, clan_level, leader_id) VALUES (` + rivals + `, 'Rivals', 5, ` + strconv.Itoa(int(siegeResRivalsLead)) + `)`,
		`INSERT INTO siege_clans (castle_id, clan_id, type) VALUES (1, ` + knights + `, '` + side + `')`,
		`INSERT INTO siege_clans (castle_id, clan_id, type) VALUES (1, ` + rivals + `, '` + rivalSide + `')`,
	}
	srv := gameservertest.Boot(t,
		gameservertest.WithCharacter("Caster", 40, 0),
		gameservertest.WithWantChars(1),
		gameservertest.WithSkills(skillPersistence(t, []modelskill.Definition{assaultSkill()})),
		gameservertest.WithDoors(gate),
		gameservertest.WithCastles(castles),
		gameservertest.WithSieges(siege.DefaultConfig()),
		gameservertest.WithZones(zones),
		gameservertest.WithClanSeed(func(db *sql.DB) {
			for _, stmt := range stmts {
				if _, err := db.ExecContext(context.Background(), stmt); err != nil {
					t.Fatalf("seed siege clans: %v", err)
				}
			}
		}),
	)
	if !srv.DrivesClock() {
		t.Skip("the assault's hit time needs the driven clock")
	}
	objID := srv.SoleObjectID(t)
	seedKnownSkill(t, srv, objID, int(assaultSkillID), 1)
	gludio, ok := srv.Sieges.Get(gludioCastleID)
	if !ok {
		t.Fatal("no Gludio siege")
	}
	if !setup.noSiege {
		gludio.Start()
		if !gludio.InProgress() {
			t.Fatal("Gludio siege not under way")
		}
	}
	enterBesieged(t, srv.Client)
	if setup.riding {
		mountStrider(t, srv, objID)
	}
	obj, ok := srv.WorldObjects.Door(assaultGateID)
	if !ok {
		t.Fatal("gate not spawned")
	}
	return srv, objID, obj
}

// strider is a player that mounts and dismounts.
type strider interface {
	Mount(npcID, controlItemID int32) bool
	Dismount() bool
}

// mountStrider seats objID on a Wind Strider.
func mountStrider(t *testing.T, srv *gameservertest.Server, objID int32) {
	t.Helper()
	ride(t, srv, objID, func(r strider) bool { return r.Mount(assaultStrider, 1) })
}

// dismount takes objID off its mount.
func dismount(t *testing.T, srv *gameservertest.Server, objID int32) {
	t.Helper()
	ride(t, srv, objID, strider.Dismount)
}

// ride runs step on objID's queue, failing when it changes no mount.
func ride(t *testing.T, srv *gameservertest.Server, objID int32, step func(strider) bool) {
	t.Helper()
	obj, ok := srv.State.Player(objID)
	if !ok {
		t.Fatal("Caster missing from the world")
	}
	rider, ok := network.OnlineCharacter(obj)
	if !ok {
		t.Fatalf("Caster is %T", obj)
	}
	done := make(chan struct{})
	srv.PlayerQueue(t, objID).Post(func() {
		defer close(done)
		if !step(rider) {
			t.Error("the mount did not change")
		}
	})
	<-done
	drainUntilQuiet(t, srv.Client)
}

// TestStriderSiegeAssaultRefusals casts the assault from each standing
// its check refuses: each answers its message alone, casts nothing and
// leaves the gate's HP.
func TestStriderSiegeAssaultRefusals(t *testing.T) {
	t.Parallel()
	cannotBeUsed := func(t *testing.T, f []byte) {
		assertSystemMessageSkillFrame(t, f, serverpackets.SystemMessageS1CannotBeUsed, assaultSkillID, 1)
	}
	for _, tt := range []struct {
		name     string
		setup    assaultSetup
		atSelf   bool
		refusing func(*testing.T, []byte)
	}{
		{name: "on foot", setup: assaultSetup{}, refusing: cannotBeUsed},
		{name: "on foot at itself", setup: assaultSetup{}, atSelf: true, refusing: cannotBeUsed},
		{
			name: "riding at itself", setup: assaultSetup{riding: true}, atSelf: true,
			refusing: func(t *testing.T, f []byte) {
				assertStaticSystemMessage(t, f, serverpackets.SystemMessageInvalidTarget)
			},
		},
		{name: "riding defender", setup: assaultSetup{riding: true, defends: true}, refusing: cannotBeUsed},
		{name: "riding attacker before the siege", setup: assaultSetup{riding: true, noSiege: true}, refusing: cannotBeUsed},
	} {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			srv, objID, gate := bootAssault(t, tt.setup)
			c := srv.Client
			if tt.atSelf {
				selectTarget(t, c, objID)
			} else {
				selectTarget(t, c, gate.ObjectID())
			}
			c.Send(encodeRequestMagicSkillUse(assaultSkillID, false, false))
			frames := collectUntilQuiet(t, c)
			if !slices.Equal(opcodes(frames), []byte{serverpackets.OpcodeSystemMessage}) {
				t.Fatalf("refused assault answered %x, want one SystemMessage", opcodes(frames))
			}
			tt.refusing(t, frames[0])
			srv.Advance(t, 3*time.Second)
			drainUntilQuiet(t, c)
			if srv.PlayerCastingNow(t, objID) {
				t.Fatal("refused assault is in flight")
			}
			if gate.CurrentHP() != assaultGateHP {
				t.Fatalf("gate HP = %v, want %d", gate.CurrentHP(), assaultGateHP)
			}
		})
	}
}

// TestStriderSiegeAssaultStrikesGate lands the assault of a riding
// attacker on the gate: the damage is reported before the gate's new HP,
// which is its maximum less exactly the reported damage.
func TestStriderSiegeAssaultStrikesGate(t *testing.T) {
	t.Parallel()
	srv, objID, gate := bootAssault(t, assaultSetup{riding: true})
	c := srv.Client
	selectTarget(t, c, gate.ObjectID())

	c.Send(encodeRequestMagicSkillUse(assaultSkillID, false, false))
	frames := collectUntilQuiet(t, c)
	if !slices.Contains(opcodes(frames), serverpackets.OpcodeMagicSkillUse) {
		t.Fatalf("assault answered %x, want it cast", opcodes(frames))
	}
	srv.Advance(t, assaultHit*time.Millisecond)
	hit := readDoorHit(t, c, objID, gate)
	if hit.reported <= 0 || hit.hp != assaultGateHP-hit.reported {
		t.Fatalf("assault reported %d damage, gate HP %d; want positive damage taken exactly off %d", hit.reported, hit.hp, assaultGateHP)
	}
	if hit.hpFirst {
		t.Fatal("gate HP shown ahead of the assault's damage report")
	}
	if got := int32(gate.CurrentHP()); got != hit.hp {
		t.Fatalf("gate HP = %d, want %d", got, hit.hp)
	}
}

// TestStriderSiegeAssaultRechecksOnLanding dismounts the Caster while its
// assault is in flight: the landing check tells it S1_CANNOT_BE_USED and
// the gate keeps its HP.
func TestStriderSiegeAssaultRechecksOnLanding(t *testing.T) {
	t.Parallel()
	srv, objID, gate := bootAssault(t, assaultSetup{riding: true})
	c := srv.Client
	selectTarget(t, c, gate.ObjectID())

	c.Send(encodeRequestMagicSkillUse(assaultSkillID, false, false))
	frames := collectUntilQuiet(t, c)
	if !slices.Contains(opcodes(frames), serverpackets.OpcodeMagicSkillUse) {
		t.Fatalf("assault answered %x, want it cast", opcodes(frames))
	}
	dismount(t, srv, objID)
	srv.Advance(t, 3*time.Second)
	landed := collectUntilQuiet(t, c)
	refused := false
	for _, f := range landed {
		if f[0] == serverpackets.OpcodeSystemMessage && wireReader(f[1:]).ReadInt32() == serverpackets.SystemMessageS1CannotBeUsed {
			assertSystemMessageSkillFrame(t, f, serverpackets.SystemMessageS1CannotBeUsed, assaultSkillID, 1)
			refused = true
		}
	}
	if !refused {
		t.Fatalf("landing on foot answered %x, want S1_CANNOT_BE_USED", opcodes(landed))
	}
	if len(framesWith(landed, serverpackets.OpcodeDoorStatusUpdate, gate.ObjectID())) != 0 || gate.CurrentHP() != assaultGateHP {
		t.Fatalf("gate HP = %v after a refused landing, want %d", gate.CurrentHP(), assaultGateHP)
	}
}

// TestShippedSiegeAssaultSkill pins how the shipped Siege Assault (325)
// loads: a STRIDER_SIEGE_ASSAULT ONE skill, not offensive, with a base
// critical rate of 15.
func TestShippedSiegeAssaultSkill(t *testing.T) {
	t.Parallel()
	datapack.Require(t)
	def := shippedSkill(t, 325, 1)
	if def.SkillType != "STRIDER_SIEGE_ASSAULT" || def.Target != modelskill.TargetOne || def.Offensive || def.BaseCritRate != 15 {
		t.Fatalf("Siege Assault loads as %s %v offensive=%v crit=%d, want STRIDER_SIEGE_ASSAULT ONE, not offensive, crit 15",
			def.SkillType, def.Target, def.Offensive, def.BaseCritRate)
	}
}
