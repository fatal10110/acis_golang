package player

import (
	"fmt"
	"math/rand/v2"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
)

// defaultAccessLevel is the access level a freshly created character starts
// at, matching the shipped server default.
const defaultAccessLevel = 0

// Character is one persisted characters-table row: the identity, appearance
// and progress state needed to create, list, delete and restore a
// character. It is not the live in-world actor — combat, AI and other
// runtime-only concerns arrive with the systems that need them.
type Character struct {
	ObjectID    int32
	AccountName string
	Name        string

	ClassID     int
	BaseClassID int
	Race        Race
	Sex         Sex

	Level int
	Exp   int64
	SP    int

	MaxHP, CurHP float64
	MaxCP, CurCP float64
	MaxMP, CurMP float64

	Face, HairStyle, HairColor int

	// Position and Heading are the character's last known world location.
	// A freshly created character carries the template's chosen spawn point
	// only in memory: the characters table leaves these columns at their
	// NULL default until the character is actually saved.
	Position location.Location
	Heading  int

	Karma             int
	PvPKills, PKKills int

	ClanID      int
	Title       string
	AccessLevel int

	// DeleteAt is the persisted deletion deadline, in epoch milliseconds;
	// zero means the character is not scheduled for deletion.
	DeleteAt   int64
	LastAccess int64
}

// NewCharacter builds a freshly created Character of profession tmpl for
// accountName, seeded with the profession's level-1 base stats and a
// random spawn point from its template. name, hairStyle, hairColor, face
// and sex are the client-supplied appearance fields; the caller is
// responsible for validating them (name charset/length, hair/face bounds)
// before calling this, since those are wire-format concerns, not modeling
// ones.
//
// objectID must already be allocated by the caller (character creation
// needs the id before the row is inserted, to grant items owned by it).
func NewCharacter(objectID int32, tmpl *Template, accountName, name string, hairStyle, hairColor, face byte, sex Sex) (*Character, error) {
	if tmpl == nil {
		return nil, fmt.Errorf("player: new character: nil template")
	}
	race, ok := ClassRace(tmpl.ID)
	if !ok {
		return nil, fmt.Errorf("player: new character: class %d has no known race", tmpl.ID)
	}
	if len(tmpl.HPTable) == 0 || len(tmpl.MPTable) == 0 || len(tmpl.CPTable) == 0 {
		return nil, fmt.Errorf("player: new character: class %d template has no level tables", tmpl.ID)
	}

	c := &Character{
		ObjectID:    objectID,
		AccountName: accountName,
		Name:        name,

		ClassID:     tmpl.ID,
		BaseClassID: tmpl.ID,
		Race:        race,
		Sex:         sex,

		Level: 1,

		MaxHP: tmpl.HPTable[0], CurHP: tmpl.HPTable[0],
		MaxCP: tmpl.CPTable[0], CurCP: tmpl.CPTable[0],
		MaxMP: tmpl.MPTable[0], CurMP: tmpl.MPTable[0],

		Face: int(face), HairStyle: int(hairStyle), HairColor: int(hairColor),

		AccessLevel: defaultAccessLevel,
	}

	if len(tmpl.Spawns) > 0 {
		c.Position = tmpl.Spawns[rand.IntN(len(tmpl.Spawns))]
	}

	return c, nil
}
