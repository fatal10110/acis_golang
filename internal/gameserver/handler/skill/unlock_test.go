package skill

import (
	"testing"

	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
)

type doorFake struct{ unlockable, opened bool }

func (d *doorFake) Unlockable() bool { return d.unlockable }
func (d *doorFake) Opened() bool     { return d.opened }
func (d *doorFake) Open()            { d.opened = true }

func TestUnlockDoorSpecialGuaranteedSuccess(t *testing.T) {
	registry := NewDefaultRegistry()
	door := &doorFake{unlockable: false}

	registry.Use(Cast{
		Skill:   modelskill.Definition{SkillType: "UNLOCK_SPECIAL", Power: 150},
		Targets: []any{door},
	})
	if !door.opened {
		t.Fatal("UNLOCK_SPECIAL with power >= 100 should always open, even an unlockable=false door")
	}
}

func TestUnlockDoorLevelZeroNeverOpens(t *testing.T) {
	registry := NewDefaultRegistry()
	door := &doorFake{unlockable: true}

	registry.Use(Cast{
		Skill:   modelskill.Definition{SkillType: "UNLOCK", Level: 0},
		Targets: []any{door},
	})
	if door.opened {
		t.Fatal("level 0 unlock should never open a door")
	}
}

func TestUnlockDoorNotUnlockableIsSkipped(t *testing.T) {
	registry := NewDefaultRegistry()
	door := &doorFake{unlockable: false}

	registry.Use(Cast{
		Skill:   modelskill.Definition{SkillType: "UNLOCK", Level: 4},
		Targets: []any{door},
	})
	if door.opened {
		t.Fatal("a non-unlockable door should not open via a regular UNLOCK")
	}
}

type chestFake struct {
	dead, interacted, box bool
	level                 int

	died, deleted          bool
	desireAdded, hateAdded bool
}

func (c *chestFake) Dead() bool       { return c.dead }
func (c *chestFake) Interacted() bool { return c.interacted }
func (c *chestFake) SetInteracted()   { c.interacted = true }
func (c *chestFake) Box() bool        { return c.box }
func (c *chestFake) Level() int       { return c.level }
func (c *chestFake) Die(killer any)   { c.died = true }
func (c *chestFake) DeleteMe()        { c.deleted = true }

func (c *chestFake) AddAttackDesire(attacker any, weight float64)     { c.desireAdded = true }
func (c *chestFake) AddDamageHate(attacker any, damage, hate float64) { c.hateAdded = true }

func TestUnlockChestNotBoxAddsAttackDesire(t *testing.T) {
	registry := NewDefaultRegistry()
	chest := &chestFake{box: false}

	registry.Use(Cast{Skill: modelskill.Definition{SkillType: "UNLOCK"}, Targets: []any{chest}})
	if !chest.desireAdded {
		t.Fatal("expected an attack desire for a non-box chest")
	}
	if chest.interacted {
		t.Fatal("a non-box chest should not be marked interacted")
	}
}

func TestUnlockChestDeluxeKeyExactLevelMatchGuaranteedOpen(t *testing.T) {
	registry := NewDefaultRegistry()
	chest := &chestFake{box: true, level: 100}

	registry.Use(Cast{
		Skill:   modelskill.Definition{SkillType: "DELUXE_KEY_UNLOCK", ID: 9999, Level: 10},
		Targets: []any{chest},
	})
	if !chest.interacted {
		t.Fatal("chest should be marked interacted")
	}
	if !chest.died || !chest.hateAdded {
		t.Fatalf("exact-level deluxe key should always open: died=%v hateAdded=%v", chest.died, chest.hateAdded)
	}
}

func TestUnlockChestAboveBracketTooLowSkillGuaranteedFail(t *testing.T) {
	registry := NewDefaultRegistry()
	chest := &chestFake{box: true, level: 70}

	registry.Use(Cast{
		Skill:   modelskill.Definition{SkillType: "UNLOCK", Level: 5},
		Targets: []any{chest},
	})
	if chest.died {
		t.Fatal("a level 5 unlock skill should never open a level-70 chest")
	}
	if !chest.deleted {
		t.Fatal("a failed chest unlock should delete the chest")
	}
}
