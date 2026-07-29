package cast

import (
	"testing"

	skilltarget "github.com/fatal10110/acis_golang/internal/gameserver/handler/target"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/attackable"
	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
)

// launchActor is a minimal fixture implementing every optional surface
// RevalidateLaunch's gates consult, each independently controllable so
// tests can isolate one gate at a time.
type launchActor struct {
	id          int32
	x, y, z     int
	category    skilltarget.Category
	radius      float64
	sees        bool
	knows       bool
	inPeaceZone bool
}

func (a *launchActor) ObjectID() int32                        { return a.id }
func (a *launchActor) Position() (int, int, int)              { return a.x, a.y, a.z }
func (a *launchActor) Heading() int                           { return 0 }
func (a *launchActor) Dead() bool                             { return false }
func (a *launchActor) Category() skilltarget.Category         { return a.category }
func (a *launchActor) CollisionRadius() float64               { return a.radius }
func (a *launchActor) SiegeGuard() bool                       { return false }
func (a *launchActor) AlikeDead() bool                        { return false }
func (a *launchActor) CanSeeTarget(skilltarget.Creature) bool { return a.sees }
func (a *launchActor) Knows(attackable.Combatant) bool        { return a.knows }
func (a *launchActor) EffectRangeInPeaceZone(x, y, z, effectRange int) bool {
	return a.inPeaceZone
}

func TestRevalidateLaunchSelfTargetSkipsEveryGate(t *testing.T) {
	caster := &launchActor{id: 1, knows: false, inPeaceZone: true, sees: false}
	def := modelskill.Definition{Offensive: true, Radius: 900, EffectRange: 50}

	if got := RevalidateLaunch(caster, caster, def); got != LaunchAbortNone {
		t.Fatalf("RevalidateLaunch(self) = %v, want LaunchAbortNone", got)
	}
}

func TestRevalidateLaunchTargetLost(t *testing.T) {
	caster := &launchActor{id: 1, knows: false}
	target := &launchActor{id: 2}
	def := modelskill.Definition{SkillType: "PDAM"}

	if got := RevalidateLaunch(caster, target, def); got != LaunchAbortTargetLost {
		t.Fatalf("RevalidateLaunch() = %v, want LaunchAbortTargetLost", got)
	}
}

func TestRevalidateLaunchSummonFriendBypassesTargetLost(t *testing.T) {
	caster := &launchActor{id: 1, knows: false}
	target := &launchActor{id: 2}
	def := modelskill.Definition{SkillType: "SUMMON_FRIEND"}

	if got := RevalidateLaunch(caster, target, def); got != LaunchAbortNone {
		t.Fatalf("RevalidateLaunch(SUMMON_FRIEND) = %v, want LaunchAbortNone", got)
	}
}

func TestRevalidateLaunchEscapeRange(t *testing.T) {
	tests := []struct {
		name string
		def  modelskill.Definition
		dist int
		want LaunchAbortReason
	}{
		{
			name: "effect range too far",
			def:  modelskill.Definition{EffectRange: 100},
			dist: 200,
			want: LaunchAbortTooFar,
		},
		{
			name: "effect range within",
			def:  modelskill.Definition{EffectRange: 100},
			dist: 50,
			want: LaunchAbortNone,
		},
		{
			name: "radius fallback when no cast range and radius over 80",
			def:  modelskill.Definition{CastRange: 0, Radius: 200},
			dist: 300,
			want: LaunchAbortTooFar,
		},
		{
			name: "no escape range when cast range is set",
			def:  modelskill.Definition{CastRange: 40, Radius: 200},
			dist: 10000,
			want: LaunchAbortNone,
		},
		{
			name: "no escape range when radius is default 80",
			def:  modelskill.Definition{Radius: 80},
			dist: 10000,
			want: LaunchAbortNone,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			caster := &launchActor{id: 1, knows: true, sees: true}
			target := &launchActor{id: 2, x: tt.dist, knows: true, sees: true}
			if got := RevalidateLaunch(caster, target, tt.def); got != tt.want {
				t.Fatalf("RevalidateLaunch() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestRevalidateLaunchEscapeRangeIncludesCollisionRadii(t *testing.T) {
	caster := &launchActor{id: 1, knows: true, sees: true, radius: 20}
	target := &launchActor{id: 2, x: 115, knows: true, sees: true, radius: 20}
	def := modelskill.Definition{EffectRange: 100}

	// Raw distance (115) exceeds the range (100), but the actors' combined
	// collision radii (40) cover the gap, matching MathUtil.checkIfInRange.
	if got := RevalidateLaunch(caster, target, def); got != LaunchAbortNone {
		t.Fatalf("RevalidateLaunch() = %v, want LaunchAbortNone (collision radii close the gap)", got)
	}
}

func TestRevalidateLaunchLineOfSight(t *testing.T) {
	caster := &launchActor{id: 1, knows: true, sees: false}
	target := &launchActor{id: 2}

	blocked := modelskill.Definition{Radius: 100}
	if got := RevalidateLaunch(caster, target, blocked); got != LaunchAbortNoLineOfSight {
		t.Fatalf("RevalidateLaunch(radius>0, blocked) = %v, want LaunchAbortNoLineOfSight", got)
	}

	noRadius := modelskill.Definition{Radius: 0}
	if got := RevalidateLaunch(caster, target, noRadius); got != LaunchAbortNone {
		t.Fatalf("RevalidateLaunch(radius=0, blocked LOS) = %v, want LaunchAbortNone (gate skipped)", got)
	}
}

func TestRevalidateLaunchPeaceZone(t *testing.T) {
	def := modelskill.Definition{Offensive: true, Radius: 0}

	casterInZone := &launchActor{id: 1, knows: true, sees: true, category: skilltarget.CategoryPlayable, inPeaceZone: true}
	target := &launchActor{id: 2, category: skilltarget.CategoryPlayable}
	if got := RevalidateLaunch(casterInZone, target, def); got != LaunchAbortCasterPeaceZone {
		t.Fatalf("RevalidateLaunch(caster in peace zone) = %v, want LaunchAbortCasterPeaceZone", got)
	}

	caster := &launchActor{id: 1, knows: true, sees: true, category: skilltarget.CategoryPlayable}
	targetInZone := &launchActor{id: 2, category: skilltarget.CategoryPlayable, inPeaceZone: true}
	if got := RevalidateLaunch(caster, targetInZone, def); got != LaunchAbortTargetPeaceZone {
		t.Fatalf("RevalidateLaunch(target in peace zone) = %v, want LaunchAbortTargetPeaceZone", got)
	}
}

func TestRevalidateLaunchPeaceZoneOnlyGatesOffensivePlayableVsPlayable(t *testing.T) {
	caster := &launchActor{id: 1, knows: true, sees: true, category: skilltarget.CategoryPlayable, inPeaceZone: true}
	target := &launchActor{id: 2, category: skilltarget.CategoryPlayable}

	nonOffensive := modelskill.Definition{Offensive: false}
	if got := RevalidateLaunch(caster, target, nonOffensive); got != LaunchAbortNone {
		t.Fatalf("RevalidateLaunch(non-offensive) = %v, want LaunchAbortNone", got)
	}

	npcTarget := &launchActor{id: 3, category: skilltarget.CategoryAttackable}
	offensive := modelskill.Definition{Offensive: true}
	if got := RevalidateLaunch(caster, npcTarget, offensive); got != LaunchAbortNone {
		t.Fatalf("RevalidateLaunch(caster peace zone, non-playable target) = %v, want LaunchAbortNone", got)
	}
}

func TestRevalidateLaunchAllGatesPass(t *testing.T) {
	caster := &launchActor{id: 1, knows: true, sees: true, category: skilltarget.CategoryPlayable}
	target := &launchActor{id: 2, category: skilltarget.CategoryPlayable}
	def := modelskill.Definition{Offensive: true, EffectRange: 100, Radius: 100}

	if got := RevalidateLaunch(caster, target, def); got != LaunchAbortNone {
		t.Fatalf("RevalidateLaunch() = %v, want LaunchAbortNone", got)
	}
}
