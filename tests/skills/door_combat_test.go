package skills

import (
	"testing"
	"time"

	gamesql "github.com/fatal10110/acis_golang/internal/gameserver/data/sql"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/door"
	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/sevensigns"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
	"github.com/fatal10110/acis_golang/internal/testsupport"
)

// Door combat follows the reference door (model/actor/instance/Door.java,
// model/actor/status/DoorStatus.java):
//   - Door.isAttackableBy: a standing door is attacked only by a playable,
//     and only during its residence's siege (a castle siege's attacker side;
//     a wall only by a siege summon). A click on a door nobody may attack is
//     no attack, and an offensive skill aimed at it is an invalid target
//     (TargetOne.meetCastConditions).
//   - Creature.onHitTimer and the damage skill handlers land their hits on
//     Door.reduceCurrentHp, which lowers HP while the siege allows it; the
//     loss is broadcast in DoorStatusUpdate.
//   - DoorStatus.getPDef/getMDef: the template defences, raised by a fifth
//     while Dawn owns the Seal of Strife and cut to 30% while Dusk does.
//   - Door.canBeHealed is false: a heal leaves a door's HP alone.

const (
	combatDoorMaxHP = 100_000
	combatDoorPDef  = 10

	doorPdamSkillID = 9301
	doorHealSkillID = 9302
	doorSkillHit    = 500
	doorSkillReuse  = 1000
)

func combatDoorTemplate(kind door.Kind) *door.Template {
	tmpl := unlockDoorTemplate(false)
	tmpl.Kind = kind
	tmpl.OpenKind = door.OpenNPC
	tmpl.HP, tmpl.PDef, tmpl.MDef = combatDoorMaxHP, combatDoorPDef, combatDoorPDef
	return tmpl
}

func doorSkill(id int32, skillType string, offensive bool, power int) modelskill.Definition {
	return modelskill.Definition{
		ID: modelskill.ID(id), Level: 1, Activation: modelskill.ActivationActive,
		Target: modelskill.TargetOne, SkillType: skillType, Power: float32(power), Offensive: offensive,
		CastRange: 900, HitTime: doorSkillHit, ReuseDelay: doorSkillReuse, StaticHitTime: true, StaticReuse: true,
	}
}

// bootDoorCombat boots one player who knows defs and stands next to the
// door tmpl, and returns the door's live object.
func bootDoorCombat(t *testing.T, tmpl *door.Template, defs []modelskill.Definition, extra ...gameservertest.Option) (*gameservertest.Server, int32, *door.Object) {
	t.Helper()
	srv := gameservertest.Boot(t, append([]gameservertest.Option{
		gameservertest.WithCharacter("Newbie", 5, 0),
		gameservertest.WithWantChars(1),
		gameservertest.WithSkills(skillPersistence(t, defs)),
		gameservertest.WithDoors(tmpl),
	}, extra...)...)
	objID := srv.SoleObjectID(t)
	for _, def := range defs {
		seedKnownSkill(t, srv, objID, int(def.ID), def.Level)
	}
	c := srv.Client
	c.Send(encodeRequestGameStart(0))
	assertFrameOpcode(t, c.Read(), serverpackets.OpcodeSSQInfo, "SSQInfo")
	assertFrameOpcode(t, c.Read(), serverpackets.OpcodeCharSelected, "CharSelected")
	c.Send(encodeEnterWorld())
	drainUntilQuiet(t, c)
	gate, ok := srv.WorldObjects.Door(tmpl.ID)
	if !ok {
		t.Fatal("door not spawned")
	}
	return srv, objID, gate
}

// steadyHits makes objID's auto-attacks land without a critical and with
// no damage spread: each hit roll lands, each critical roll fails, and
// every other roll takes the middle of its range.
func steadyHits(t *testing.T, srv *gameservertest.Server, objID int32) {
	t.Helper()
	obj, ok := srv.State.Player(objID)
	if !ok {
		t.Fatalf("world.Player(%d) missing", objID)
	}
	permille := 0
	done := make(chan struct{})
	if !srv.PlayerQueue(t, objID).Post(func() {
		defer close(done)
		obj.(interface{ SetRollSource(func(int) int) }).SetRollSource(func(n int) int {
			if n == 1000 {
				permille++
				if permille%2 == 1 {
					return 0
				}
				return n - 1
			}
			return n / 2
		})
	}) {
		t.Fatal("player queue closed")
	}
	<-done
}

// doorHit is what one landed hit on the door showed its attacker: the
// damage it reported, the HP the door's DoorStatusUpdate carried, whether
// that update came ahead of the report, and the Attack frame's damage when
// there was one (else -1).
type doorHit struct {
	reported, hp, swing int32
	hpFirst             bool
}

// readDoorHit reads until both the YOU_DID_S1_DMG and the DoorStatusUpdate
// of gate's first landed hit have arrived.
func readDoorHit(t *testing.T, c *testsupport.ScriptedClient, objID int32, gate *door.Object) doorHit {
	t.Helper()
	hit := doorHit{reported: -1, hp: -1, swing: -1}
	for end := c.Now().Add(5 * time.Second); c.Now().Before(end); {
		frame := c.ReadWithTimeout(300 * time.Millisecond)
		if frame == nil {
			continue
		}
		r := wireReader(frame[1:])
		switch frame[0] {
		case serverpackets.OpcodeAttack:
			if r.ReadInt32() != objID || hit.swing >= 0 {
				continue
			}
			if target := r.ReadInt32(); target != gate.ObjectID() {
				t.Fatalf("Attack target = %d, want the door %d", target, gate.ObjectID())
			}
			hit.swing = r.ReadInt32()
		case serverpackets.OpcodeSystemMessage:
			if r.ReadInt32() != serverpackets.SystemMessageYouDidS1Dmg || hit.reported >= 0 {
				continue
			}
			r.ReadInt32() // parameter count
			r.ReadInt32() // parameter type
			hit.reported = r.ReadInt32()
		case serverpackets.OpcodeDoorStatusUpdate:
			if r.ReadInt32() != gate.ObjectID() || hit.hp >= 0 {
				continue
			}
			r.ReadInt32() // closed
			r.ReadInt32() // damage stage
			r.ReadInt32() // show HP
			r.ReadInt32() // door id
			r.ReadInt32() // max HP
			hit.hp = r.ReadInt32()
			hit.hpFirst = hit.reported < 0
		}
		if hit.reported >= 0 && hit.hp >= 0 {
			return hit
		}
	}
	t.Fatalf("no damage report and DoorStatusUpdate within 5s: %+v", hit)
	return hit
}

// attackDoor selects gate and clicks it again, the click that attacks.
func attackDoor(t *testing.T, c *testsupport.ScriptedClient, gate *door.Object) {
	t.Helper()
	selectTarget(t, c, gate.ObjectID())
	c.Send(encodeAction(gate.ObjectID(), 0, 0, 0, false))
}

func TestAutoAttackLowersDoorHPUnderSiege(t *testing.T) {
	t.Parallel()
	srv, objID, gate := bootDoorCombat(t, combatDoorTemplate(door.KindDoor), nil)
	srv.BeginDoorSiege(t, gate.DoorID())
	steadyHits(t, srv, objID)

	attackDoor(t, srv.Client, gate)
	hit := readDoorHit(t, srv.Client, objID, gate)

	if hit.swing <= 0 || hit.reported != hit.swing {
		t.Fatalf("Attack damage %d, reported %d; want the same positive damage", hit.swing, hit.reported)
	}
	// Creature.onHitTimer reports the hit before the target takes it.
	if hit.hpFirst {
		t.Fatal("DoorStatusUpdate ahead of the hit's damage report")
	}
	if hit.hp != combatDoorMaxHP-hit.swing {
		t.Fatalf("DoorStatusUpdate HP = %d, want %d - %d", hit.hp, combatDoorMaxHP, hit.swing)
	}
	if got := int32(gate.CurrentHP()); got > hit.hp {
		t.Fatalf("door HP = %d after the hit, want at most %d", got, hit.hp)
	}
}

// TestAttackOnDoorNobodyMayAttackIsRefused pins Door.isAttackableBy on a
// click: with no siege a door may not be attacked, and under a siege a wall
// may not be attacked by a player. The click answers ActionFailed, nothing
// swings and the door keeps its HP.
func TestAttackOnDoorNobodyMayAttackIsRefused(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name  string
		kind  door.Kind
		siege bool
	}{
		{name: "door outside a siege", kind: door.KindDoor},
		{name: "wall under a siege", kind: door.KindWall, siege: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			srv, objID, gate := bootDoorCombat(t, combatDoorTemplate(tc.kind), nil)
			if tc.siege {
				srv.BeginDoorSiege(t, gate.DoorID())
			}
			c := srv.Client
			attackDoor(t, c, gate)

			assertFrameOpcode(t, c.Read(), serverpackets.OpcodeActionFailed, "ActionFailed")
			for _, frame := range c.ReadQueued() {
				if frame[0] == serverpackets.OpcodeAttack && wireReader(frame[1:]).ReadInt32() == objID {
					t.Fatal("the player swung at a door it may not attack")
				}
			}
			if gate.CurrentHP() != combatDoorMaxHP {
				t.Fatalf("door HP = %v, want %d", gate.CurrentHP(), combatDoorMaxHP)
			}
		})
	}
}

func TestDamageSkillLowersDoorHPUnderSiege(t *testing.T) {
	t.Parallel()
	def := doorSkill(doorPdamSkillID, "PDAM", true, 200)
	srv, objID, gate := bootDoorCombat(t, combatDoorTemplate(door.KindDoor), []modelskill.Definition{def})
	srv.BeginDoorSiege(t, gate.DoorID())
	c := srv.Client
	selectTarget(t, c, gate.ObjectID())

	c.Send(encodeRequestMagicSkillUse(doorPdamSkillID, false, false))
	readCastStartFrames(t, c, objID, doorPdamSkillID, 1, doorSkillHit, doorSkillReuse, gate.ObjectID())
	hit := readDoorHit(t, c, objID, gate)

	// The report and the HP each truncate: the HP left is the maximum less
	// the reported damage, or one less for the damage's fraction.
	if hit.reported <= 0 || hit.hp > combatDoorMaxHP-hit.reported || hit.hp < combatDoorMaxHP-hit.reported-1 {
		t.Fatalf("skill reported %d damage, door HP %d; want positive damage taken off %d", hit.reported, hit.hp, combatDoorMaxHP)
	}
	// Pdam lowers the target's HP, then reports the damage.
	if !hit.hpFirst {
		t.Fatal("PDAM damage report ahead of the door's new HP")
	}
	if got := int32(gate.CurrentHP()); got != hit.hp {
		t.Fatalf("door HP = %d, want %d", got, hit.hp)
	}
}

// TestDamageSkillOnDoorOutsideSiegeIsInvalidTarget pins TargetOne's door
// rule: an offensive skill on a door its caster may not attack answers
// INVALID_TARGET alone, casts nothing and leaves the HP alone.
func TestDamageSkillOnDoorOutsideSiegeIsInvalidTarget(t *testing.T) {
	t.Parallel()
	def := doorSkill(doorPdamSkillID, "PDAM", true, 200)
	srv, _, gate := bootDoorCombat(t, combatDoorTemplate(door.KindDoor), []modelskill.Definition{def})
	c := srv.Client
	selectTarget(t, c, gate.ObjectID())

	c.Send(encodeRequestMagicSkillUse(doorPdamSkillID, false, false))
	assertStaticSystemMessage(t, c.Read(), serverpackets.SystemMessageInvalidTarget)
	for _, frame := range c.ReadQueued() {
		if frame[0] == serverpackets.OpcodeMagicSkillUse || frame[0] == serverpackets.OpcodeDoorStatusUpdate {
			t.Fatalf("refused cast sent opcode %#x", frame[0])
		}
	}
	if gate.CurrentHP() != combatDoorMaxHP {
		t.Fatalf("door HP = %v, want %d", gate.CurrentHP(), combatDoorMaxHP)
	}
}

func TestHealLeavesDoorHP(t *testing.T) {
	t.Parallel()
	def := doorSkill(doorHealSkillID, "HEAL", false, 500)
	srv, objID, gate := bootDoorCombat(t, combatDoorTemplate(door.KindDoor), []modelskill.Definition{def})
	srv.BeginDoorSiege(t, gate.DoorID())
	c := srv.Client
	srv.HitDoor(t, gate.DoorID(), objID, 1000)
	drainUntilQuiet(t, c)
	selectTarget(t, c, gate.ObjectID())

	frames := castUnlock(t, srv, c, objID, def, gate.ObjectID())

	if got := framesWith(frames, serverpackets.OpcodeDoorStatusUpdate, gate.ObjectID()); len(got) != 0 {
		t.Fatalf("heal sent %d DoorStatusUpdates, want none", len(got))
	}
	if want := float64(combatDoorMaxHP - 1000); gate.CurrentHP() != want {
		t.Fatalf("door HP = %v after a heal, want %v", gate.CurrentHP(), want)
	}
}

// seedDoorStrife seeds the Seal of Strife as owned by cabal.
func seedDoorStrife(t *testing.T, cabal sevensigns.Cabal) gameservertest.Option {
	return gameservertest.WithSevenSignsSeed(func(store *gamesql.SevenSignsStore) {
		row, found, err := store.LoadStatus(t.Context())
		if err != nil || !found {
			t.Fatalf("load status row: found=%v err=%v", found, err)
		}
		row.SealOwners = [3]sevensigns.Cabal{sevensigns.NoCabal, sevensigns.NoCabal, cabal}
		if err := store.SaveStatus(t.Context(), row); err != nil {
			t.Fatalf("seed status row: %v", err)
		}
	})
}

// TestDoorHitsFollowSealOfStrife lands the same steady first hit on a door
// under each Seal of Strife owner: the door's P.Def is 12 under Dawn, 10
// unowned and 3 under Dusk, so the hit grows as the defence falls.
func TestDoorHitsFollowSealOfStrife(t *testing.T) {
	t.Parallel()
	hits := map[sevensigns.Cabal]int32{}
	for _, owner := range []sevensigns.Cabal{sevensigns.Dawn, sevensigns.NoCabal, sevensigns.Dusk} {
		t.Run(owner.String(), func(t *testing.T) {
			srv, objID, gate := bootDoorCombat(t, combatDoorTemplate(door.KindDoor), nil, seedDoorStrife(t, owner))
			srv.BeginDoorSiege(t, gate.DoorID())
			steadyHits(t, srv, objID)
			attackDoor(t, srv.Client, gate)
			hit := readDoorHit(t, srv.Client, objID, gate)
			if hit.hp != combatDoorMaxHP-hit.reported {
				t.Fatalf("door HP %d after a %d hit, want %d", hit.hp, hit.reported, combatDoorMaxHP-hit.reported)
			}
			hits[owner] = hit.reported
		})
	}
	dawn, unowned, dusk := hits[sevensigns.Dawn], hits[sevensigns.NoCabal], hits[sevensigns.Dusk]
	if dawn <= 0 || dawn >= unowned || unowned >= dusk {
		t.Fatalf("hits under Dawn %d, unowned %d, Dusk %d; want 0 < Dawn < unowned < Dusk", dawn, unowned, dusk)
	}
}
