package skills

import (
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/player"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/door"
	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
)

// A door is a creature to every offensive skill handler of the reference
// (handler/skillhandlers): the landing roll runs on it as on any target and
// a failure is answered as on a creature, while L2Skill.getEffects lands
// nothing on a door.
//   - Disablers: STUN/ROOT/SLEEP/PARALYZE/MUTE report S1_RESISTED_YOUR_S2
//     with the skill at level 1, BETRAY/AGGREDUCE_CHAR/ERASE at the cast
//     level; CONFUSION answers INVALID_TARGET to anything not a monster.
//   - Continuous: an offensive or debuff skill that fails its roll answers
//     ATTACK_FAILED.
//   - Manadam: calcMagicAffected either misses (MISSED_TARGET) or the
//     effect roll runs and the caster is told its opponent's MP fell by 0,
//     a door having no MP.

const (
	doorDebuffSkillID = 9310
	doorDebuffLevel   = 3
)

func doorDebuffSkill(skillType string) modelskill.Definition {
	return modelskill.Definition{
		ID: doorDebuffSkillID, Level: doorDebuffLevel, Activation: modelskill.ActivationActive,
		Target: modelskill.TargetOne, SkillType: skillType, Power: 80, Offensive: true,
		CastRange: 900, HitTime: doorSkillHit, StaticHitTime: true,
		Effects: []modelskill.EffectTemplate{{Name: "Debuff", Time: 10}},
	}
}

// bootDoorDebuff boots a player who knows def next to a door under siege,
// with every landing roll out of 100 answered by landing.
func bootDoorDebuff(t *testing.T, def modelskill.Definition, landing int) (*gameservertest.Server, int32, *door.Object) {
	t.Helper()
	return bootDoorDebuffAt(t, combatDoorTemplate(door.KindDoor), def, landing)
}

// bootDoorDebuffAt is bootDoorDebuff with the door built from tmpl.
func bootDoorDebuffAt(t *testing.T, tmpl *door.Template, def modelskill.Definition, landing int) (*gameservertest.Server, int32, *door.Object) {
	t.Helper()
	srv, objID, gate := bootDoorCombat(t, tmpl, []modelskill.Definition{def})
	srv.BeginDoorSiege(t, gate.DoorID())
	onPlayerQueue(t, srv, objID, func(pc *player.Character) {
		pc.SetRollSource(func(n int) int {
			if n == 100 {
				return landing
			}
			return randomRoll(n)
		})
	})
	selectTarget(t, srv.Client, gate.ObjectID())
	return srv, objID, gate
}

// castOnDoor casts def at the selected door, lets the cast run to its end,
// and returns every frame the server sent the caster after the cast start.
func castOnDoor(t *testing.T, srv *gameservertest.Server, objID int32, def modelskill.Definition, gate *door.Object) frameLog {
	t.Helper()
	c := srv.Client
	c.Send(encodeRequestMagicSkillUse(int32(def.ID), false, false))
	readCastStartFrames(t, c, objID, int32(def.ID), int32(def.Level), doorSkillHit, 0, gate.ObjectID())
	srv.AdvanceUntil(t, "cast end", func() bool { return !srv.PlayerCastingNow(t, objID) })
	return c.ReadQueued()
}

func TestDisablerOnDoorIsResistedAsOnCreature(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		skillType string
		level     int32
	}{
		{skillType: "STUN", level: 1},
		{skillType: "ERASE", level: doorDebuffLevel},
	} {
		t.Run(tc.skillType, func(t *testing.T) {
			t.Parallel()
			def := doorDebuffSkill(tc.skillType)
			srv, objID, gate := bootDoorDebuff(t, def, 99)
			srv.Client.Send(encodeRequestMagicSkillUse(int32(def.ID), false, false))
			readCastStartFrames(t, srv.Client, objID, int32(def.ID), int32(def.Level), doorSkillHit, 0, gate.ObjectID())
			if !assertResistedAt(t, srv.Client, int32(def.ID), tc.level, gate.CharacterName()) {
				t.Fatalf("%s on a door that resisted it sent no resisted message", tc.skillType)
			}
		})
	}
}

// TestDisablerLandingOnDoorLandsNothing lands a STUN roll on a door: nothing
// is reported, and the door keeps its HP.
func TestDisablerLandingOnDoorLandsNothing(t *testing.T) {
	t.Parallel()
	def := doorDebuffSkill("STUN")
	srv, objID, gate := bootDoorDebuff(t, def, 0)
	log := castOnDoor(t, srv, objID, def, gate)
	if i := log.index(isSystemMessage(serverpackets.SystemMessageS1ResistedYourS2)); i >= 0 {
		t.Fatal("a STUN that landed on a door was reported resisted")
	}
	if gate.CurrentHP() != combatDoorMaxHP {
		t.Fatalf("door HP = %v, want %d", gate.CurrentHP(), combatDoorMaxHP)
	}
}

func TestConfusionOnDoorIsInvalidTarget(t *testing.T) {
	t.Parallel()
	def := doorDebuffSkill("CONFUSION")
	srv, objID, gate := bootDoorDebuff(t, def, 0)
	log := castOnDoor(t, srv, objID, def, gate)
	if log.index(isSystemMessage(serverpackets.SystemMessageInvalidTarget)) < 0 {
		t.Fatal("CONFUSION on a door did not answer INVALID_TARGET")
	}
}

func TestDebuffOnDoorFailsAsOnCreature(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name    string
		landing int
		failed  bool
	}{
		{name: "resisted", landing: 99, failed: true},
		{name: "landed", landing: 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			def := doorDebuffSkill("DEBUFF")
			def.Debuff = true
			srv, objID, gate := bootDoorDebuff(t, def, tc.landing)
			log := castOnDoor(t, srv, objID, def, gate)
			if failed := log.index(isSystemMessage(serverpackets.SystemMessageAttackFailed)) >= 0; failed != tc.failed {
				t.Fatalf("ATTACK_FAILED sent = %v, want %v", failed, tc.failed)
			}
		})
	}
}

// TestManadamOnDoorReducesNoMP casts MANADAM with an effect at a door whose
// effect roll fails: once the cast is not missed, the caster hears the door
// resisted the skill, then that its opponent's MP fell by 0.
func TestManadamOnDoorReducesNoMP(t *testing.T) {
	t.Parallel()
	def := doorDebuffSkill("MANADAM")
	// A door with next to no M.Def lets most casts through the gaussian
	// magic-affected roll.
	tmpl := combatDoorTemplate(door.KindDoor)
	tmpl.MDef = 1
	srv, objID, gate := bootDoorDebuffAt(t, tmpl, def, 99)
	for range 40 {
		log := castOnDoor(t, srv, objID, def, gate)
		if log.index(isSystemMessage(serverpackets.SystemMessageMissedTarget)) >= 0 {
			continue
		}
		resisted := log.index(isSystemMessage(serverpackets.SystemMessageS1ResistedYourS2))
		reduced := log.index(isSystemMessage(serverpackets.SystemMessageYourOpponentsMPWasReducedByS1))
		if resisted < 0 || reduced < resisted {
			t.Fatalf("MANADAM on a door: resisted at %d, MP reduced at %d; want the resist, then the MP report", resisted, reduced)
		}
		r, _ := systemMessage(log[reduced], serverpackets.SystemMessageYourOpponentsMPWasReducedByS1)
		r.ReadInt32() // parameter count
		r.ReadInt32() // parameter type
		if mp := r.ReadInt32(); mp != 0 {
			t.Fatalf("opponent's MP reduced by %d, want 0", mp)
		}
		return
	}
	t.Fatal("MANADAM missed the door on every cast")
}
