package skill

import (
	"testing"

	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/world"
)

// ---- from teleport_test.go ----
type jumpFakeTarget struct {
	world.Presence
	fakeActor
	heading, x, y, z int
}

func (t *jumpFakeTarget) Heading() int              { return t.heading }
func (t *jumpFakeTarget) Position() (int, int, int) { return t.x, t.y, t.z }
func (t *jumpFakeTarget) X() int                    { return t.x }
func (t *jumpFakeTarget) Y() int                    { return t.y }
func (t *jumpFakeTarget) Z() int                    { return t.z }

type jumpFakeCaster struct {
	neutralCreature
	world.Presence
	fakeActor
	aborted     bool
	broadcasted bool
	x, y, z     int
}

func (c *jumpFakeCaster) AbortAll(force bool) { c.aborted = true }
func (c *jumpFakeCaster) SetXYZ(x, y, z int)  { c.x, c.y, c.z = x, y, z }
func (c *jumpFakeCaster) BroadcastPosition()  { c.broadcasted = true }

func TestInstantJumpRepositionsBehindTarget(t *testing.T) {
	registry := NewDefaultRegistry()
	// Heading 0 faces due "east"; +180 degrees puts the jump point due
	// west of the target, 25 units out: cos(pi) = -1, sin(pi) = 0.
	target := &jumpFakeTarget{heading: 0, x: 100, y: 100, z: 50}
	caster := &jumpFakeCaster{}

	if !registry.Use(Cast{
		Caster:  caster,
		Skill:   modelskill.Definition{SkillType: "INSTANT_JUMP"},
		Targets: []Actor{target},
	}) {
		t.Fatal("Use() returned false for INSTANT_JUMP")
	}
	if !caster.aborted {
		t.Error("caster should abort its current action before jumping")
	}
	if !caster.broadcasted {
		t.Error("caster should broadcast its new position")
	}
	if caster.x != 75 || caster.y != 100 || caster.z != 50 {
		t.Errorf("caster position = (%d,%d,%d), want (75,100,50)", caster.x, caster.y, caster.z)
	}
}

// TestInstantJumpTruncatesLandingSum pins InstantJump.useSkill
// (InstantJump.java): the landing is (int)(targetX + 25*cos), the sum
// truncated toward zero, not the target coordinate plus a truncated offset.
func TestInstantJumpTruncatesLandingSum(t *testing.T) {
	for _, tc := range []struct {
		heading, x, y int
		wantX, wantY  int
	}{
		// 45 degrees: the jump point is 225 degrees, offset (-17.68,-17.68).
		{heading: 8192, x: 100, y: 100, wantX: 82, wantY: 82},
		// 135 degrees: offset (+17.68,-17.68).
		{heading: 24576, x: 100, y: 100, wantX: 117, wantY: 82},
		// Negative coordinates truncate toward zero: -100-17.68 = -117.68.
		{heading: 8192, x: -100, y: -100, wantX: -117, wantY: -117},
		{heading: 0, x: 100, y: 100, wantX: 75, wantY: 100},
	} {
		target := &jumpFakeTarget{heading: tc.heading, x: tc.x, y: tc.y, z: 50}
		caster := &jumpFakeCaster{}
		NewDefaultRegistry().Use(Cast{
			Caster:  caster,
			Skill:   modelskill.Definition{SkillType: "INSTANT_JUMP"},
			Targets: []Actor{target},
		})
		if caster.x != tc.wantX || caster.y != tc.wantY {
			t.Errorf("heading %d at (%d,%d): landed (%d,%d), want (%d,%d)", tc.heading, tc.x, tc.y, caster.x, caster.y, tc.wantX, tc.wantY)
		}
	}
}

func TestInstantJumpNoTargetsIsNoop(t *testing.T) {
	registry := NewDefaultRegistry()
	caster := &jumpFakeCaster{}
	registry.Use(Cast{Caster: caster, Skill: modelskill.Definition{SkillType: "INSTANT_JUMP"}})
	if caster.aborted {
		t.Error("caster should not act without a target")
	}
}

type getPlayerFakeCaster struct {
	neutralCreature
	world.Presence
	fakeActor
	x, y, z int
}

func (c *getPlayerFakeCaster) AlikeDead() bool           { return false }
func (c *getPlayerFakeCaster) Position() (int, int, int) { return c.x, c.y, c.z }

type getPlayerFakeTarget struct {
	world.Presence
	fakeActor
	dead       bool
	teleported bool
	tx, ty, tz int
}

func (t *getPlayerFakeTarget) AlikeDead() bool { return t.dead }
func (t *getPlayerFakeTarget) Dead() bool      { return t.dead }
func (t *getPlayerFakeTarget) TeleportTo(x, y, z int) {
	t.teleported = true
	t.tx, t.ty, t.tz = x, y, z
}

func TestGetPlayerPullsLivingTargetsToCaster(t *testing.T) {
	registry := NewDefaultRegistry()
	caster := &getPlayerFakeCaster{x: 1, y: 2, z: 3}
	target := &getPlayerFakeTarget{}
	deadTarget := &getPlayerFakeTarget{dead: true}

	registry.Use(Cast{
		Caster:  caster,
		Skill:   modelskill.Definition{SkillType: "GET_PLAYER"},
		Targets: []Actor{target, deadTarget},
	})

	if !target.teleported || target.tx != 1 || target.ty != 2 || target.tz != 3 {
		t.Fatalf("target not pulled to caster position: %+v", target)
	}
	if deadTarget.teleported {
		t.Fatal("dead target should not be teleported")
	}
}
