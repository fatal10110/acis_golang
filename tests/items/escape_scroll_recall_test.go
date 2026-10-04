package items

import (
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/rs/zerolog"

	gamesql "github.com/fatal10110/acis_golang/internal/gameserver/data/sql"
	"github.com/fatal10110/acis_golang/internal/gameserver/data/sql/sqltest"
	xmldata "github.com/fatal10110/acis_golang/internal/gameserver/data/xml"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/restart"
	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
	skillstate "github.com/fatal10110/acis_golang/internal/gameserver/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/world"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
	"github.com/fatal10110/acis_golang/internal/testsupport/datapack"
)

// shippedEscapeScrollSkill is the shipped Scroll of Escape skill (2013 level
// 1: RECALL, SELF, 20 s static hit time, no teleCoords, no recallType).
var shippedEscapeScrollSkill = sync.OnceValues(func() (modelskill.Definition, error) {
	dir, _ := datapack.Find()
	table, err := xmldata.LoadSkillDefinitions(filepath.Join(dir, "data", "xml", "skills"), zerolog.Nop())
	if err != nil {
		return modelskill.Definition{}, err
	}
	def, _ := table.Definition(modelskill.Ref{ID: 2013, Level: 1})
	return def, nil
})

// TestEscapeScrollRecallsToTown pins the Scroll of Escape half of the
// recall: using item 736 casts the shipped 2013, and at the end of its 20 s
// hit the caster is taken to its nearest town restart point within 20 of it.
// Before the hit ends the caster has not moved.
func TestEscapeScrollRecallsToTown(t *testing.T) {
	t.Parallel()
	datapack.Require(t)
	def, err := shippedEscapeScrollSkill()
	if err != nil {
		t.Fatalf("load shipped skills: %v", err)
	}
	if def.ID != 2013 || def.SkillType != "RECALL" || def.HitTime != 20000 || def.TeleCoords != nil {
		t.Fatalf("shipped 2013 = id %d type %q hit %d teleCoords %v, want RECALL with a 20000 hit and no teleCoords", def.ID, def.SkillType, def.HitTime, def.TeleCoords)
	}
	db := sqltest.SharedDB(t)
	skills := skillstate.NewPersistence(gamesql.NewSkillSaveStore(db), modelskill.NewTable([]modelskill.Definition{def}), gamesql.NewCharacterSkillStore(db))

	town := location.Location{X: -5000, Y: 2000, Z: 30}
	region := location.Point{
		X: (0-world.MinX)/world.TileSize + world.TileXMin,
		Y: (0-world.MinY)/world.TileSize + world.TileYMin,
	}
	srv := gameservertest.Boot(t,
		gameservertest.WithSkills(skills),
		gameservertest.WithRestartPoints(&restart.Table{Points: []restart.Point{{
			Name: "TestTown", MapRegions: []location.Point{region}, Points: []location.Location{town},
		}}}),
		gameservertest.WithCharacter("Newbie", 5, 0),
		gameservertest.WithWantChars(1))
	c, objID := srv.Client, srv.SoleObjectID(t)
	scroll := srv.GiveItem(t, objID, 736, 1)
	startInWorld(t, c)
	homeX, homeY, _ := srv.PlayerPosition(t, objID)

	c.Send(encodeUseItem(scroll, false))
	assertMagicSkillUseSelf(t, c.Read(), objID, 2013, 1, 20000, 0)

	srv.Advance(t, 19*time.Second)
	if x, y, _ := srv.PlayerPosition(t, objID); x != homeX || y != homeY {
		t.Fatalf("player at %d, %d before the cast's end, want it still at %d, %d", x, y, homeX, homeY)
	}
	srv.AdvanceUntil(t, "escape scroll recall", func() bool {
		x, y, _ := srv.PlayerPosition(t, objID)
		return x >= town.X-20 && x <= town.X+20 && y >= town.Y-20 && y <= town.Y+20
	})
}
