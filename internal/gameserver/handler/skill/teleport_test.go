package skill

import (
	"testing"

	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
)

type jumpFakeTarget struct{ heading, x, y, z int }

func (t jumpFakeTarget) Heading() int { return t.heading }
func (t jumpFakeTarget) X() int       { return t.x }
func (t jumpFakeTarget) Y() int       { return t.y }
func (t jumpFakeTarget) Z() int       { return t.z }

type jumpFakeCaster struct {
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
	target := jumpFakeTarget{heading: 0, x: 100, y: 100, z: 50}
	caster := &jumpFakeCaster{}

	if !registry.Use(Cast{
		Caster:  caster,
		Skill:   modelskill.Definition{SkillType: "INSTANT_JUMP"},
		Targets: []any{target},
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

func TestInstantJumpNoTargetsIsNoop(t *testing.T) {
	registry := NewDefaultRegistry()
	caster := &jumpFakeCaster{}
	registry.Use(Cast{Caster: caster, Skill: modelskill.Definition{SkillType: "INSTANT_JUMP"}})
	if caster.aborted {
		t.Error("caster should not act without a target")
	}
}

type getPlayerFakeCaster struct{ x, y, z int }

func (c getPlayerFakeCaster) AlikeDead() bool           { return false }
func (c getPlayerFakeCaster) Position() (int, int, int) { return c.x, c.y, c.z }

type getPlayerFakeTarget struct {
	dead       bool
	teleported bool
	tx, ty, tz int
}

func (t *getPlayerFakeTarget) AlikeDead() bool { return t.dead }
func (t *getPlayerFakeTarget) TeleportTo(x, y, z int) {
	t.teleported = true
	t.tx, t.ty, t.tz = x, y, z
}

func TestGetPlayerPullsLivingTargetsToCaster(t *testing.T) {
	registry := NewDefaultRegistry()
	caster := getPlayerFakeCaster{x: 1, y: 2, z: 3}
	target := &getPlayerFakeTarget{}
	deadTarget := &getPlayerFakeTarget{dead: true}

	registry.Use(Cast{
		Caster:  caster,
		Skill:   modelskill.Definition{SkillType: "GET_PLAYER"},
		Targets: []any{target, deadTarget},
	})

	if !target.teleported || target.tx != 1 || target.ty != 2 || target.tz != 3 {
		t.Fatalf("target not pulled to caster position: %+v", target)
	}
	if deadTarget.teleported {
		t.Fatal("dead target should not be teleported")
	}
}
