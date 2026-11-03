package skills

import (
	"math"
	"testing"
	"time"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/attackable"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/player"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
)

const casterTailSkill = modelskill.ID(9341)

// casterTailAura is an offensive aura of skillType around its caster, as
// wide as radius. 4139 Boom Attack is an MDAM aura of 150.
func casterTailAura(skillType string, radius int) modelskill.Definition {
	return modelskill.Definition{
		ID: casterTailSkill, Level: 1, Activation: modelskill.ActivationActive, Target: modelskill.TargetAura,
		Radius: radius, Offensive: true, SkillType: skillType, Power: 10, HitTime: 500, StaticHitTime: true,
	}
}

// TestSuicideAttackKillsItsCaster (Pdam.useSkill, Mdam.useSkill: a suicide
// attack ends with creature.doDie(creature)): a monster's suicide aura kills
// the monster whether or not it reached anyone. The same aura without the
// flag leaves the monster standing. The death cancels the cast's
// animation before the monster falls.
func TestSuicideAttackKillsItsCaster(t *testing.T) {
	t.Parallel()
	for _, tt := range []struct {
		name      string
		skillType string
		radius    int
		suicide   bool
	}{
		{name: "MDAM reaching the player", skillType: "MDAM", radius: 150, suicide: true},
		{name: "MDAM on nobody", skillType: "MDAM", radius: 10, suicide: true},
		{name: "PDAM reaching the player", skillType: "PDAM", radius: 150, suicide: true},
		{name: "PDAM on nobody", skillType: "PDAM", radius: 10, suicide: true},
		{name: "MDAM without the flag", skillType: "MDAM", radius: 150},
		{name: "PDAM without the flag", skillType: "PDAM", radius: 150},
	} {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			def := casterTailAura(tt.skillType, tt.radius)
			def.SuicideAttack = tt.suicide
			srv, caster, cast := bootRealDamageCaster(t, def)

			cast(caster, casterTailSkill)
			if !tt.suicide {
				srv.Advance(t, 2*time.Second)
				drainUntilQuiet(t, srv.Client)
				if caster.Dead() {
					t.Fatal("the caster of a plain aura died")
				}
				return
			}
			srv.AdvanceUntil(t, "the suicide attacker dying", caster.Dead)
			if hp := caster.HP(); hp != 0 {
				t.Fatalf("suicide attacker HP = %v, want 0", hp)
			}
			// The death aborts the cast still in flight before the caster
			// falls (Creature.doDie: abortAll, then the AI's DEAD event).
			canceled := readUntil(t, srv.Client, serverpackets.OpcodeMagicSkillCanceled)
			if id := wireReader(canceled[1:]).ReadInt32(); id != caster.ObjectID() {
				t.Fatalf("MagicSkillCanceled for %d, want the suicide attacker %d", id, caster.ObjectID())
			}
			readDieOf(t, srv.Client, caster.ObjectID())
		})
	}
}

// chargeFrames is what a CHARGE skill's caster shows its observers: the
// FlyToLocation it charges with and the ValidateLocation that follows.
type chargeFrames struct {
	dest, from location.Location
	flight     int32
	validated  location.Location
}

// readCharge reads the caster's FlyToLocation and the next ValidateLocation
// broadcast for it.
func readCharge(t *testing.T, c interface{ Read() []byte }, objID int32) chargeFrames {
	t.Helper()
	var got chargeFrames
	for {
		frame := readUntil(t, c, serverpackets.OpcodeFlyToLocation)
		r := wireReader(frame[1:])
		if r.ReadInt32() != objID {
			continue
		}
		got.dest = location.Location{X: int(r.ReadInt32()), Y: int(r.ReadInt32()), Z: int(r.ReadInt32())}
		got.from = location.Location{X: int(r.ReadInt32()), Y: int(r.ReadInt32()), Z: int(r.ReadInt32())}
		got.flight = r.ReadInt32()
		break
	}
	for {
		frame := readUntil(t, c, serverpackets.OpcodeValidateLocation)
		r := wireReader(frame[1:])
		if r.ReadInt32() != objID {
			continue
		}
		got.validated = location.Location{X: int(r.ReadInt32()), Y: int(r.ReadInt32()), Z: int(r.ReadInt32())}
		return got
	}
}

// chargePoint is the reference's charge landing: distance units from
// (x, y) along heading (MathUtil.getNewLocationByDistanceAndHeading), each
// axis offset truncated.
func chargePoint(x, y, z, heading, distance int) location.Location {
	radians := float64(heading) / 182.04444444444444 * math.Pi / 180
	return location.Location{X: x + int(float64(distance)*math.Cos(radians)), Y: y + int(float64(distance)*math.Sin(radians)), Z: z}
}

// TestChargeSkillMovesItsCaster (Pdam.useSkill, FlyType.CHARGE): after its
// hit, a CHARGE skill's caster flies flyRadius units toward its first
// target, or along its own heading when the skill reached nobody, then sits
// at the landing point. Observers see FlyToLocation from the old position
// with the CHARGE type, then ValidateLocation at the landing point. A PDAM
// skill without the fly type leaves its caster in place.
func TestChargeSkillMovesItsCaster(t *testing.T) {
	t.Parallel()
	const flyRadius, heading = 400, 10_000
	charge := modelskill.FlightCharge
	for _, tt := range []struct {
		name      string
		targetOne bool
		flight    *modelskill.Flight
	}{
		{name: "toward its target", targetOne: true, flight: &charge},
		{name: "along its heading with no target", flight: &charge},
		{name: "plain PDAM stays put", targetOne: true},
	} {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			def := casterTailAura("PDAM", 10)
			if tt.targetOne {
				def.Target, def.Radius, def.CastRange = modelskill.TargetOne, 0, 900
			}
			def.Flight, def.FlyRadius = tt.flight, flyRadius
			srv, caster, cast := bootRealDamageCaster(t, def)
			objID := srv.SoleObjectID(t)
			var px, py int
			onPlayerQueue(t, srv, objID, func(pc *player.Character) { px, py, _ = pc.Position() })
			onNPCQueue(t, caster, func() { caster.SetHeading(heading) })
			sx, sy, sz := caster.Position()

			var target attackable.Combatant = caster
			want := chargePoint(sx, sy, sz, heading, flyRadius)
			if tt.targetOne {
				target = worldCombatant(t, srv, objID)
				want = chargePoint(sx, sy, sz, location.Location{X: sx, Y: sy}.HeadingTo(location.Location{X: px, Y: py}), flyRadius)
			}
			cast(target, casterTailSkill)

			if tt.flight == nil {
				srv.Advance(t, 2*time.Second)
				drainUntilQuiet(t, srv.Client)
				if x, y, z := caster.Position(); (location.Location{X: x, Y: y, Z: z}) != (location.Location{X: sx, Y: sy, Z: sz}) {
					t.Fatalf("plain PDAM moved its caster to (%d,%d,%d), want it at (%d,%d,%d)", x, y, z, sx, sy, sz)
				}
				return
			}
			srv.Advance(t, 2*time.Second)
			got := readCharge(t, srv.Client, caster.ObjectID())
			if got.dest != want || got.validated != want {
				t.Fatalf("charge landing = FlyToLocation %+v, ValidateLocation %+v, want %+v", got.dest, got.validated, want)
			}
			if from := (location.Location{X: sx, Y: sy, Z: sz}); got.from != from {
				t.Fatalf("FlyToLocation origin = %+v, want the pre-charge position %+v", got.from, from)
			}
			if got.flight != int32(modelskill.FlightCharge) {
				t.Fatalf("FlyToLocation type = %d, want CHARGE (%d)", got.flight, modelskill.FlightCharge)
			}
			if x, y, z := caster.Position(); (location.Location{X: x, Y: y, Z: z}) != want {
				t.Fatalf("caster position after the charge = (%d,%d,%d), want %+v", x, y, z, want)
			}
		})
	}
}
