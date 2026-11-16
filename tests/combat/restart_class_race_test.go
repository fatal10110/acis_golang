package combat

import (
	"context"
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/player"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/restart"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
)

// spellsingerClass is an elven profession, played here as the subclass of
// a human.
const spellsingerClass = 27

var (
	humanAreaRestartPoint = location.Location{X: -12000, Y: 120000, Z: -3000}
	elfAreaRestartPoint   = location.Location{X: 45487, Y: 49767, Z: -2950}
)

// raceSplitRestartTable has one area around the spawn that sends humans to
// one town and elves to another.
func raceSplitRestartTable(t *testing.T) *restart.Table {
	t.Helper()
	nodes := []location.Point{
		{X: playerOrigin.X - 2000, Y: playerOrigin.Y - 2000},
		{X: playerOrigin.X + 2000, Y: playerOrigin.Y - 2000},
		{X: playerOrigin.X + 2000, Y: playerOrigin.Y + 2000},
		{X: playerOrigin.X - 2000, Y: playerOrigin.Y + 2000},
	}
	area, err := restart.NewArea(nodes, -10000, 10000, map[player.Race]string{
		player.RaceHuman: "HumanTown",
		player.RaceElf:   "ElfTown",
	})
	if err != nil {
		t.Fatalf("NewArea: %v", err)
	}
	return &restart.Table{
		Areas: []restart.Area{area},
		Points: []restart.Point{
			{Name: "HumanTown", Points: []location.Location{humanAreaRestartPoint}, ChaoPoints: []location.Location{humanAreaRestartPoint}},
			{Name: "ElfTown", Points: []location.Location{elfAreaRestartPoint}, ChaoPoints: []location.Location{elfAreaRestartPoint}},
		},
	}
}

// TestRestartAreaFollowsActiveClassRace kills a human playing an elven
// subclass inside an area whose restart town depends on race: the restart
// lands in the elven town, the town of the class being played.
func TestRestartAreaFollowsActiveClassRace(t *testing.T) {
	t.Parallel()
	// The Spellsinger line: Elven Mystic, Elven Wizard, Spellsinger.
	var elvenLine []*player.Template
	for _, id := range []int{25, 26, spellsingerClass} {
		tmpl := gameservertest.ClassTemplate()
		tmpl.ID = id
		elvenLine = append(elvenLine, tmpl)
	}
	srv := gameservertest.Boot(t,
		gameservertest.WithCharacter("Singer", 40, 0),
		gameservertest.WithWantChars(1),
		gameservertest.WithClassTemplates(elvenLine...),
		gameservertest.WithRestartPoints(raceSplitRestartTable(t)),
	)
	c, objID := srv.Client, srv.SoleObjectID(t)
	ctx := context.Background()
	for _, q := range []struct {
		query string
		args  []any
	}{
		{`UPDATE characters SET classid = ?, base_class = 0 WHERE obj_Id = ?`, []any{spellsingerClass, objID}},
		{`INSERT INTO character_subclasses (char_obj_id, class_id, exp, sp, level, class_index) VALUES (?, ?, 0, 0, 40, 1)`, []any{objID, spellsingerClass}},
	} {
		if _, err := srv.DB.ExecContext(ctx, q.query, q.args...); err != nil {
			t.Fatalf("%s: %v", q.query, err)
		}
	}
	startInWorld(t, c)

	srv.MarkPlayerDead(t, objID)
	c.Send(encodeRequestRestartPoint(0))
	assertRestartTeleport(t, c, objID, elfAreaRestartPoint)
}
