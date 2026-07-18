package player

import (
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
)

func humanFighterTemplate() *Template {
	return &Template{
		ID:        0,
		BaseLevel: 1,
		HPTable:   []float64{80, 91.83},
		MPTable:   []float64{30, 35.46},
		CPTable:   []float64{32, 36.732},
		Spawns: []location.Location{
			{X: 1, Y: 2, Z: 3},
		},
	}
}

func TestNewCharacter(t *testing.T) {
	tmpl := humanFighterTemplate()

	c, err := NewCharacter(0x10000001, tmpl, "acct1", "Newbie", 1, 2, 0, SexMale)
	if err != nil {
		t.Fatalf("NewCharacter() unexpected error: %v", err)
	}

	if c.ID != 0x10000001 || c.AccountName != "acct1" || c.Name != "Newbie" {
		t.Fatalf("NewCharacter() identity = %+v", c)
	}
	if c.ClassID != 0 || c.BaseClassID != 0 {
		t.Errorf("ClassID/BaseClassID = %d/%d, want 0/0", c.ClassID, c.BaseClassID)
	}
	if c.Race != RaceHuman {
		t.Errorf("Race = %v, want %v", c.Race, RaceHuman)
	}
	if c.Sex != SexMale {
		t.Errorf("Sex = %v, want %v", c.Sex, SexMale)
	}
	if c.Level != 1 {
		t.Errorf("Level = %d, want 1", c.Level)
	}
	res := c.ResourceValues()
	if res.MaxHP != tmpl.HPTable[0] || res.CurrentHP != tmpl.HPTable[0] {
		t.Errorf("HP = %v/%v, want %v/%v", res.MaxHP, res.CurrentHP, tmpl.HPTable[0], tmpl.HPTable[0])
	}
	if res.MaxMP != tmpl.MPTable[0] || res.MaxCP != tmpl.CPTable[0] {
		t.Errorf("MaxMP/MaxCP = %v/%v, want %v/%v", res.MaxMP, res.MaxCP, tmpl.MPTable[0], tmpl.CPTable[0])
	}
	if c.HairStyle != 1 || c.HairColor != 2 || c.Face != 0 {
		t.Errorf("appearance = hairStyle=%d hairColor=%d face=%d, want 1/2/0", c.HairStyle, c.HairColor, c.Face)
	}
	if c.Location != tmpl.Spawns[0] {
		t.Errorf("Location = %+v, want %+v", c.Location, tmpl.Spawns[0])
	}
	if c.AccessLevel != defaultAccessLevel {
		t.Errorf("AccessLevel = %d, want %d", c.AccessLevel, defaultAccessLevel)
	}
}

func TestNewCharacter_NilTemplate(t *testing.T) {
	if _, err := NewCharacter(1, nil, "acct1", "Newbie", 0, 0, 0, SexMale); err == nil {
		t.Fatal("NewCharacter() with nil template: want error, got nil")
	}
}

func TestNewCharacter_UnknownClass(t *testing.T) {
	tmpl := humanFighterTemplate()
	tmpl.ID = 9999
	if _, err := NewCharacter(1, tmpl, "acct1", "Newbie", 0, 0, 0, SexMale); err == nil {
		t.Fatal("NewCharacter() with unknown class id: want error, got nil")
	}
}

func TestNewCharacter_MissingLevelTables(t *testing.T) {
	tmpl := humanFighterTemplate()
	tmpl.HPTable = nil
	if _, err := NewCharacter(1, tmpl, "acct1", "Newbie", 0, 0, 0, SexMale); err == nil {
		t.Fatal("NewCharacter() with no HP table: want error, got nil")
	}
}

func TestNewCharacter_NoSpawnsLeavesZeroPosition(t *testing.T) {
	tmpl := humanFighterTemplate()
	tmpl.Spawns = nil

	c, err := NewCharacter(1, tmpl, "acct1", "Newbie", 0, 0, 0, SexMale)
	if err != nil {
		t.Fatalf("NewCharacter() unexpected error: %v", err)
	}
	if c.Location != (location.Location{}) {
		t.Errorf("Location = %+v, want zero value", c.Location)
	}
}
