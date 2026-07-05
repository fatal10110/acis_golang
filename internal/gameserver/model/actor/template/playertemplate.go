// Package template contains actor template datatypes.
package template

import (
	"fmt"

	"github.com/fatal10110/acis_golang/internal/commons"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/holder/skillnode"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/records"
)

// PlayerTemplate holds the base stats, starter equipment, spawn points and
// granted skills for one player profession, e.g. Human Fighter, Warrior,
// Duelist (PlayerTemplate.java). The game defines one template per
// profession id, forming a tree that starts at 9 base professions and runs
// three tiers deep.
//
// The Java class extends CreatureTemplate, which isn't ported yet: the
// base-stat fields it would carry (STR..MEN, attack/defense, speeds,
// collision) are flattened here until another template type needs them
// (CreatureTemplate.java).
type PlayerTemplate struct {
	// ID is the raw profession id; it becomes a ClassId once the full enum
	// is ported (ClassId.java, partially in enums/actors).
	ID int

	// BaseLevel is the character level required to take this profession.
	BaseLevel int

	// FistsItemID is the weapon id used when a character of this profession
	// has nothing equipped. Resolving it to an actual weapon template
	// depends on the item table (#41).
	FistsItemID int

	STR, CON, DEX, INT, WIT, MEN int

	PAtk, PDef, MAtk, MDef float64
	RunSpeed, WalkSpeed    float64
	SwimSpeed              int

	CollisionRadius, CollisionHeight             float64
	CollisionRadiusFemale, CollisionHeightFemale float64

	// SafeFallHeight{Female,Male} is the fall distance, in units, a
	// character of this profession can drop without taking damage. The data
	// stores the female value first (Java indexes male as [1]).
	SafeFallHeightFemale, SafeFallHeightMale int

	// {HP,MP,CP}Table and their Regen counterparts are indexed by level-1,
	// giving the max/regen value at every character level.
	HPTable, MPTable, CPTable                []float64
	HPRegenTable, MPRegenTable, CPRegenTable []float64

	// Items and SpawnLocations are populated for the 9 base professions
	// only; every other profession in the tree carries none of its own.
	Items          []records.NewbieItem
	SpawnLocations []location.Location

	// Skills holds this profession's own granted skills; the loader appends
	// every ancestor profession's afterwards, so a character on this line
	// can learn anything the line ever unlocked.
	Skills []skillnode.GeneralSkillNode
}

// NewPlayerTemplate builds a PlayerTemplate from set, which carries the
// merged <set> attributes of one <class> element plus the "items", "skills"
// and "spawnLocations" lists the loader packed in — the same shape the Java
// constructor consumes.
func NewPlayerTemplate(set *commons.StatSet) (*PlayerTemplate, error) {
	id, err := set.GetInt("id")
	if err != nil {
		return nil, fmt.Errorf("player template: %w", err)
	}
	wrap := func(err error) error { return fmt.Errorf("player template %d: %w", id, err) }

	t := &PlayerTemplate{ID: id}

	if t.BaseLevel, err = set.GetInt("baseLvl"); err != nil {
		return nil, wrap(err)
	}
	if t.FistsItemID, err = set.GetInt("fists"); err != nil {
		return nil, wrap(err)
	}

	if t.STR, err = set.GetInt("str"); err != nil {
		return nil, wrap(err)
	}
	if t.CON, err = set.GetInt("con"); err != nil {
		return nil, wrap(err)
	}
	if t.DEX, err = set.GetInt("dex"); err != nil {
		return nil, wrap(err)
	}
	if t.INT, err = set.GetInt("int"); err != nil {
		return nil, wrap(err)
	}
	if t.WIT, err = set.GetInt("wit"); err != nil {
		return nil, wrap(err)
	}
	if t.MEN, err = set.GetInt("men"); err != nil {
		return nil, wrap(err)
	}

	if t.PAtk, err = set.GetDouble("pAtk"); err != nil {
		return nil, wrap(err)
	}
	if t.PDef, err = set.GetDouble("pDef"); err != nil {
		return nil, wrap(err)
	}
	if t.MAtk, err = set.GetDouble("mAtk"); err != nil {
		return nil, wrap(err)
	}
	if t.MDef, err = set.GetDouble("mDef"); err != nil {
		return nil, wrap(err)
	}
	if t.RunSpeed, err = set.GetDouble("runSpd"); err != nil {
		return nil, wrap(err)
	}
	if t.WalkSpeed, err = set.GetDouble("walkSpd"); err != nil {
		return nil, wrap(err)
	}

	// swimSpd defaults to 1 when absent but still fails on a malformed
	// value, matching Java's StatSet.getInteger(key, default), which only
	// defaults a missing key.
	t.SwimSpeed = 1
	if set.Has("swimSpd") {
		if t.SwimSpeed, err = set.GetInt("swimSpd"); err != nil {
			return nil, wrap(err)
		}
	}

	if t.CollisionRadius, err = set.GetDouble("radius"); err != nil {
		return nil, wrap(err)
	}
	if t.CollisionHeight, err = set.GetDouble("height"); err != nil {
		return nil, wrap(err)
	}
	if t.CollisionRadiusFemale, err = set.GetDouble("radiusFemale"); err != nil {
		return nil, wrap(err)
	}
	if t.CollisionHeightFemale, err = set.GetDouble("heightFemale"); err != nil {
		return nil, wrap(err)
	}

	safeFall, err := set.GetIntArray("safeFallHeight")
	if err != nil {
		return nil, wrap(err)
	}
	if len(safeFall) != 2 {
		return nil, wrap(fmt.Errorf("attribute %q: want 2 values, got %d", "safeFallHeight", len(safeFall)))
	}
	t.SafeFallHeightFemale, t.SafeFallHeightMale = safeFall[0], safeFall[1]

	if t.HPTable, err = set.GetDoubleArray("hpTable"); err != nil {
		return nil, wrap(err)
	}
	if t.MPTable, err = set.GetDoubleArray("mpTable"); err != nil {
		return nil, wrap(err)
	}
	if t.CPTable, err = set.GetDoubleArray("cpTable"); err != nil {
		return nil, wrap(err)
	}
	if t.HPRegenTable, err = set.GetDoubleArray("hpRegenTable"); err != nil {
		return nil, wrap(err)
	}
	if t.MPRegenTable, err = set.GetDoubleArray("mpRegenTable"); err != nil {
		return nil, wrap(err)
	}
	if t.CPRegenTable, err = set.GetDoubleArray("cpRegenTable"); err != nil {
		return nil, wrap(err)
	}

	if t.Items, err = commons.GetList[records.NewbieItem](set, "items"); err != nil {
		return nil, wrap(err)
	}
	if t.Skills, err = commons.GetList[skillnode.GeneralSkillNode](set, "skills"); err != nil {
		return nil, wrap(err)
	}
	if t.SpawnLocations, err = commons.GetList[location.Location](set, "spawnLocations"); err != nil {
		return nil, wrap(err)
	}

	return t, nil
}
